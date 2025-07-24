package mcp

import (
	"encoding/json"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"

	"errors"
)

// McpStdioServer config for stdio transport.
type McpStdioServer struct {
	Transport string            `json:"transport"` // always "stdio"
	Command   string            `json:"command"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// McpHttpServer config for HTTP transport.
type McpHttpServer struct {
	Transport string `json:"transport"` // always "http"
	URL       string `json:"url"`
}

// McpServer is a union of the two MCP transports.
type McpServer interface{}
type McpConfig struct {
	McpServers map[string]McpServer `json:"mcpServers"`
}

// MCPProvider represents an MCP (Model Context Protocol) tool provider.
type MCPProvider struct {
	Name       string            `json:"name" yaml:"name"`
	Command    []string          `json:"command" yaml:"command"`
	Args       []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	WorkingDir string            `json:"workingDir,omitempty" yaml:"workingDir,omitempty"`
	StdinData  string            `json:"stdinData,omitempty" yaml:"stdinData,omitempty"`
	Timeout    int               `json:"timeout,omitempty" yaml:"timeout,omitempty"` // seconds
}

// NewMCPProvider constructs a new MCPProvider with the given name and command.
func NewMCPProvider(name string, command []string) *MCPProvider {
	return &MCPProvider{
		Name:    name,
		Command: command,
		Env:     make(map[string]string),
	}
}

// NewMCPProviderFromJSON creates an MCPProvider from JSON configuration.
func NewMCPProviderFromJSON(data []byte) (*MCPProvider, error) {
	var provider MCPProvider
	if err := json.Unmarshal(data, &provider); err != nil {
		return nil, err
	}
	if provider.Env == nil {
		provider.Env = make(map[string]string)
	}
	return &provider, nil
}

// Type returns the provider type.
func (p *MCPProvider) Type() ProviderType {
	return ProviderType("mcp")
}

// GetName returns the provider's name.
func (p *MCPProvider) GetName() string {
	return p.Name
}

// WithArgs sets command line arguments for the MCP server process.
func (p *MCPProvider) WithArgs(args ...string) *MCPProvider {
	p.Args = args
	return p
}

// WithEnv sets environment variables for the MCP server process.
func (p *MCPProvider) WithEnv(key, value string) *MCPProvider {
	if p.Env == nil {
		p.Env = make(map[string]string)
	}
	p.Env[key] = value
	return p
}

// WithWorkingDir sets the working directory for the MCP server process.
func (p *MCPProvider) WithWorkingDir(dir string) *MCPProvider {
	p.WorkingDir = dir
	return p
}

// WithStdinData sets data to be sent to the MCP server's stdin on startup.
func (p *MCPProvider) WithStdinData(data string) *MCPProvider {
	p.StdinData = data
	return p
}

// WithTimeout sets the timeout for MCP operations in seconds.
func (p *MCPProvider) WithTimeout(seconds int) *MCPProvider {
	p.Timeout = seconds
	return p
}

// Validate ensures the provider configuration is valid.
func (p *MCPProvider) Validate() error {
	if p.Name == "" {
		return errors.New("MCP provider name cannot be empty")
	}
	if len(p.Command) == 0 {
		return errors.New("MCP provider command cannot be empty")
	}
	return nil
}
