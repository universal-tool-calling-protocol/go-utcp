package utcp

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
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	tools  []Tool
	mutex  sync.RWMutex
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

// mcpError represents an MCP JSON-RPC error.
type mcpError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
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

	t.logger("Starting MCP server '%s' with command: %v", mp.Name, mp.Command)

	// Check if process already exists
	t.mutex.RLock()
	if proc, exists := t.processes[mp.Name]; exists {
		t.mutex.RUnlock()
		return proc.tools, nil
	}
	t.mutex.RUnlock()

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

// CallTool calls a specific tool on the MCP server.
func (t *MCPTransport) CallTool(ctx context.Context, toolName string, args map[string]any, p Provider, l *string) (any, error) {
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

	// Send tool call request
	request := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
	}

	return t.sendMCPRequest(ctx, process, request, mp.Timeout)
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

	if _, err := t.sendMCPRequest(ctx, process, initRequest, mp.Timeout); err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	// Send tools/list request to discover available tools
	toolsRequest := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	result, err := t.sendMCPRequest(ctx, process, toolsRequest, mp.Timeout)
	if err != nil {
		return fmt.Errorf("tools/list failed: %w", err)
	}

	// Parse tools from response
	if err := t.parseToolsResponse(process, result); err != nil {
		return fmt.Errorf("failed to parse tools response: %w", err)
	}

	return nil
}

// sendMCPRequest sends a JSON-RPC request and waits for response.
func (t *MCPTransport) sendMCPRequest(ctx context.Context, process *mcpProcess, request mcpRequest, timeoutSeconds int) (interface{}, error) {
	// Set default timeout if not specified
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

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

	// Read response
	respChan := make(chan *mcpResponse, 1)
	errChan := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(process.stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var response mcpResponse
			if err := json.Unmarshal([]byte(line), &response); err != nil {
				t.logger("Failed to parse MCP response: %v", err)
				continue
			}

			if response.ID == request.ID {
				respChan <- &response
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errChan <- err
		}
	}()

	select {
	case response := <-respChan:
		if response.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", response.Error.Code, response.Error.Message)
		}
		return response.Result, nil
	case err := <-errChan:
		return nil, fmt.Errorf("failed to read response: %w", err)
	case <-requestCtx.Done():
		return nil, fmt.Errorf("request timeout after %d seconds", timeoutSeconds)
	}
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
}

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

	t.logger("Starting streaming call for tool '%s' on provider '%s'", toolName, mp.Name)

	// Prepare request
	request := mcpRequest{
		JSONRPC: "2.0",
		ID:      generateStreamID(),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
	}

	// Send and stream
	return t.sendMCPStreamRequest(ctx, process, request, mp.Timeout)
}

// sendMCPStreamRequest writes a JSON-RPC request and returns a channel streaming matching responses.
func (t *MCPTransport) sendMCPStreamRequest(ctx context.Context, process *mcpProcess, request mcpRequest, timeoutSeconds int) (<-chan any, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	// Serialize request
	reqData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Write request
	process.mutex.Lock()
	if _, err := process.stdin.Write(append(reqData, '\n')); err != nil {
		process.mutex.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	process.mutex.Unlock()

	// Create result channel
	out := make(chan any)

	// Start reader goroutine
	go func() {
		defer close(out)
		scanner := bufio.NewScanner(process.stdout)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var msg mcpResponse
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				t.logger("Skipping unparsable MCP message: %v", err)
				continue
			}

			// Match by ID to our request
			if msg.ID == request.ID {
				if msg.Error != nil {
					// Send error and terminate streaming
					out <- fmt.Errorf("streaming MCP error %d: %s", msg.Error.Code, msg.Error.Message)
					return
				}
				out <- msg.Result
			}
		}
	}()

	return out, nil
}

// generateStreamID generates a unique request ID for streaming calls.
// This example uses a timestamp-based approach.
func generateStreamID() int {
	return int(time.Now().UnixNano())
}
