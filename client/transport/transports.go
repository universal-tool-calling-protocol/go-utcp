package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"server"
	"strings"
	"time"
)

// SSEClientTransport implements Server-Sent Events over HTTP for UTCP tools.
type SSEClientTransport struct {
	client *http.Client
	logger func(format string, args ...interface{})
}

// decodeToolsResponse parses a common tools discovery response.
func decodeToolsResponse(r io.ReadCloser) ([]server.Tool, error) {
	defer r.Close()
	var resp struct {
		Tools []server.Tool `json:"tools"`
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

// RegisterToolProvider registers an SSE-based provider.
func (t *SSEClientTransport) RegisterToolProvider(ctx context.Context, prov server.Provider) ([]server.Tool, error) {
	sseProv, ok := prov.(*server.SSEProvider)
	if !ok {
		return nil, errors.New("SSETransport can only be used with SSEProvider")
	}

	// Create discovery endpoint URL
	discoveryURL := fmt.Sprintf("%s/tools", strings.TrimSuffix(sseProv.BaseURL, "/"))

	req, err := http.NewRequestWithContext(ctx, "GET", discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if sseProv.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+sseProv.APIKey)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to discover tools: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("discovery failed with status %d", resp.StatusCode)
	}

	tools, err := decodeToolsResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tools response: %w", err)
	}

	t.logger("Discovered %d tools from SSE provider '%s'", len(tools), sseProv.Name)
	return tools, nil
}

// DeregisterToolProvider cleans up any resources (no-op for SSE).
func (t *SSEClientTransport) DeregisterToolProvider(ctx context.Context, prov server.Provider) error {
	sseProv, ok := prov.(*server.SSEProvider)
	if !ok {
		return errors.New("SSETransport can only be used with SSEProvider")
	}

	t.logger("Deregistered SSE provider '%s'", sseProv.Name)
	return nil
}

