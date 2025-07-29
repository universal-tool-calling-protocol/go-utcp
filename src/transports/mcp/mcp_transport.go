package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpapi "github.com/mark3labs/mcp-go/mcp"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// MCPTransport implements ClientTransportInterface for MCP (Model Context Protocol) providers.
type MCPTransport struct {
	processes map[string]*mcpProcess
	mutex     sync.RWMutex
	logger    func(format string, args ...interface{})
}

// mcpProcess represents a running MCP server process.
type mcpProcess struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	httpClient *mcpclient.Client
	tools      []Tool
	mutex      sync.RWMutex
}

// mcpRequest represents an MCP JSON-RPC request.
type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// mcpResponse represents an MCP JSON-RPC response.
type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

type mcpError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// mcpNotification represents an MCP JSON-RPC notification.
type mcpNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// NewMCPTransport constructs a new MCPTransport.
func NewMCPTransport(logger func(format string, args ...interface{})) *MCPTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &MCPTransport{
		processes: make(map[string]*mcpProcess),
		logger:    logger,
	}
}

// RegisterToolProvider starts an MCP server process and discovers its tools.
func (t *MCPTransport) RegisterToolProvider(ctx context.Context, p Provider) ([]Tool, error) {
	mp, ok := p.(*MCPProvider)
	if !ok {
		return nil, errors.New("MCPTransport can only be used with MCPProvider")
	}

	if err := mp.Validate(); err != nil {
		return nil, fmt.Errorf("invalid MCP provider configuration: %w", err)
	}

	// Check if process already exists
	t.mutex.RLock()
	if proc, exists := t.processes[mp.Name]; exists {
		t.mutex.RUnlock()
		return proc.tools, nil
	}
	t.mutex.RUnlock()

	if mp.URL != "" {
		// Use HTTP client via mcp-go
		cli, err := mcpclient.NewStreamableHttpClient(mp.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to create MCP HTTP client: %w", err)
		}
		if err := cli.Start(ctx); err != nil {
			return nil, fmt.Errorf("failed to start MCP HTTP client: %w", err)
		}
		initReq := mcpapi.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcpapi.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcpapi.Implementation{Name: "utcp", Version: "1.0.0"}
		if _, err := cli.Initialize(ctx, initReq); err != nil {
			cli.Close()
			return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
		}
		toolsRes, err := cli.ListTools(ctx, mcpapi.ListToolsRequest{})
		if err != nil {
			cli.Close()
			return nil, fmt.Errorf("failed to list tools: %w", err)
		}
		tools := make([]Tool, len(toolsRes.Tools))
		for i, tl := range toolsRes.Tools {
			tools[i] = Tool{
				Name:        tl.Name,
				Description: tl.Description,
				Inputs: ToolInputOutputSchema{
					Type:       tl.InputSchema.Type,
					Properties: tl.InputSchema.Properties,
					Required:   tl.InputSchema.Required,
				},
			}
		}
		process := &mcpProcess{httpClient: cli, tools: tools}
		t.mutex.Lock()
		t.processes[mp.Name] = process
		t.mutex.Unlock()
		t.logger("Successfully registered MCP HTTP provider '%s'", mp.Name)
		return tools, nil
	}

	t.logger("Starting MCP server '%s' with command: %v", mp.Name, mp.Command)

	// Start the MCP server process
	cmd := exec.CommandContext(ctx, mp.Command[0], mp.Command[1:]...)

	// Set environment variables
	cmd.Env = os.Environ()
	for key, value := range mp.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	if mp.WorkingDir != "" {
		cmd.Dir = mp.WorkingDir
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	process := &mcpProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}

	// Send initial stdin data if provided
	if mp.StdinData != "" {
		if _, err := stdin.Write([]byte(mp.StdinData)); err != nil {
			t.logger("Warning: failed to write stdin data: %v", err)
		}
	}

	// Initialize MCP connection
	if err := t.initializeMCPConnection(ctx, process, mp); err != nil {
		t.cleanupProcess(process)
		return nil, fmt.Errorf("failed to initialize MCP connection: %w", err)
	}

	// Store the process
	t.mutex.Lock()
	t.processes[mp.Name] = process
	t.mutex.Unlock()

	t.logger("Successfully registered MCP provider '%s' with %d tools", mp.Name, len(process.tools))
	return process.tools, nil
}

