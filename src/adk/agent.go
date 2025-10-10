package adk

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/universal-tool-calling-protocol/go-utcp/src/manual"
	base "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// Handler represents the function signature used for UTCP tools created via the ADK.
type Handler func(ctx context.Context, input map[string]any) (map[string]any, error)

// ToolDefinition describes a tool that should be exposed by the agent.
type ToolDefinition struct {
	Name                string
	Description         string
	Tags                []string
	Inputs              *tools.ToolInputOutputSchema
	Outputs             *tools.ToolInputOutputSchema
	AverageResponseSize *int
	Provider            base.Provider
	Handler             Handler
}

// AgentOption mutates an Agent during construction.
type AgentOption func(*Agent)

// Agent is a lightweight registry for tools and metadata exposed over UTCP.
type Agent struct {
	name        string
	description string
	version     string

	defaultProvider base.Provider

	mu    sync.RWMutex
	tools map[string]*tools.Tool
	order []string
}

// NewAgent creates a new Agent with sensible defaults suitable for exposing UTCP tools.
func NewAgent(name string, opts ...AgentOption) *Agent {
	ag := &Agent{
		name:            name,
		version:         manual.Version,
		defaultProvider: &base.BaseProvider{Name: name, ProviderType: base.ProviderCLI},
		tools:           make(map[string]*tools.Tool),
	}
	for _, opt := range opts {
		opt(ag)
	}
	return ag
}

// WithDescription sets the human friendly description of the agent.
func WithDescription(description string) AgentOption {
	return func(a *Agent) {
		a.description = description
	}
}

// WithVersion overrides the UTCP manual version advertised by the agent.
func WithVersion(version string) AgentOption {
	return func(a *Agent) {
		if version != "" {
			a.version = version
		}
	}
}

// WithDefaultProvider sets the provider metadata used for tools that do not specify their own provider.
func WithDefaultProvider(provider base.Provider) AgentOption {
	return func(a *Agent) {
		if provider != nil {
			a.defaultProvider = provider
		}
	}
}

// Description returns the description configured for the agent.
func (a *Agent) Description() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.description
}

// Name returns the agent name.
func (a *Agent) Name() string {
	return a.name
}

// RegisterTool wires a tool handler into the agent and returns the created Tool instance.
func (a *Agent) RegisterTool(def ToolDefinition) (*tools.Tool, error) {
	if def.Name == "" {
		return nil, errors.New("tool name is required")
	}
	if def.Handler == nil {
		return nil, fmt.Errorf("tool %q must provide a handler", def.Name)
	}

	provider := def.Provider
	if provider == nil {
		provider = a.defaultProvider
	}
	inputs := def.Inputs
	if inputs == nil {
		inputs = &tools.ToolInputOutputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}}
	}
	outputs := def.Outputs
	if outputs == nil {
		outputs = &tools.ToolInputOutputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}}
	}

	tool := &tools.Tool{
		Name:                def.Name,
		Description:         def.Description,
		Inputs:              *inputs,
		Outputs:             *outputs,
		Tags:                append([]string(nil), def.Tags...),
		AverageResponseSize: def.AverageResponseSize,
		Provider:            provider,
	}
	tool.Handler = func(ctx map[string]any, inputs map[string]any) (map[string]any, error) {
		var goCtx context.Context = context.Background()
		if ctx != nil {
			if raw, ok := ctx["context"].(context.Context); ok && raw != nil {
				goCtx = raw
			}
		}
		return def.Handler(goCtx, inputs)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.tools[def.Name]; exists {
		return nil, fmt.Errorf("tool %q already registered", def.Name)
	}
	a.tools[def.Name] = tool
	a.order = append(a.order, def.Name)
	return tool, nil
}

// MustRegisterTool registers the tool or panics on error.
func (a *Agent) MustRegisterTool(def ToolDefinition) *tools.Tool {
	tool, err := a.RegisterTool(def)
	if err != nil {
		panic(err)
	}
	return tool
}

// Manual returns the UTCP manual that describes the agent.
func (a *Agent) Manual() manual.UtcpManual {
	a.mu.RLock()
	defer a.mu.RUnlock()
	manualCopy := manual.UtcpManual{
		Version: a.version,
		Name:    a.name,
	}
	manualCopy.Tools = make([]tools.Tool, 0, len(a.tools))
	for _, name := range a.order {
		t := a.tools[name]
		manualCopy.Tools = append(manualCopy.Tools, tools.Tool{
			Name:                t.Name,
			Description:         t.Description,
			Inputs:              t.Inputs,
			Outputs:             t.Outputs,
			Tags:                append([]string(nil), t.Tags...),
			AverageResponseSize: t.AverageResponseSize,
			Provider:            t.Provider,
		})
	}
	return manualCopy
}

// Tools returns a snapshot of the registered tools including handlers.
func (a *Agent) Tools() []tools.Tool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]tools.Tool, 0, len(a.tools))
	for _, name := range a.order {
		if t, ok := a.tools[name]; ok {
			out = append(out, *t)
		}
	}
	return out
}

// Call executes the named tool with the provided input payload.
func (a *Agent) Call(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
	a.mu.RLock()
	tool, ok := a.tools[toolName]
	a.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("tool %q not found", toolName)
	}
	if tool.Handler == nil {
		return nil, fmt.Errorf("tool %q is missing a handler", toolName)
	}
	if input == nil {
		input = map[string]any{}
	}
	ctxMap := map[string]any{}
	if ctx != nil {
		ctxMap["context"] = ctx
	}
	outputs, err := tool.Handler(ctxMap, input)
	if err != nil {
		return nil, err
	}
	if outputs == nil {
		outputs = map[string]any{}
	}
	return outputs, nil
}

// ToolNames returns the registered tool names in registration order.
func (a *Agent) ToolNames() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]string(nil), a.order...)
}