// CallTool invokes a named tool over SSE.
func (t *SSEClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov server.Provider, l *string) (interface{}, error) {
	sseProv, ok := prov.(*server.SSEProvider)
	if !ok {
		return nil, errors.New("SSETransport can only be used with SSEProvider")
	}

	// Create tool invocation URL
	toolURL := fmt.Sprintf("%s/tools/%s/invoke", strings.TrimSuffix(sseProv.BaseURL, "/"), url.PathEscape(toolName))

	reqBody, err := json.Marshal(map[string]interface{}{
		"arguments": args,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", toolURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create tool request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if sseProv.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+sseProv.APIKey)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke tool: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tool invocation failed with status %d", resp.StatusCode)
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	var result strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var eventData map[string]interface{}
			if err := json.Unmarshal([]byte(data), &eventData); err == nil {
				if content, ok := eventData["content"].(string); ok {
					result.WriteString(content)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading SSE stream: %w", err)
	}

	t.logger("SSE tool '%s' executed successfully", toolName)
	return result.String(), nil
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

// RegisterToolProvider registers an HTTP streaming provider.
func (t *StreamableHTTPClientTransport) RegisterToolProvider(ctx context.Context, prov server.Provider) ([]server.Tool, error) {
	hp, ok := prov.(*server.HttpProvider)
	if !ok {
		return nil, errors.New("StreamableHTTPTransport can only be used with HttpProvider")
	}

	// Discover tools via HTTP endpoint
	discoveryURL := fmt.Sprintf("%s/tools", strings.TrimSuffix(hp.BaseURL, "/"))

	req, err := http.NewRequestWithContext(ctx, "GET", discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if hp.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+hp.APIKey)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to discover tools: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("discovery failed with status %d", resp.StatusCode)
	}

	tools, err := decodeToolsResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tools response: %w", err)
	}

	// Mark tools as streaming-capable
	for i := range tools {
		tools[i].Streaming = true
	}

	t.logger("Discovered %d streaming tools from HTTP provider '%s'", len(tools), hp.Name)
	return tools, nil
}

// DeregisterToolProvider cleans up streaming HTTP resources.
func (t *StreamableHTTPClientTransport) DeregisterToolProvider(ctx context.Context, prov server.Provider) error {
	hp, ok := prov.(*server.HttpProvider)
	if !ok {
		return errors.New("StreamableHTTPTransport can only be used with HttpProvider")
	}

	t.logger("Deregistered HTTP streaming provider '%s'", hp.Name)
	return nil
}

// CallTool invokes a tool and returns a stream (io.Reader) or aggregated result.
func (t *StreamableHTTPClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov server.Provider, l *string) (interface{}, error) {
	hp, ok := prov.(*server.HttpProvider)
	if !ok {
		return nil, errors.New("StreamableHTTPTransport can only be used with HttpProvider")
	}

	// Create tool invocation URL
	toolURL := fmt.Sprintf("%s/tools/%s/invoke", strings.TrimSuffix(hp.BaseURL, "/"), url.PathEscape(toolName))

	reqBody, err := json.Marshal(map[string]interface{}{
		"arguments": args,
		"stream":    true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", toolURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create tool request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if hp.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+hp.APIKey)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke tool: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("tool invocation failed with status %d", resp.StatusCode)
	}

	// Check if response is streaming
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "stream") {
		// Return the response body as a stream
		t.logger("HTTP streaming tool '%s' invoked, returning stream", toolName)
		return resp.Body, nil
	}

	// Non-streaming response, read and return as JSON
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	t.logger("HTTP tool '%s' executed successfully", toolName)
	return result, nil
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

func (t *MCPClientTransport) RegisterToolProvider(ctx context.Context, prov server.Provider) ([]server.Tool, error) {
	mcpProv, ok := prov.(*server.MCPProvider)
	if !ok {
		return nil, errors.New("MCPTransport can only be used with MCPProvider")
	}

	// MCP discovery protocol
	discoveryReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	reqBody, err := json.Marshal(discoveryReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP discovery request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", mcpProv.Endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP discovery request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if mcpProv.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+mcpProv.APIKey)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to discover MCP tools: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP discovery failed with status %d", resp.StatusCode)
	}

	var mcpResp struct {
		JSONRPC string `json:"jsonrpc"`
		ID      string `json:"id"`
		Result  struct {
			Tools []server.Tool `json:"tools"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		return nil, fmt.Errorf("failed to decode MCP response: %w", err)
	}

	if mcpResp.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", mcpResp.Error.Code, mcpResp.Error.Message)
	}

	t.logger("Discovered %d tools from MCP provider '%s'", len(mcpResp.Result.Tools), mcpProv.Name)
	return mcpResp.Result.Tools, nil
}

func (t *MCPClientTransport) DeregisterToolProvider(ctx context.Context, prov server.Provider) error {
	mcpProv, ok := prov.(*server.MCPProvider)
	if !ok {
		return errors.New("MCPTransport can only be used with MCPProvider")
	}

	t.logger("Deregistered MCP provider '%s'", mcpProv.Name)
	return nil
}

func (t *MCPClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov server.Provider, l *string) (interface{}, error) {
	mcpProv, ok := prov.(*server.MCPProvider)
	if !ok {
		return nil, errors.New("MCPTransport can only be used with MCPProvider")
	}

	// MCP tool invocation protocol
	toolReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
	}

	reqBody, err := json.Marshal(toolReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP tool request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", mcpProv.Endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP tool request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if mcpProv.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+mcpProv.APIKey)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke MCP tool: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP tool invocation failed with status %d", resp.StatusCode)
	}

	var mcpResp struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      string      `json:"id"`
		Result  interface{} `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		return nil, fmt.Errorf("failed to decode MCP response: %w", err)
	}

	if mcpResp.Error != nil {
		return nil, fmt.Errorf("MCP tool error %d: %s", mcpResp.Error.Code, mcpResp.Error.Message)
	}

	t.logger("MCP tool '%s' executed successfully", toolName)
	return mcpResp.Result, nil
}

// TextClientTransport is a simple in-memory/text-based transport.
type TextClientTransport struct {
	prefix string
	tools  map[string]server.Tool
}

// NewTextTransport constructs a TextClientTransport.
func NewTextTransport(prefix string) *TextClientTransport {
	return &TextClientTransport{
		prefix: prefix,
		tools:  make(map[string]server.Tool),
	}
}

func (t *TextClientTransport) RegisterToolProvider(ctx context.Context, prov server.Provider) ([]server.Tool, error) {
	textProv, ok := prov.(*server.TextProvider)
	if !ok {
		return nil, errors.New("TextTransport can only be used with TextProvider")
	}

	// For text transport, tools are typically predefined
	tools := []server.Tool{
		{
			Name:        "echo",
			Description: "Echo the input text",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"text": map[string]interface{}{
						"type":        "string",
						"description": "Text to echo",
					},
				},
				"required": []string{"text"},
			},
		},
		{
			Name:        "format",
			Description: "Format text with prefix",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"text": map[string]interface{}{
						"type":        "string",
						"description": "Text to format",
					},
					"style": map[string]interface{}{
						"type":        "string",
						"description": "Formatting style (upper, lower, title)",
						"enum":        []string{"upper", "lower", "title"},
					},
				},
				"required": []string{"text"},
			},
		},
	}

	// Store tools for later reference
	for _, tool := range tools {
		t.tools[tool.Name] = tool
	}

	return tools, nil
}

func (t *TextClientTransport) DeregisterToolProvider(ctx context.Context, prov server.Provider) error {
	// Clear stored tools
	t.tools = make(map[string]server.Tool)
	return nil
}

func (t *TextClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov server.Provider, l *string) (interface{}, error) {
	tool, exists := t.tools[toolName]
	if !exists {
		return nil, fmt.Errorf("tool '%s' not found", toolName)
	}

	switch toolName {
	case "echo":
		text, ok := args["text"].(string)
		if !ok {
			return nil, errors.New("echo tool requires 'text' parameter")
		}
		return fmt.Sprintf("%s: %s", t.prefix, text), nil

	case "format":
		text, ok := args["text"].(string)
		if !ok {
			return nil, errors.New("format tool requires 'text' parameter")
		}

		style, _ := args["style"].(string)
		if style == "" {
			style = "title"
		}

		var formatted string
		switch style {
		case "upper":
			formatted = strings.ToUpper(text)
		case "lower":
			formatted = strings.ToLower(text)
		case "title":
			formatted = strings.Title(text)
		default:
			formatted = text
		}

		return fmt.Sprintf("%s: %s", t.prefix, formatted), nil

	default:
		return fmt.Sprintf("%s: called %s with args %v", t.prefix, toolName, args), nil
	}
}