// DeregisterToolProvider stops and cleans up an MCP server process.
func (t *MCPTransport) DeregisterToolProvider(ctx context.Context, p Provider) error {
	mp, ok := p.(*MCPProvider)
	if !ok {
		return errors.New("MCPTransport can only be used with MCPProvider")
	}

	t.mutex.Lock()
	process, exists := t.processes[mp.Name]
	if exists {
		delete(t.processes, mp.Name)
	}
	t.mutex.Unlock()

	if !exists {
		return nil
	}

	t.logger("Deregistering MCP provider '%s'", mp.Name)
	t.cleanupProcess(process)
	return nil
}

// CallTool invokes a tool: returns a []any for non‑streaming calls or a StreamResult when the tool streams.
// or transports.ChannelStreamResult for streaming calls.
func (t *MCPTransport) CallTool(
	ctx context.Context,
	toolName string,
	args map[string]any,
	p Provider,
	_l *string,
) (interface{}, error) {
	mp, ok := p.(*MCPProvider)
	if !ok {
		return nil, errors.New("MCPTransport can only be used with MCPProvider")
	}

	// Lookup the process for this provider
	t.mutex.RLock()
	proc, exists := t.processes[mp.Name]
	t.mutex.RUnlock()
	if !exists {
		return nil, fmt.Errorf("provider '%s' not registered", mp.Name)
	}

	// Dispatch based on tool capabilities
	var res interface{}
	var err error

	switch {
	case proc.httpClient != nil:
		// HTTP‑capable synchronous tools
		res, err = t.callHTTPTool(ctx, proc.httpClient, toolName, args)

	default:
		// StdIO blocking tools
		res, err = t.callStdioTool(ctx, proc, toolName, args, mp.Timeout)
	}
	if err != nil {
		return nil, err
	}

	return res, nil
}

// callStdioTool runs a stdio‐backed tool to completion, skipping
// any JSON‐RPC notifications and returning the first real result.
func (t *MCPTransport) callStdioTool(
	ctx context.Context,
	process *mcpProcess,
	toolName string,
	args map[string]any,
	timeoutSeconds int,
) (interface{}, error) {
	// Spawn the stdio stream (reuse your existing helper)
	ch, err := t.callStdioToolStream(ctx, process, toolName, args, timeoutSeconds)
	if err != nil {
		return nil, err
	}

	// Drain until we find a non‐notification message
	for item := range ch {
		if m, ok := item.(map[string]any); ok {
			if typ, hasType := m["type"]; hasType && typ == "notification" {
				// skip JSON‐RPC notifications
				continue
			}
			// it's the final result map
			return m, nil
		}
		// primitive or other payload → wrap for consistency
		return map[string]any{"result": item}, nil
	}

	return nil, fmt.Errorf("no output from tool %q", toolName)
}

func (t *MCPTransport) CallToolStream(
	ctx context.Context,
	toolName string,
	args map[string]any,
	p Provider,
) (transports.StreamResult, error) {
	stream, err := t.CallingToolStream(ctx, toolName, args, p)
	if err != nil {
		return nil, err
	}

	// Read first element (or bail on EOF)
	first, err := stream.Next()
	if err != nil {
		stream.Close()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("no output from tool %s", toolName)
		}
		return nil, err
	}

	ch := make(chan any, 1)
	go func() {
		defer func() {
			stream.Close()
			close(ch)
		}()

		// helper to flatten one message
		flattenAndSend := func(msg any) {
			m, ok := msg.(map[string]any)
			if !ok {
				ch <- msg
				return
			}
			cont, ok := m["content"].([]any)
			if !ok {
				ch <- msg
				return
			}
			for _, part := range cont {
				pm, ok := part.(map[string]any)
				if !ok {
					continue
				}
				txt, ok := pm["text"].(string)
				if !ok {
					continue
				}
				// try to parse as JSON array of strings
				var items []string
				if err := json.Unmarshal([]byte(txt), &items); err == nil {
					for _, item := range items {
						ch <- item
					}
					continue
				}
				// not a JSON list → just emit the raw text
				ch <- txt
			}
		}

		// send the first message (flattened)
		flattenAndSend(first)

		// now stream the rest
		for {
			item, err := stream.Next()
			if err != nil {
				return
			}
			flattenAndSend(item)
		}
	}()

	return transports.NewChannelStreamResult(ch, stream.Close), nil
}

