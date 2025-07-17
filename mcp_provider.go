package UTCP

// MCPProvider is the concrete provider your transport expects.
type MCPProvider struct {
	name string
}

// NewMCPProvider constructs one with the given name.
func NewMCPProvider(name string) *MCPProvider {
	return &MCPProvider{name: name}
}

// Type satisfies your Provider interface.
func (p *MCPProvider) Type() ProviderType {
	return ProviderType("mcp")
}

// Name returns the provider’s human‐readable name.
func (p *MCPProvider) Name() string {
	return p.name
}
