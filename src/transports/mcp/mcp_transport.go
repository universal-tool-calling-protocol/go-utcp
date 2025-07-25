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

// CallTool calls a specific tool on the MCP server and collects all streaming results.
func (t *MCPTransport) CallTool(ctx context.Context, toolName string, args map[string]any, p Provider, l *string) (any, error) {
	resultChan, err := t.CallToolStream(ctx, toolName, args, p)
	if err != nil {
		return nil, err
	}

	var results []any
	var lastError error
	chunkCount := 0

	// Collect all results from the stream
	for {
		select {
		case result, ok := <-resultChan:
			if !ok {
				// Channel closed, stream finished
				t.logger("Stream finished. Received %d chunks total", chunkCount)

				if lastError != nil {
					return nil, lastError
				}
				// Return single result if only one, otherwise return array
				if len(results) == 0 {
					return nil, fmt.Errorf("no results received from tool call")
				}
				if len(results) == 1 {
					return results[0], nil
				}
				return results, nil
			}

			if err, ok := result.(error); ok {
				lastError = err
				t.logger("Stream error: %v", err)
				continue
			}

			chunkCount++
			t.logger("Stream chunk %d received", chunkCount)
			results = append(results, result)

		case <-ctx.Done():
			t.logger("Stream cancelled by context")
			return nil, ctx.Err()
		}
	}
}

// CallToolStream calls a specific tool on the MCP server and returns a streaming channel.
func (t *MCPTransport) CallToolStream(ctx context.Context, toolName string, args map[string]any, p Provider) (<-chan any, error) {
	mp, ok := p.(*MCPProvider)
	if !ok {
		return nil, errors.New("MCPTransport can only be used with MCPProvider")
	}

	t.mutex.RLock()
	process, exists := t.processes[mp.Name]
	t.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("MCP provider '%s' is not registered", mp.Name)
	}

	t.logger("Calling MCP tool '%s' on provider '%s'", toolName, mp.Name)

	// Handle HTTP client case
	if process.httpClient != nil {
		return t.callHTTPToolStream(ctx, process.httpClient, toolName, args)
	}

	// Handle stdio process case
	return t.callStdioToolStream(ctx, process, toolName, args, mp.Timeout)
}

// callHTTPToolStream handles tool calls via HTTP client.
func (t *MCPTransport) callHTTPToolStream(
	ctx context.Context,
	client *mcpclient.Client,
	toolName string,
	args map[string]any,
) (<-chan any, error) {
	ch := make(chan any, 1)
	done := make(chan struct{}) // signal to stop sending

	client.OnNotification(func(n mcp.JSONRPCNotification) {
		select {
		case <-done:
			return // gracefully stop if stream ended
		default:
		}

		raw, err := json.Marshal(n)
		if err != nil {
			return
		}
		var chunk any
		_ = json.Unmarshal(raw, &chunk)

		select {
		case ch <- chunk:
		default:
		}
	})

	go func() {
		defer func() {
			close(done)
			close(ch)
		}()

		req := mcpapi.CallToolRequest{}
		req.Params.Name = toolName
		req.Params.Arguments = args

		res, err := client.CallTool(ctx, req)
		if err != nil {
			select {
			case ch <- err:
			case <-ctx.Done():
			}
			return
		}

		raw, _ := json.Marshal(res)
		var out any
		_ = json.Unmarshal(raw, &out)

		select {
		case ch <- out:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// callStdioToolStream handles tool calls via stdio process.
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
	resultChan := make(chan any)

	go func() {
		defer close(resultChan)

		// Create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		scanner := bufio.NewScanner(process.stdout)
		responseReceived := false

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
						// This is a notification, send it as a streaming chunk
						t.logger("Received MCP notification: %s", notification.Method)

						// Create a structured notification result
						notificationResult := map[string]interface{}{
							"type":   "notification",
							"method": notification.Method,
							"params": notification.Params,
						}

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
