package transport

import (
	"context"
	"core"
)

// ToolRepository defines the contract for persisting providers and their tools.
type ToolRepository interface {
	// SaveProviderWithTools saves a provider and its associated tools.
	SaveProviderWithTools(ctx context.Context, provider core.Provider, tools []core.Tool) error

	// RemoveProvider removes a provider and all its tools by name.
	// Returns an error if the provider does not exist.
	RemoveProvider(ctx context.Context, providerName string) error

	// RemoveTool removes a single tool by name.
	// Returns an error if the tool does not exist.
	RemoveTool(ctx context.Context, toolName string) error

	// GetTool retrieves a tool by name.
	// Returns (nil, nil) if the tool is not found.
	GetTool(ctx context.Context, toolName string) (*core.Tool, error)

	// GetTools returns all tools in the repository.
	GetTools(ctx context.Context) ([]core.Tool, error)

	// GetToolsByProvider returns all tools for a specific provider.
	// Returns (nil, nil) if the provider is not found.
	GetToolsByProvider(ctx context.Context, providerName string) ([]core.Tool, error)

	// GetProvider retrieves a provider by name.
	// Returns (nil, nil) if the provider is not found.
	GetProvider(ctx context.Context, providerName string) (*core.Provider, error)

	// GetProviders returns all providers in the repository.
	GetProviders(ctx context.Context) ([]core.Provider, error)
}