// CallingToolStream returns a transports.StreamResult for live streaming.
func (t *MCPTransport) CallingToolStream(
	ctx context.Context,
	toolName string,
	args map[string]any,
	p Provider,
) (transports.StreamResult, error) {
	streamCtx, cancelFn := context.WithCancel(ctx)
	mp, ok := p.(*MCPProvider)
	if !ok {
		cancelFn()
		return nil, errors.New("MCPTransport can only be used with MCPProvider")
	}
	t.mutex.RLock()
	proc, exists := t.processes[mp.Name]
	t.mutex.RUnlock()
	if !exists {
		cancelFn()
		return nil, fmt.Errorf("provider '%s' not registered", mp.Name)
	}

	t.logger("Calling MCP tool '%s' on provider '%s'", toolName, mp.Name)

	var ch <-chan any
	var err error
	if proc.httpClient != nil {
		ch, err = t.callHTTPToolStream(streamCtx, proc.httpClient, toolName, args)
	} else {
		ch, err = t.callStdioToolStream(streamCtx, proc, toolName, args, mp.Timeout)
	}
	if err != nil {
		cancelFn()
		return nil, err
	}

	return transports.NewChannelStreamResult(ch, defaultClose(cancelFn)), nil
}

// defaultClose wraps a context.CancelFunc into a func() error for StreamResult closing.
func defaultClose(cancel context.CancelFunc) func() error {
	return func() error {
		cancel()
		return nil
	}
}

