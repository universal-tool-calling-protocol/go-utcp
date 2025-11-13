package utcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

// --- Mock tool functions ---

func mockEcho(args map[string]any) (any, error) {
	msg, ok := args["msg"].(string)
	if !ok {
		return nil, errors.New("missing msg")
	}
	return msg, nil
}

func mockAdd(args map[string]any) (any, error) {
	a, ok1 := args["a"].(int)
	b, ok2 := args["b"].(int)
	if !ok1 || !ok2 {
		return nil, errors.New("invalid args")
	}
	return a + b, nil
}

// --- Mock UTCP client implementing ToolCaller interface ---

type mockToolCaller struct{}

func (m *mockToolCaller) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	switch name {
	case "echo":
		return mockEcho(args)
	case "add":
		return mockAdd(args)
	case "run_code":
		client := &CodeModeUtcpClient{Client: m} // pass self so nested calls work
		return client.runExternalCode(ctx, args, time.Millisecond*6)
	default:
		return nil, errors.New("unknown tool")
	}
}

func (m *mockToolCaller) CallToolStream(ctx context.Context, name string, args map[string]any) (transports.StreamResult, error) {
	// For simplicity, treat streaming same as normal call
	// Return the result directly since StreamResult type structure is unknown
	return transports.NewChannelStreamResult(nil, nil), nil
}

// --- Tests ---

