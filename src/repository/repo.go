package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	providerhelpers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/helpers"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

type InMemoryToolRepository struct {
	// Tools and Providers remain exported for source compatibility. Treat them as
	// read-only after the first repository operation; mutate through the methods.
	Tools     map[string][]Tool
	Providers map[string]Provider

	mu            sync.RWMutex
	toolIndex     map[string]Tool
	toolProviders map[string]string
	toolCount     int
	toolSnapshot  []Tool
	snapshotReady bool
	revision      atomic.Uint64
}

func (r *InMemoryToolRepository) GetProvider(ctx context.Context, providerName string) (*Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.Providers[providerName]
	if !ok {
		return nil, nil
	}
	return &provider, nil
}

func (r *InMemoryToolRepository) GetProviders(ctx context.Context) ([]Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	providers := make([]Provider, 0, len(r.Providers))
	for _, p := range r.Providers {
		providers = append(providers, p)
	}
	return providers, nil
}

func (r *InMemoryToolRepository) GetTool(ctx context.Context, toolName string) (*Tool, error) {
	r.ensureIndex()
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.toolIndex[toolName]
	if !ok {
		return nil, nil
	}
	return &tool, nil
}

func (r *InMemoryToolRepository) GetTools(ctx context.Context) ([]Tool, error) {
	r.ensureIndex()
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]Tool, 0, r.toolCount)
	for _, tools := range r.Tools {
		all = append(all, tools...)
	}
	return all, nil
}

// RangeTools visits a stable repository snapshot without first copying the
// complete catalog. Returning false stops iteration early.
func (r *InMemoryToolRepository) RangeTools(ctx context.Context, visit func(Tool) bool) error {
	snapshot := r.snapshot()
	for index, tool := range snapshot {
		if index&63 == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if !visit(tool) {
			return nil
		}
	}
	return nil
}

// ToolRevision returns the current catalog revision for search-index caches.
func (r *InMemoryToolRepository) ToolRevision() uint64 {
	return r.revision.Load()
}

func (r *InMemoryToolRepository) GetToolsByProvider(ctx context.Context, providerName string) ([]Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools, ok := r.Tools[providerName]
	if !ok {
		return nil, fmt.Errorf("no tools found for provider %s", providerName)
	}
	return append([]Tool(nil), tools...), nil
}

func (r *InMemoryToolRepository) RemoveProvider(ctx context.Context, providerName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Providers[providerName]; !ok {
		return fmt.Errorf("provider not found: %s", providerName)
	}
	r.ensureIndexLocked()
	for _, tool := range r.Tools[providerName] {
		delete(r.toolIndex, tool.Name)
		delete(r.toolProviders, tool.Name)
		r.toolCount--
	}
	delete(r.Tools, providerName)
	delete(r.Providers, providerName)
	r.invalidateSnapshotLocked()
	r.revision.Add(1)
	return nil
}

func (r *InMemoryToolRepository) RemoveTool(ctx context.Context, toolName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureIndexLocked()
	providerName, ok := r.toolProviders[toolName]
	if !ok {
		return fmt.Errorf("tool not found: %s", toolName)
	}
	providerTools := r.Tools[providerName]
	for i := range providerTools {
		if providerTools[i].Name != toolName {
			continue
		}
		copy(providerTools[i:], providerTools[i+1:])
		providerTools[len(providerTools)-1] = Tool{}
		r.Tools[providerName] = providerTools[:len(providerTools)-1]
		delete(r.toolIndex, toolName)
		delete(r.toolProviders, toolName)
		r.toolCount--
		r.invalidateSnapshotLocked()
		r.revision.Add(1)
		return nil
	}
	return fmt.Errorf("tool not found: %s", toolName)
}

func (r *InMemoryToolRepository) SaveProviderWithTools(ctx context.Context, provider Provider, tools []Tool) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	providerName, ok := providerhelpers.ProviderName(provider)
	if !ok {
		return fmt.Errorf("unsupported provider type for saving: %T", provider)
	}
	r.ensureIndexLocked()
	if previous, exists := r.Tools[providerName]; exists {
		for _, tool := range previous {
			delete(r.toolIndex, tool.Name)
			delete(r.toolProviders, tool.Name)
		}
		r.toolCount -= len(previous)
	}
	tools = append([]Tool(nil), tools...)
	r.Providers[providerName] = provider
	r.Tools[providerName] = tools
	for _, tool := range tools {
		r.toolIndex[tool.Name] = tool
		r.toolProviders[tool.Name] = providerName
	}
	r.toolCount += len(tools)
	r.invalidateSnapshotLocked()
	r.revision.Add(1)
	return nil
}

func NewInMemoryToolRepository() ToolRepository {
	return &InMemoryToolRepository{
		Tools:     make(map[string][]Tool),
		Providers: make(map[string]Provider),
	}
}

func (r *InMemoryToolRepository) ensureIndex() {
	r.mu.RLock()
	ready := r.toolIndex != nil && r.toolProviders != nil
	r.mu.RUnlock()
	if ready {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureIndexLocked()
}

func (r *InMemoryToolRepository) ensureIndexLocked() {
	if r.toolIndex != nil && r.toolProviders != nil {
		return
	}
	if r.Tools == nil {
		r.Tools = make(map[string][]Tool)
	}
	if r.Providers == nil {
		r.Providers = make(map[string]Provider)
	}
	r.toolIndex = make(map[string]Tool)
	r.toolProviders = make(map[string]string)
	r.toolCount = 0
	for providerName, providerTools := range r.Tools {
		for _, tool := range providerTools {
			r.toolIndex[tool.Name] = tool
			r.toolProviders[tool.Name] = providerName
			r.toolCount++
		}
	}
}

func (r *InMemoryToolRepository) snapshot() []Tool {
	r.ensureIndex()
	r.mu.RLock()
	if r.snapshotReady {
		snapshot := r.toolSnapshot
		r.mu.RUnlock()
		return snapshot
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.snapshotReady {
		r.toolSnapshot = make([]Tool, 0, r.toolCount)
		for _, providerTools := range r.Tools {
			r.toolSnapshot = append(r.toolSnapshot, providerTools...)
		}
		r.snapshotReady = true
	}
	return r.toolSnapshot
}

func (r *InMemoryToolRepository) invalidateSnapshotLocked() {
	r.toolSnapshot = nil
	r.snapshotReady = false
}

// TextTransport interface for setting base path
// kept here for tests relying on it
type TextTransport interface {
	ClientTransport
	SetBasePath(path string)
}
