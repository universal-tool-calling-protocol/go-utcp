package repository

import (
	"context"
	"fmt"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	"sync"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/cli"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/graphql"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/sse"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/streamable"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/tcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/text"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/udp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/webrtc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/websocket"
)

type InMemoryToolRepository struct {
	Tools     map[string][]Tool   // providerName -> tools
	Providers map[string]Provider // providerName -> Provider
	mu        sync.RWMutex        // for concurrent access
}

func (r InMemoryToolRepository) GetProvider(ctx context.Context, providerName string) (*Provider, error) {
	provider, ok := r.Providers[providerName]
	if !ok {
		return nil, nil
	}
	return &provider, nil
}

func (r InMemoryToolRepository) GetProviders(ctx context.Context) ([]Provider, error) {
	var providers []Provider
	for _, p := range r.Providers {
		providers = append(providers, p)
	}
	return providers, nil
}

func (r InMemoryToolRepository) GetTool(ctx context.Context, toolName string) (*Tool, error) {
	for _, tools := range r.Tools {
		for _, tool := range tools {
			if tool.Name == toolName {
				return &tool, nil
			}
		}
	}
	return nil, nil
}

func (r InMemoryToolRepository) GetTools(ctx context.Context) ([]Tool, error) {
	var all []Tool
	for _, tools := range r.Tools {
		all = append(all, tools...)
	}
	return all, nil
}

func (r InMemoryToolRepository) GetToolsByProvider(ctx context.Context, providerName string) ([]Tool, error) {
	tools, ok := r.Tools[providerName]
	if !ok {
		return nil, fmt.Errorf("no tools found for provider %s", providerName)
	}
	return tools, nil
}

func (r InMemoryToolRepository) RemoveProvider(ctx context.Context, providerName string) error {
	if _, ok := r.Providers[providerName]; !ok {
		return fmt.Errorf("provider not found: %s", providerName)
	}
	delete(r.Providers, providerName)
	delete(r.Tools, providerName)
	return nil
}

func (r InMemoryToolRepository) RemoveTool(ctx context.Context, toolName string) error {
	for providerName, tools := range r.Tools {
		for i, tool := range tools {
			if tool.Name == toolName {
				r.Tools[providerName] = append(tools[:i], tools[i+1:]...)
				return nil
			}
		}
	}
	return fmt.Errorf("tool not found: %s", toolName)
}

func (r *InMemoryToolRepository) SaveProviderWithTools(ctx context.Context, provider Provider, tools []Tool) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	var providerName string
	switch p := provider.(type) {
	case *CliProvider:
		providerName = p.Name
	case *HttpProvider:
		providerName = p.Name
	case *SSEProvider:
		providerName = p.Name
	case *StreamableHttpProvider:
		providerName = p.Name
	case *WebSocketProvider:
		providerName = p.Name
	case *GRPCProvider:
		providerName = p.Name
	case *GraphQLProvider:
		providerName = p.Name
	case *TCPProvider:
		providerName = p.Name
	case *UDPProvider:
		providerName = p.Name
	case *WebRTCProvider:
		providerName = p.Name
	case *MCPProvider:
		providerName = p.Name
	case *TextProvider:
		providerName = p.Name
	default:
		return fmt.Errorf("unsupported provider type for saving: %T", provider)
	}
	r.Providers[providerName] = provider
	r.Tools[providerName] = tools
	return nil
}

func NewInMemoryToolRepository() ToolRepository {
	return &InMemoryToolRepository{
		Tools:     make(map[string][]Tool),
		Providers: make(map[string]Provider),
		mu:        sync.RWMutex{},
	}
}

// TextTransport interface for setting base path
// kept here for tests relying on it
type TextTransport interface {
	ClientTransport
	SetBasePath(path string)
}
