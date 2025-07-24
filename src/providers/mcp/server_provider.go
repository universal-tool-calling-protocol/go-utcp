package mcp

import (
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// MCPServerProvider represents a remote MCP server reachable via HTTP.
type MCPServerProvider struct {
	BaseProvider
	URL     string            `json:"url"`
	Timeout int               `json:"timeout,omitempty"` // seconds
	Headers map[string]string `json:"headers,omitempty"`
}

func NewMCPServerProvider(url string) *MCPServerProvider {
	return &MCPServerProvider{
		BaseProvider: BaseProvider{ProviderType: ProviderMCPServer},
		URL:          url,
		Headers:      make(map[string]string),
	}
}

func (p *MCPServerProvider) Type() ProviderType { return ProviderMCPServer }
