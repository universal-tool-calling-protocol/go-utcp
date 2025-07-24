package utcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	mcpprov "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// MCPServerTransport communicates with a remote MCP server over HTTP.
type MCPServerTransport struct {
	client *http.Client
	logger func(format string, args ...interface{})
}

// NewMCPServerTransport creates a new transport instance.
func NewMCPServerTransport(logger func(format string, args ...interface{})) *MCPServerTransport {
	if logger == nil {
		logger = func(string, ...interface{}) {}
	}
	return &MCPServerTransport{
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

func (t *MCPServerTransport) logf(format string, args ...interface{}) {
	if t.logger != nil {
		t.logger(format, args...)
	}
}

// RegisterToolProvider fetches the list of tools from the remote MCP server.
func (t *MCPServerTransport) RegisterToolProvider(ctx context.Context, p Provider) ([]Tool, error) {
	prov, ok := p.(*mcpprov.MCPServerProvider)
	if !ok {
		return nil, errors.New("MCPServerTransport can only be used with MCPServerProvider")
	}
	req := map[string]any{"jsonrpc": "2.0", "id": 0, "method": "tools/list"}
	var result map[string]any
	if err := t.send(ctx, prov, req, &result); err != nil {
		return nil, err
	}
	toolsIface, _ := result["tools"].([]interface{})
	var tools []Tool
	for _, ti := range toolsIface {
		if tm, ok := ti.(map[string]interface{}); ok {
			tool := Tool{}
			if n, ok := tm["name"].(string); ok {
				tool.Name = n
			}
			if d, ok := tm["description"].(string); ok {
				tool.Description = d
			}
			if inp, ok := tm["input_schema"].(map[string]interface{}); ok {
				tool.Inputs = ToolInputOutputSchema{Type: "object", Properties: inp}
			}
			tools = append(tools, tool)
		}
	}
	return tools, nil
}

// DeregisterToolProvider is a no-op for HTTP transport.
func (t *MCPServerTransport) DeregisterToolProvider(ctx context.Context, p Provider) error {
	if _, ok := p.(*mcpprov.MCPServerProvider); !ok {
		return errors.New("MCPServerTransport can only be used with MCPServerProvider")
	}
	return nil
}

// CallTool invokes a tool on the remote server.
func (t *MCPServerTransport) CallTool(ctx context.Context, toolName string, args map[string]any, p Provider, _ *string) (any, error) {
	prov, ok := p.(*mcpprov.MCPServerProvider)
	if !ok {
		return nil, errors.New("MCPServerTransport can only be used with MCPServerProvider")
	}
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	var result map[string]any
	if err := t.send(ctx, prov, req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// send POSTs the JSON-RPC request and decodes the response into out.
func (t *MCPServerTransport) send(ctx context.Context, prov *mcpprov.MCPServerProvider, payload any, out any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, prov.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range prov.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s", string(b))
	}
	return json.NewDecoder(resp.Body).Decode(&out)
}
