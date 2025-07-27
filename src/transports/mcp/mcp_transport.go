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

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

// MCPTransport implements ClientTransport for MCP providers using stdio.
type MCPTransport struct {
	mu        sync.Mutex
	processes map[string]*mcpProcess
	logger    func(format string, args ...interface{})
	nextID    int
}

// mcpProcess holds the running process and associated state.
type mcpProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	scan   *bufio.Scanner
	mu     sync.Mutex
	tools  []Tool
}

// NewMCPTransport creates a new transport instance.
func NewMCPTransport(logger func(format string, args ...interface{})) *MCPTransport {
	if logger == nil {
		logger = func(string, ...interface{}) {}
	}
	return &MCPTransport{logger: logger, processes: make(map[string]*mcpProcess)}
}

// generateID returns a simple increasing id.
func (t *MCPTransport) generateID() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	return t.nextID
}

// RegisterToolProvider starts the MCP process and discovers its tools.
func (t *MCPTransport) RegisterToolProvider(ctx context.Context, p Provider) ([]Tool, error) {
	mp, ok := p.(*MCPProvider)
	if !ok {
		return nil, errors.New("MCPTransport can only be used with MCPProvider")
	}

	if err := mp.Validate(); err != nil {
		return nil, err
	}

	t.mu.Lock()
	if proc, ok := t.processes[mp.Name]; ok {
		t.mu.Unlock()
		return proc.tools, nil
	}
	t.mu.Unlock()

	if len(mp.Command) == 0 {
		return nil, errors.New("missing command for MCP provider")
	}

	cmd := exec.CommandContext(ctx, mp.Command[0], mp.Command[1:]...)
	if len(mp.Args) > 0 {
		cmd.Args = append(cmd.Args, mp.Args...)
	}
	cmd.Env = os.Environ()
	for k, v := range mp.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	if mp.WorkingDir != "" {
		cmd.Dir = mp.WorkingDir
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, err
	}

	proc := &mcpProcess{cmd: cmd, stdin: stdin, stdout: stdout, scan: bufio.NewScanner(stdout)}
	if mp.StdinData != "" {
		io.WriteString(proc.stdin, mp.StdinData)
	}

	// initialize
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      t.generateID(),
		"method":  "initialize",
		"params":  map[string]any{},
	}
	if _, err := t.sendRequest(ctx, proc, initReq, mp.Timeout); err != nil {
		t.cleanupProcess(proc)
		return nil, err
	}

	toolsReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      t.generateID(),
		"method":  "tools/list",
	}
	res, err := t.sendRequest(ctx, proc, toolsReq, mp.Timeout)
	if err != nil {
		t.cleanupProcess(proc)
		return nil, err
	}

	var tools []Tool
	if arr, ok := res["tools"].([]any); ok {
		for _, v := range arr {
			if m, ok := v.(map[string]any); ok {
				tl := Tool{}
				if n, ok := m["name"].(string); ok {
					tl.Name = n
				}
				if d, ok := m["description"].(string); ok {
					tl.Description = d
				}
				tools = append(tools, tl)
			}
		}
	}
	proc.tools = tools

	t.mu.Lock()
	t.processes[mp.Name] = proc
	t.mu.Unlock()
	return tools, nil
}

// DeregisterToolProvider stops the running process.
func (t *MCPTransport) DeregisterToolProvider(ctx context.Context, p Provider) error {
	mp, ok := p.(*MCPProvider)
	if !ok {
		return errors.New("MCPTransport can only be used with MCPProvider")
	}
	t.mu.Lock()
	proc, ok := t.processes[mp.Name]
	if ok {
		delete(t.processes, mp.Name)
	}
	t.mu.Unlock()
	if ok {
		t.cleanupProcess(proc)
	}
	return nil
}

