package UTCP

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

// SSEClientTransport implements Server-Sent Events over HTTP for UTCP tools.
type SSEClientTransport struct {
	client *http.Client
	logger func(format string, args ...interface{})
}

// decodeToolsResponse parses a common tools discovery response.
func decodeToolsResponse(r io.ReadCloser) ([]Tool, error) {
	defer r.Close()
	var resp struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		return nil, err
	}
	return resp.Tools, nil
}

// NewSSETransport constructs a new SSEClientTransport.
func NewSSETransport(logger func(format string, args ...interface{})) *SSEClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &SSEClientTransport{
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

// RegisterToolProvider registers an SSE-based provider by fetching its tool list.
func (t *SSEClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	sseProv, ok := prov.(*SSEProvider)
	if !ok {
		return nil, errors.New("SSEClientTransport can only be used with SSEProvider")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sseProv.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range sseProv.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	return decodeToolsResponse(resp.Body)
}

// DeregisterToolProvider cleans up any resources (no-op for SSE).
func (t *SSEClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	sseProv, ok := prov.(*SSEProvider)
	if !ok {
		return errors.New("SSEClientTransport can only be used with SSEProvider")
	}

	t.logger("Deregistered SSE provider '%s'", sseProv.Name)
	return nil
}

// CallTool invokes a named tool over SSE by POSTing inputs and decoding JSON output.
func (t *SSEClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov Provider, l *string) (interface{}, error) {
	sseProv, ok := prov.(*SSEProvider)
	if !ok {
		return nil, errors.New("SSEClientTransport can only be used with SSEProvider")
	}
	url := fmt.Sprintf("%s/%s", sseProv.URL, toolName)
	payload := args
	if sseProv.BodyField != nil {
		payload = map[string]interface{}{*sseProv.BodyField: args}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range sseProv.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// StreamableHTTPClientTransport implements HTTP with streaming support.
type StreamableHTTPClientTransport struct {
	client *http.Client
	logger func(format string, args ...interface{})
}

// NewStreamableHTTPTransport constructs a new StreamableHTTPClientTransport.
func NewStreamableHTTPTransport(logger func(format string, args ...interface{})) *StreamableHTTPClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &StreamableHTTPClientTransport{
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

// RegisterToolProvider registers an HTTP streaming provider by fetching its tool list.
func (t *StreamableHTTPClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	streamProv, ok := prov.(*StreamableHttpProvider)
	if !ok {
		return nil, errors.New("StreamableHTTPClientTransport can only be used with StreamableHttpProvider")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamProv.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range streamProv.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	return decodeToolsResponse(resp.Body)
}

// CallTool invokes a named tool via HTTP POST for streaming providers.
func (t *StreamableHTTPClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov Provider, l *string) (interface{}, error) {
	streamProv, ok := prov.(*StreamableHttpProvider)
	if !ok {
		return nil, errors.New("StreamableHTTPClientTransport can only be used with StreamableHttpProvider")
	}
	url := fmt.Sprintf("%s/%s", streamProv.URL, toolName)
	data, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range streamProv.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeregisterToolProvider clears any streaming-specific state (no-op).
func (t *StreamableHTTPClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	// No persistent state to clean up
	return nil
}

// MCPClientTransport implements a custom MCP protocol transport.
type MCPClientTransport struct {
	client *http.Client
	logger func(format string, args ...interface{})
}

// NewMCPTransport constructs a new MCPClientTransport.
func NewMCPTransport(logger func(format string, args ...interface{})) *MCPClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &MCPClientTransport{
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

// RegisterToolProvider for MCP logs registration; discovery via MCP protocol not implemented.
func (t *MCPClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	mcpProv, ok := prov.(*MCPProvider)
	if !ok {
		return nil, errors.New("MCPClientTransport can only be used with MCPProvider")
	}
	t.logger("Registered MCP provider '%s'", mcpProv.Name)
	// TODO: perform MCP discovery based on mcpProv.Config
	return nil, nil
}

// CallTool invokes a named tool over MCP (not implemented).
func (t *MCPClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov Provider, l *string) (interface{}, error) {
	mcpProv, ok := prov.(*MCPProvider)
	if !ok {
		return nil, errors.New("MCPClientTransport can only be used with MCPProvider")
	}
	t.logger("Calling MCP tool '%s' on provider '%s'", toolName, mcpProv.Name)
	// TODO: implement MCP protocol invocation logic
	return nil, errors.New("MCP transport invocation not implemented yet")
}

// DeregisterToolProvider cleans up any resources for MCP.
func (t *MCPClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	mcpProv, ok := prov.(*MCPProvider)
	if !ok {
		return errors.New("MCPClientTransport can only be used with MCPProvider")
	}
	t.logger("Deregistered MCP provider '%s'", mcpProv.Name)
	return nil
}

// TextClientTransport is a simple in-memory/text-based transport.
type TextClientTransport struct {
	prefix string
	tools  map[string]Tool
}

// NewTextTransport constructs a TextClientTransport.
func NewTextTransport(prefix string) *TextClientTransport {
	return &TextClientTransport{
		prefix: prefix,
		tools:  make(map[string]Tool),
	}
}

// RegisterToolProvider loads tool definitions from a local text (JSON) file.
func (t *TextClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	textProv, ok := prov.(*TextProvider)
	if !ok {
		return nil, errors.New("TextClientTransport can only be used with TextProvider")
	}
	data, err := ioutil.ReadFile(textProv.FilePath)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	manual := NewUtcpManualFromMap(raw)
	t.tools = make(map[string]Tool, len(manual.Tools))
	for _, tool := range manual.Tools {
		t.tools[tool.Name] = tool
	}
	return manual.Tools, nil
}

// DeregisterToolProvider cleans up in-memory tools.
func (t *TextClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	t.tools = make(map[string]Tool)
	return nil
}

// CallTool invokes a named in-memory tool handler.
func (t *TextClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov Provider, l *string) (interface{}, error) {
	tool, ok := t.tools[toolName]
	if !ok {
		return nil, fmt.Errorf("tool %q not registered", toolName)
	}
	return tool.Handler(nil, args)
}