// callHTTPToolStream handles tool calls via HTTP client with streaming support.
func (t *MCPTransport) callHTTPToolStream(
	ctx context.Context,
	client *mcpclient.Client,
	toolName string,
	args map[string]any,
) (<-chan any, error) {
	ch := make(chan any, 10)
	done := make(chan struct{})
	notificationReceived := make(chan struct{}, 1)

	// 1) Register handler for real JSON‑RPC notifications
	client.OnNotification(func(n mcp.JSONRPCNotification) {
		select {
		case <-done:
			return
		default:
		}
		payload := map[string]any{
			"type":   "notification",
			"method": n.Method,
			"params": n.Params,
		}
		// signal that we saw at least one
		select {
		case notificationReceived <- struct{}{}:
		default:
		}
		select {
		case ch <- payload:
		case <-ctx.Done():
		}
	})

	go func() {
		defer func() {
			close(done)
			close(ch)
		}()

		// 2) Call the tool (sync or streaming)
		req := mcpapi.CallToolRequest{
			Params: mcpapi.CallToolParams{
				Name:      toolName,
				Arguments: args,
			},
		}
		res, err := client.CallTool(ctx, req)
		if err != nil {
			select {
			case ch <- err:
			case <-ctx.Done():
			}
			return
		}

		// 3) Brief window for real streaming
		select {
		case <-notificationReceived:
			t.logger("Streaming notifications detected for tool '%s'", toolName)
			return
		case <-time.After(150 * time.Millisecond):
			// no notifications → fallback to sync handling
		}

		// 4) Marshal/unmarshal to inspect fields
		raw, _ := json.Marshal(res)
		var respMap map[string]any
		_ = json.Unmarshal(raw, &respMap)

		// Fallback: emit the result object if present, otherwise the entire map
		final := any(respMap)
		if resultObj, ok := respMap["result"].(map[string]any); ok {
			final = resultObj
		}
		select {
		case ch <- final:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// callStdioToolStream handles tool calls via stdio process with streaming support.
func (t *MCPTransport) callStdioToolStream(ctx context.Context, process *mcpProcess, toolName string, args map[string]any, timeoutSeconds int) (<-chan any, error) {
	// Set default timeout if not specified
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	// Prepare request
	request := mcpRequest{
		JSONRPC: "2.0",
		ID:      t.generateRequestID(),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
	}

	// Serialize request
	reqData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	process.mutex.Lock()
	if _, err := process.stdin.Write(append(reqData, '\n')); err != nil {
		process.mutex.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	process.mutex.Unlock()

	// Create result channel and start reader goroutine
	resultChan := make(chan any, 10)

	go func() {
		defer close(resultChan)

		// Create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		scanner := bufio.NewScanner(process.stdout)
		responseReceived := false
		notificationCount := 0

		for {
			select {
			case <-timeoutCtx.Done():
				if timeoutCtx.Err() == context.DeadlineExceeded && !responseReceived {
					resultChan <- fmt.Errorf("request timeout after %d seconds", timeoutSeconds)
				}
				return
			default:
				if !scanner.Scan() {
					if err := scanner.Err(); err != nil && !responseReceived {
						resultChan <- fmt.Errorf("failed to read response: %w", err)
					}
					return
				}

				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}

				// Try to parse as notification first
				var notification mcpNotification
				if err := json.Unmarshal([]byte(line), &notification); err == nil {
					// Check if it's a notification (no ID field)
					var hasID bool
					var tempMap map[string]interface{}
					if json.Unmarshal([]byte(line), &tempMap) == nil {
						_, hasID = tempMap["id"]
					}

					if !hasID && notification.Method != "" {
						notificationCount++
						// Create a structured notification result
						notificationResult := map[string]interface{}{
							"type":   "notification",
							"method": notification.Method,
							"params": notification.Params,
						}

						t.logger("Received notification %d for tool '%s': %s", notificationCount, toolName, notification.Method)

						select {
						case resultChan <- notificationResult:
						case <-ctx.Done():
							return
						}

						// Reset timeout for more notifications/response
						timeoutCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
						defer cancel()
						continue
					}
				}

				// Try to parse as response
				var response mcpResponse
				if err := json.Unmarshal([]byte(line), &response); err != nil {
					t.logger("Failed to parse MCP message: %v", err)
					continue
				}

				// Check if this response matches our request
				if response.ID == request.ID {
					responseReceived = true

					if response.Error != nil {
						resultChan <- fmt.Errorf("MCP error %d: %s", response.Error.Code, response.Error.Message)
						return
					}

					// Send the final response
					t.logger("Received final response for tool '%s' after %d notifications", toolName, notificationCount)
					select {
					case resultChan <- response.Result:
					case <-ctx.Done():
						return
					}

					// For tool calls, we're done after receiving the response
					return
				}
			}
		}
	}()

	return resultChan, nil
}

// initializeMCPConnection establishes the MCP connection and discovers tools.
func (t *MCPTransport) initializeMCPConnection(ctx context.Context, process *mcpProcess, mp *MCPProvider) error {
	// Send initialize request
	initRequest := mcpRequest{
		JSONRPC: "2.0",
		ID:      0,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"clientInfo": map[string]interface{}{
				"name":    "utcp",
				"version": "1.0.0",
			},
		},
	}

	if _, err := t.sendMCPRequestBlocking(ctx, process, initRequest, mp.Timeout); err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	// Send tools/list request to discover available tools
	toolsRequest := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	result, err := t.sendMCPRequestBlocking(ctx, process, toolsRequest, mp.Timeout)
	if err != nil {
		return fmt.Errorf("tools/list failed: %w", err)
	}

	// Parse tools from response
	if err := t.parseToolsResponse(process, result); err != nil {
		return fmt.Errorf("failed to parse tools response: %w", err)
	}

	return nil
}

// sendMCPRequestBlocking sends a JSON-RPC request and waits for a single response (used for initialization).
func (t *MCPTransport) sendMCPRequestBlocking(ctx context.Context, process *mcpProcess, request mcpRequest, timeoutSeconds int) (interface{}, error) {
	resultChan, err := t.callStdioToolStreamInternal(ctx, process, request, timeoutSeconds)
	if err != nil {
		return nil, err
	}

	// Get the first (and only expected) result
	select {
	case result := <-resultChan:
		if err, ok := result.(error); ok {
			return nil, err
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// callStdioToolStreamInternal is a helper that accepts a raw mcpRequest (used for initialization calls).
func (t *MCPTransport) callStdioToolStreamInternal(ctx context.Context, process *mcpProcess, request mcpRequest, timeoutSeconds int) (<-chan any, error) {
	// Set default timeout if not specified
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	// Serialize request
	reqData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	process.mutex.Lock()
	if _, err := process.stdin.Write(append(reqData, '\n')); err != nil {
		process.mutex.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	process.mutex.Unlock()

	// Create result channel and start reader goroutine
	resultChan := make(chan any)

	go func() {
		defer close(resultChan)

		// Create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		scanner := bufio.NewScanner(process.stdout)

		for {
			select {
			case <-timeoutCtx.Done():
				if timeoutCtx.Err() == context.DeadlineExceeded {
					resultChan <- fmt.Errorf("request timeout after %d seconds", timeoutSeconds)
				}
				return
			default:
				if !scanner.Scan() {
					if err := scanner.Err(); err != nil {
						resultChan <- fmt.Errorf("failed to read response: %w", err)
					}
					return
				}

				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}

				var response mcpResponse
				if err := json.Unmarshal([]byte(line), &response); err != nil {
					t.logger("Failed to parse MCP response: %v", err)
					continue
				}

				// Check if this response matches our request
				if response.ID == request.ID {
					if response.Error != nil {
						resultChan <- fmt.Errorf("MCP error %d: %s", response.Error.Code, response.Error.Message)
					} else {
						resultChan <- response.Result
					}
					return
				}
			}
		}
	}()

	return resultChan, nil
}

// parseToolsResponse parses the tools/list response and populates the process tools.
func (t *MCPTransport) parseToolsResponse(process *mcpProcess, result interface{}) error {
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return errors.New("invalid tools response format")
	}

	toolsInterface, exists := resultMap["tools"]
	if !exists {
		process.tools = []Tool{}
		return nil
	}

	toolsList, ok := toolsInterface.([]interface{})
	if !ok {
		return errors.New("tools field is not an array")
	}

	var tools []Tool
	for _, toolInterface := range toolsList {
		toolMap, ok := toolInterface.(map[string]interface{})
		if !ok {
			continue
		}

		tool := Tool{}

		if name, ok := toolMap["name"].(string); ok {
			tool.Name = name
		}

		if description, ok := toolMap["description"].(string); ok {
			tool.Description = description
		}

		// Parse input schema if present
		if inputSchema, ok := toolMap["inputSchema"].(ToolInputOutputSchema); ok {
			tool.Inputs = inputSchema
		}

		tools = append(tools, tool)
	}

	process.tools = tools
	return nil
}

// cleanupProcess terminates and cleans up an MCP server process.
func (t *MCPTransport) cleanupProcess(process *mcpProcess) {
	if process.stdin != nil {
		process.stdin.Close()
	}
	if process.stdout != nil {
		process.stdout.Close()
	}
	if process.stderr != nil {
		process.stderr.Close()
	}
	if process.cmd != nil && process.cmd.Process != nil {
		process.cmd.Process.Kill()
		process.cmd.Wait()
	}
	if process.httpClient != nil {
		process.httpClient.Close()
	}
}

// generateRequestID generates a unique request ID.
func (t *MCPTransport) generateRequestID() int {
	return int(time.Now().UnixNano())
}

func (t *MCPTransport) callHTTPTool(
	ctx context.Context,
	client *mcpclient.Client,
	toolName string,
	args map[string]any,
) (map[string]any, error) {
	// Prepare the request
	req := mcpapi.CallToolRequest{
		Params: mcpapi.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	}

	// Perform the call
	res, err := client.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("HTTP call to tool '%s' failed: %w", toolName, err)
	}

	// Marshal the response to JSON bytes
	raw, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response for tool '%s': %w", toolName, err)
	}

	// Unmarshal into a generic map
	var respMap map[string]any
	if err := json.Unmarshal(raw, &respMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response into map for tool '%s': %w", toolName, err)
	}

	// If the result field is itself a map, return that directly
	if resultObj, ok := respMap["result"].(map[string]any); ok {
		return resultObj, nil
	}

	// Otherwise, return the full response map
	return respMap, nil
}