func TestCallToolCode_SingleStep(t *testing.T) {
	client := &CodeModeUtcpClient{
		Client: &mockToolCaller{},
	}

	// Wrap the function explicitly to ensure proper type
	echoWrapper := func(args map[string]any) (any, error) {
		msg, ok := args["msg"].(string)
		if !ok {
			return nil, errors.New("missing msg")
		}
		return msg, nil
	}

	tools := map[string]func(map[string]any) (any, error){
		"echo": echoWrapper,
	}

	code := `tools["echo"](map[string]any{"msg": "hello"})`

	res, err := client.CallToolCode(context.Background(), tools, code, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res != "hello" {
		t.Fatalf("expected 'hello', got %v", res)
	}
}

func TestCallCodeModeChain_MultiStep(t *testing.T) {
	mock := &mockToolCaller{}
	client := &CodeModeUtcpClient{
		Client: mock,
	}

	// Prepare steps
	steps := []ChainStep{
		{
			ToolName: "echo",
			Inputs:   map[string]any{"msg": "test"},
		},
		{
			ToolName: "add",
			Inputs:   map[string]any{"a": 1, "b": 2},
		},
	}

	// Run chain
	results, err := client.CallCodeModeChain(context.Background(), steps, 10*time.Second)
	if err != nil {
		t.Fatalf("chain failed: %v", err)
	}

	// Assertions
	if results["echo"] != "test" {
		t.Errorf("expected 'test', got %v", results["echo"])
	}
	if results["add"] != 3 {
		t.Errorf("expected 3, got %v", results["add"])
	}
}

func TestCallToolCode_Error(t *testing.T) {
	client := &CodeModeUtcpClient{Client: &mockToolCaller{}}

	failWrapper := func(args map[string]any) (any, error) {
		return nil, errors.New("forced error")
	}

	tools := map[string]func(map[string]any) (any, error){
		"fail": failWrapper,
	}

	code := `tools["fail"](map[string]any{})`

	_, err := client.CallToolCode(context.Background(), tools, code, 5*time.Second)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCallToolCode_Timeout(t *testing.T) {
	client := &CodeModeUtcpClient{Client: &mockToolCaller{}}

	sleepWrapper := func(args map[string]any) (any, error) {
		time.Sleep(2 * time.Second)
		return "done", nil
	}

	tools := map[string]func(map[string]any) (any, error){
		"sleep": sleepWrapper,
	}

	code := `tools["sleep"](map[string]any{})`

	_, err := client.CallToolCode(context.Background(), tools, code, 1*time.Second)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

type mockCaller struct{}

func (m *mockCaller) CallTool(ctx context.Context, toolName string, args map[string]any) (any, error) {
	if toolName == "fail" {
		return nil, os.ErrInvalid
	}
	return "ok:" + toolName, nil
}

func (m *mockCaller) CallToolStream(ctx context.Context, toolName string, args map[string]any) (transports.StreamResult, error) {
	return transports.NewChannelStreamResult(nil, nil), nil
}

// --- Helper to create temp code files ---
func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Make shell scripts executable
	if strings.HasSuffix(name, ".sh") {
		if err := os.Chmod(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestRunYaegiCode(t *testing.T) {
	client := &CodeModeUtcpClient{Client: &mockCaller{}}
	step := ChainStep{
		ToolName: "mock",
		Inputs:   map[string]any{},
		Stream:   false,
	}

	res, err := client.CallCodeModeChain(context.Background(), []ChainStep{step}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res["mock"] != "ok:mock" {
		t.Fatalf("unexpected result: %v", res["mock"])
	}
}

func TestRunExternalCode_Python(t *testing.T) {
	tmpDir := t.TempDir()
	file := writeTempFile(t, tmpDir, "hello.py", `print("hello")`)
	fileName := filepath.Base(file) // ✅ base name only

	client := &CodeModeUtcpClient{}
	res, err := client.CallCodeModeChain(context.Background(), []ChainStep{
		{
			ToolName: "python",
			Inputs: map[string]any{
				"language": "python",
				"path":     tmpDir,
				"file":     fileName,
				"timeout":  5.0,
			},
		},
	}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	out := res["python"].(string)
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected 'hello' in output, got: %s", out)
	}
}

func TestCallCodeModeChain_UsePrevious(t *testing.T) {
	client := &CodeModeUtcpClient{Client: &mockCaller{}}
	steps := []ChainStep{
		{
			ToolName: "step1",
			Inputs:   map[string]any{},
		},
		{
			ToolName:    "step2",
			Inputs:      map[string]any{},
			UsePrevious: true,
		},
	}

	res, err := client.CallCodeModeChain(context.Background(), steps, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res["step1"] != "ok:step1" || res["step2"] != "ok:step2" {
		t.Fatalf("unexpected chain results: %v", res)
	}
}

func TestRunExternalCode_Timeout(t *testing.T) {
	tmpDir := t.TempDir()

	// Use a longer sleep to ensure timeout is hit reliably
	file := writeTempFile(t, tmpDir, "sleep.sh", `sleep 10`)
	os.Chmod(file, 0o755) // ensure executable
	fileName := filepath.Base(file)

	client := &CodeModeUtcpClient{}

	// Use a shorter timeout to ensure it triggers before the sleep finishes
	// Give enough margin for process startup
	start := time.Now()
	res, err := client.CallCodeModeChain(context.Background(), []ChainStep{
		{
			ToolName: "bash",
			Inputs: map[string]any{
				"language": "bash",
				"path":     tmpDir,
				"file":     fileName,
				"timeout":  2.0, // 2 seconds - will timeout before 10s sleep completes
			},
		},
	}, 5*time.Second) // Chain timeout should be longer than command timeout
	elapsed := time.Since(start)

	// Verify it actually timed out (didn't run for full 10 seconds)
	if elapsed > 4*time.Second {
		t.Fatalf("test took too long (%v), timeout may not have triggered", elapsed)
	}

	if err != nil {
		t.Fatal(err)
	}

	output, ok := res["bash"].(string)
	if !ok {
		t.Fatalf("expected string output, got %T", res["bash"])
	}

	// More specific assertion - should contain the timeout message
	if !strings.Contains(output, "Timeout reached") {
		t.Fatalf("expected 'Timeout reached' in output, got: %s", output)
	}
}
func TestRunExternalCode_ServerDetection(t *testing.T) {
	tmpDir := t.TempDir()
	file := writeTempFile(t, tmpDir, "server.sh", `echo "Listening on 8080"; sleep 5`)
	fileName := filepath.Base(file) // ✅ base name only

	client := &CodeModeUtcpClient{}
	res, err := client.CallCodeModeChain(context.Background(), []ChainStep{
		{
			ToolName: "bash",
			Inputs: map[string]any{
				"language": "bash",
				"path":     tmpDir,
				"file":     fileName,
				"timeout":  3.0,
			},
		},
	}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res["bash"].(string), "Server detected") {
		t.Fatalf("expected server detection message, got: %s", res["bash"])
	}
}

// Add this test to code_mode_utcp_test.go

func TestRunExternalCode_Go(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple Go program
	goCode := `package main

import "fmt"

func main() {
	fmt.Println("Hello from Go!")
}
`
	file := writeTempFile(t, tmpDir, "hello.go", goCode)
	fileName := filepath.Base(file)

	client := &CodeModeUtcpClient{}
	res, err := client.CallCodeModeChain(context.Background(), []ChainStep{
		{
			ToolName: "go_script",
			Inputs: map[string]any{
				"language": "go",
				"path":     tmpDir,
				"file":     fileName,
				"timeout":  5.0,
			},
		},
	}, 5*time.Second)

	if err != nil {
		t.Fatal(err)
	}

	out := res["go_script"].(string)
	if !strings.Contains(out, "Hello from Go!") {
		t.Fatalf("expected 'Hello from Go!' in output, got: %s", out)
	}
}

func TestRunExternalCode_Go_UsePrevious(t *testing.T) {
	tmpDir := t.TempDir()

	// Helper to write Go code files
	writeGoFile := func(name, code string) string {
		return writeTempFile(t, tmpDir, name, code)
	}

	// Step 1 Go program
	file1 := writeGoFile("step1.go", `package main
import "fmt"
func main() {
    fmt.Print("Hello from Go!")
}`)

	// Step 2 Go program: appends previous output if present
	file2 := writeGoFile("step2.go", `package main
import (
    "fmt"
    "os"
)
func main() {
    prev := ""
    if len(os.Args) > 1 {
        prev = os.Args[1]
    }
    fmt.Print(prev + " And Hello again!")
}`)

	// Step 3 Go program: just to chain further
	file3 := writeGoFile("step3.go", `package main
import (
    "fmt"
    "os"
)
func main() {
    prev := ""
    if len(os.Args) > 1 {
        prev = os.Args[1]
    }
    fmt.Print(prev + " Final step!")
}`)

	client := &CodeModeUtcpClient{}

	// Build chain steps with UNIQUE tool names
	chain := []ChainStep{
		{
			ToolName: "step1",
			Inputs: map[string]any{
				"language": "go",
				"path":     tmpDir,
				"file":     filepath.Base(file1),
				"timeout":  5.0,
			},
			UsePrevious: false,
		},
		{
			ToolName: "step2",
			Inputs: map[string]any{
				"language": "go",
				"path":     tmpDir,
				"file":     filepath.Base(file2),
				"timeout":  5.0,
			},
			UsePrevious: true,
		},
		{
			ToolName: "step3",
			Inputs: map[string]any{
				"language": "go",
				"path":     tmpDir,
				"file":     filepath.Base(file3),
				"timeout":  5.0,
			},
			UsePrevious: true,
		},
	}

	res, err := client.CallCodeModeChain(context.Background(), chain, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Check each step's output
	step1Out := strings.TrimSpace(res["step1"].(string))
	step2Out := strings.TrimSpace(res["step2"].(string))
	step3Out := strings.TrimSpace(res["step3"].(string))

	if step1Out != "Hello from Go!" {
		t.Fatalf("step1: expected 'Hello from Go!', got: %q", step1Out)
	}

	if step2Out != "Hello from Go! And Hello again!" {
		t.Fatalf("step2: expected 'Hello from Go! And Hello again!', got: %q", step2Out)
	}

	if step3Out != "Hello from Go! And Hello again! Final step!" {
		t.Fatalf("step3: expected 'Hello from Go! And Hello again! Final step!', got: %q", step3Out)
	}
}

// --- Mock ToolCaller for streaming ---
type mockStreamToolCaller struct{}

func (m *mockStreamToolCaller) CallTool(ctx context.Context, toolName string, args map[string]any) (any, error) {
	if toolName == "step2" {
		prev, _ := args["__previous_output"].(string)
		return strings.ToUpper(prev), nil
	}
	return nil, nil
}

func (m *mockStreamToolCaller) CallToolStream(ctx context.Context, toolName string, args map[string]any) (transports.StreamResult, error) {
	if toolName == "step1" {
		ch := make(chan any)
		go func() {
			defer close(ch)
			for _, s := range []string{"hello ", "world"} {
				ch <- s
				time.Sleep(2 * time.Millisecond) // simulate streaming delay
			}
		}()
		return transports.NewChannelStreamResult(ch, func() error { return nil }), nil
	}
	return nil, nil
}

// --- Stream + UsePrevious test ---
func TestStreamUsePreviousChain(t *testing.T) {
	client := &CodeModeUtcpClient{
		Client: &mockStreamToolCaller{},
	}

	steps := []ChainStep{
		{
			ToolName: "step1",
			Inputs:   map[string]any{},
			Stream:   true,
		},
		{
			ToolName:    "step2",
			Inputs:      map[string]any{},
			UsePrevious: true,
		},
	}

	results, err := client.CallCodeModeChain(context.Background(), steps, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step1Result, ok := results["step1"].(string)
	if !ok || step1Result != "hello world" {
		t.Errorf("step1 result mismatch: got %v, want %v", step1Result, "hello world")
	}

	step2Result, ok := results["step2"].(string)
	if !ok || step2Result != "HELLO WORLD" {
		t.Errorf("step2 result mismatch: got %v, want %v", step2Result, "HELLO WORLD")
	}
}

func TestCallCodeModeChain_InlinePythonCode(t *testing.T) {
	client := &CodeModeUtcpClient{}

	code := `print("INLINE_OK")`

	steps := []ChainStep{
		{
			ToolName: "inline_python",
			Inputs: map[string]any{
				"language": "python",
				"code":     code,
				"timeout":  5.0,
			},
		},
	}

	res, err := client.CallCodeModeChain(context.Background(), steps, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := res["inline_python"].(string)
	if !ok {
		t.Fatalf("expected string output, got %T", res["inline_python"])
	}

	if !strings.Contains(out, "INLINE_OK") {
		t.Fatalf("expected INLINE_OK in output, got: %s", out)
	}
}

func TestCallCodeModeChain_InlineCode_NoLanguage_YaegiFallback(t *testing.T) {
	mock := &mockCaller{}
	client := &CodeModeUtcpClient{Client: mock}

	steps := []ChainStep{
		{
			ToolName: "mock",
			Inputs: map[string]any{
				"code": `tools["mock"](map[string]any{})`,
			},
		},
	}

	res, err := client.CallCodeModeChain(context.Background(), steps, 3*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res["mock"] != "ok:mock" {
		t.Fatalf("expected 'ok:mock', got %v", res["mock"])
	}
}

func TestCallCodeModeChain_StepID(t *testing.T) {
	client := &CodeModeUtcpClient{
		Client: &mockCaller{},
	}

	steps := []ChainStep{
		{
			ID:       "first",
			ToolName: "echo",
			Inputs:   map[string]any{"msg": "hello"},
		},
		{
			ID:       "sum-step",
			ToolName: "add",
			Inputs:   map[string]any{"a": 10, "b": 5},
		},
	}

	res, err := client.CallCodeModeChain(context.Background(), steps, 3*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check keys — must be IDs, not tool names
	if _, ok := res["echo"]; ok {
		t.Fatalf("unexpected key 'echo', ID override did not work")
	}
	if _, ok := res["add"]; ok {
		t.Fatalf("unexpected key 'add', ID override did not work")
	}

	// Check ID keys exist
	if _, ok := res["first"]; !ok {
		t.Fatalf("expected key 'first' missing")
	}
	if _, ok := res["sum-step"]; !ok {
		t.Fatalf("expected key 'sum-step' missing")
	}

	// Validate values
	if res["first"] != "ok:echo" {
		t.Fatalf("expected ok:echo, got %v", res["first"])
	}
	if res["sum-step"] != "ok:add" {
		t.Fatalf("expected ok:add, got %v", res["sum-step"])
	}
}
