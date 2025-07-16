package transport

import (
	"context"

	"core"
)

// ClientTransport defines how a client registers, deregisters, and invokes UTCP tools.
type ClientTransport interface {
	// RegisterToolProvider registers a tool provider (e.g. via the /utcp endpoint)
	// and returns the list of tools it exposes.
	RegisterToolProvider(ctx context.Context, manualProvider core.Provider) ([]core.Tool, error)

	// DeregisterToolProvider removes a previously registered provider.
	DeregisterToolProvider(ctx context.Context, manualProvider core.Provider) error

	// CallTool invokes a named tool with the given arguments on a specific provider.
	// It returns whatever the tool returns (often map[string]interface{} or a typed result).
	CallTool(ctx context.Context, toolName string, arguments map[string]any, toolProvider core.Provider, l *string) (any, error)
}