// CallTool invokes a tool. If streaming notifications are produced, a StreamResult is returned.
func (t *MCPTransport) CallTool(ctx context.Context, toolName string, args map[string]any, p Provider, l *string) (any, error) {
	stream, err := t.CallToolStream(ctx, toolName, args, p)
	if err != nil {
		return nil, err
	}
	first, err := stream.Next()
	if err != nil {
		stream.Close()
		return nil, err
	}

	if m, ok := first.(map[string]any); ok && m["type"] == "notification" {
		ch := make(chan any, 1)
		ch <- first
		go func() {
			defer close(ch)
			for {
				v, err := stream.Next()
				if err != nil {
					return
				}
				ch <- v
			}
		}()
		return transports.NewChannelStreamResult(ch, stream.Close), nil
	}

	res := []any{first}
	for {
		v, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			stream.Close()
			return nil, err
		}
		res = append(res, v)
	}
	stream.Close()
	if len(res) == 1 {
		return res[0], nil
	}
	return res, nil
}

// CallToolStream invokes the tool and streams results.
func (t *MCPTransport) CallToolStream(ctx context.Context, toolName string, args map[string]any, p Provider) (transports.StreamResult, error) {
	mp, ok := p.(*MCPProvider)
	if !ok {
		return nil, errors.New("MCPTransport can only be used with MCPProvider")
	}
	t.mu.Lock()
	proc, ok := t.processes[mp.Name]
	t.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("provider %s not registered", mp.Name)
	}
	id := t.generateID()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}

	ch := make(chan any, 10)
	go t.handleStream(ctx, proc, req, id, ch)
	return transports.NewChannelStreamResult(ch, func() error { return nil }), nil
}

func (t *MCPTransport) handleStream(ctx context.Context, proc *mcpProcess, req map[string]any, id int, ch chan any) {
	defer close(ch)
	data, _ := json.Marshal(req)
	proc.mu.Lock()
	_, err := proc.stdin.Write(append(data, '\n'))
	proc.mu.Unlock()
	if err != nil {
		ch <- err
		return
	}

	for {
		if ctx.Err() != nil {
			ch <- ctx.Err()
			return
		}
		proc.mu.Lock()
		ok := proc.scan.Scan()
		line := proc.scan.Text()
		err := proc.scan.Err()
		proc.mu.Unlock()
		if !ok {
			if err != nil {
				ch <- err
			}
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var msg map[string]any
		if e := json.Unmarshal([]byte(line), &msg); e != nil {
			continue
		}
		if method, ok := msg["method"].(string); ok && msg["id"] == nil {
			ch <- map[string]any{"type": "notification", "method": method, "params": msg["params"]}
			continue
		}
		if idVal, ok := msg["id"]; ok && int(toInt(idVal)) == id {
			if errVal, ok := msg["error"]; ok {
				ch <- fmt.Errorf("mcp error: %v", errVal)
			} else {
				ch <- msg["result"]
			}
			return
		}
	}
}

func (t *MCPTransport) sendRequest(ctx context.Context, proc *mcpProcess, req map[string]any, timeout int) (map[string]any, error) {
	if timeout <= 0 {
		timeout = 30
	}
	id := req["id"].(int)
	ch := make(chan any, 1)
	go t.handleStream(ctx, proc, req, id, ch)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-ch:
		if err, ok := v.(error); ok {
			return nil, err
		}
		if m, ok := v.(map[string]any); ok {
			return m, nil
		}
		return nil, fmt.Errorf("unexpected response type %T", v)
	case <-time.After(time.Duration(timeout) * time.Second):
		return nil, fmt.Errorf("timeout waiting for response")
	}
}

func (t *MCPTransport) cleanupProcess(proc *mcpProcess) {
	if proc.stdin != nil {
		proc.stdin.Close()
	}
	if proc.stdout != nil {
		proc.stdout.Close()
	}
	if proc.cmd != nil && proc.cmd.Process != nil {
		proc.cmd.Process.Kill()
		proc.cmd.Wait()
	}
}

func toInt(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}
