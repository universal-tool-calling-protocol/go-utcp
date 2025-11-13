package chain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

// --- Language Configurations ---
type LanguageConfig struct {
	Cmd          string
	Args         []string
	Extension    string
	CompileArgs  []string
	NeedsCompile bool
	RunCompiled  bool
}

var languageConfigs = map[string]LanguageConfig{
	"python":     {Cmd: "python3", Extension: ".py"},
	"javascript": {Cmd: "node", Extension: ".js"},
	"go":         {Cmd: "go", Args: []string{"run"}, Extension: ".go"},
	"rust":       {Cmd: "rustc", Extension: ".rs", NeedsCompile: true, RunCompiled: true},
	"java":       {Cmd: "javac", Extension: ".java", NeedsCompile: true, RunCompiled: true},
	"c":          {Cmd: "gcc", CompileArgs: []string{"-o"}, Extension: ".c", NeedsCompile: true, RunCompiled: true},
	"cpp":        {Cmd: "g++", CompileArgs: []string{"-o"}, Extension: ".cpp", NeedsCompile: true, RunCompiled: true},
	"bash":       {Cmd: "bash", Extension: ".sh"},
	"shell":      {Cmd: "sh", Extension: ".sh"},
	"typescript": {Cmd: "ts-node", Extension: ".ts"},
	"perl":       {Cmd: "perl", Extension: ".pl"},
	"ruby":       {Cmd: "ruby", Extension: ".rb"},
	"php":        {Cmd: "php", Extension: ".php"},
	"r":          {Cmd: "Rscript", Extension: ".R"},
	"lua":        {Cmd: "lua", Extension: ".lua"},
	"elixir":     {Cmd: "elixir", Extension: ".exs"},
}

// --- Server Detection ---
var serverRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)listening`),
	regexp.MustCompile(`(?i)running\s+on\s+port`),
	regexp.MustCompile(`(?i)http://`),
	regexp.MustCompile(`(?i)localhost:\d+`),
	regexp.MustCompile(`(?i):\d{4,5}`),
}

func looksLikeServerOutput(s string) bool {
	for _, re := range serverRegexes {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// --- Core UTCP Client ---
type UtcpChainClient struct {
	Client ToolCaller
}

// CallToolChain executes a chain of steps with optional result passing and streaming support.
func (c *UtcpChainClient) CallToolChain(
	ctx context.Context,
	steps []ChainStep,
	timeout time.Duration,
) (map[string]any, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	results := make(map[string]any, len(steps))
	var lastOutput string
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for i, step := range steps {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		if step.UsePrevious {
			// Merge all previous results
			for k, v := range results {
				if _, exists := step.Inputs[k]; !exists {
					step.Inputs[k] = v
				}
			}
			if lastOutput != "" {
				step.Inputs["__previous_output"] = lastOutput
			}
		}

		var (
			res any
			err error
		)

		// Handle code execution modes
		if lang, ok := step.Inputs["language"].(string); ok {
			if path, hasPath := step.Inputs["path"].(string); hasPath {
				if file, hasFile := step.Inputs["file"].(string); hasFile && path != "" && file != "" {
					res, err = c.runExternalCodeDirect(ctx, step.Inputs, timeout)
				} else {
					err = fmt.Errorf("path and file are required for %s execution", lang)
				}
			} else if codeStr, hasCode := step.Inputs["code"].(string); hasCode && lang != "go" {
				res, err = c.runExternalToolDirect(ctx, lang, codeStr, timeout)
			} else if c.Client != nil {
				res, err = c.runYaegiCode(ctx, step, timeout)
			} else {
				err = fmt.Errorf("Client is required for Yaegi execution")
			}
		} else if c.Client != nil {
			res, err = c.runYaegiCode(ctx, step, timeout)
		} else {
			err = fmt.Errorf("Client is required for tool execution")
		}

		if err != nil {
			return results, fmt.Errorf("step %d (%s) failed: %w", i+1, step.ToolName, err)
		}
		resultKey := step.ToolName
		if step.ID != "" {
			resultKey = step.ID
		}
		// --- Handle stream results properly ---
		switch v := res.(type) {
		case transports.StreamResult:
			var buf strings.Builder
			for {
				chunk, err := v.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return results, fmt.Errorf("stream error in step %d (%s): %w", i+1, step.ToolName, err)
				}
				buf.WriteString(chunk.(string))
			}
			lastOutput = strings.TrimSpace(buf.String())
			results[resultKey] = lastOutput

		default:
			// Normal result (non-stream)
			if strRes, ok := res.(string); ok {
				lastOutput = strings.TrimSpace(strRes)
			}
			results[resultKey] = res
		}

	}

	return results, nil
}

func (c *UtcpChainClient) runExternalCodeDirect(ctx context.Context, args map[string]any, timeout time.Duration) (any, error) {
	lang, ok := args["language"].(string)
	if !ok {
		return nil, fmt.Errorf("missing language")
	}

	config, ok := languageConfigs[lang]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", lang)
	}

	// Use timeout from args if provided, otherwise use the passed timeout
	if t, ok := args["timeout"].(float64); ok {
		timeout = time.Duration(int(t)) * time.Second
	}

	// Don't create a context timeout here - let runCommandWithDetection handle the timeout
	// The context is only for compilation errors or early cancellation

	path, _ := args["path"].(string)
	file, _ := args["file"].(string)
	if path == "" || file == "" {
		return nil, fmt.Errorf("path and file are required for %s execution", lang)
	}
	target := filepath.Join(path, file)

	var cmd *exec.Cmd

	// Extract previous output if UsePrevious was set
	prevOutput := ""
	if po, ok := args["__previous_output"].(string); ok {
		prevOutput = strings.TrimSpace(po)
	}

	if config.NeedsCompile {
		// Compiled languages (Rust, C, C++, Java)
		bin := filepath.Join(os.TempDir(), fmt.Sprintf("utcp-bin-%d", time.Now().UnixNano()))
		defer os.Remove(bin)

		// Use context for compilation
		compile := exec.CommandContext(ctx, config.Cmd, append(config.CompileArgs, bin, target)...)
		if out, err := compile.CombinedOutput(); err != nil {
			return string(out), fmt.Errorf("compile error: %w", err)
		}

		// Create command without context timeout - let runCommandWithDetection handle it
		if prevOutput != "" {
			cmd = exec.Command(bin, prevOutput)
		} else {
			cmd = exec.Command(bin)
		}
	} else if lang == "go" && prevOutput != "" {
		// Special handling for Go with previous output: compile then run
		bin := filepath.Join(os.TempDir(), fmt.Sprintf("utcp-go-bin-%d", time.Now().UnixNano()))
		defer os.Remove(bin)

		// Compile first - use context for compilation
		compile := exec.CommandContext(ctx, "go", "build", "-o", bin, target)
		if out, err := compile.CombinedOutput(); err != nil {
			return string(out), fmt.Errorf("go build error: %w", err)
		}

		// Run with previous output as argument - no context timeout
		cmd = exec.Command(bin, prevOutput)
	} else {
		// Interpreted languages or Go run without previous output
		argsList := append(config.Args, target)
		if prevOutput != "" {
			argsList = append(argsList, prevOutput)
		}
		// Don't use CommandContext - let runCommandWithDetection handle timeout
		cmd = exec.Command(config.Cmd, argsList...)
	}

	// runCommandWithDetection now returns (string, error) where timeouts/server detection
	// return (message, nil) instead of (message, error)
	return runCommandWithDetection(ctx, cmd, timeout, strings.Join(cmd.Args, " "))
}

func (c *UtcpChainClient) runExternalToolDirect(ctx context.Context, lang, code string, timeout time.Duration) (any, error) {
	config, ok := languageConfigs[lang]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", lang)
	}

	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("utcp-%d%s", time.Now().UnixNano(), config.Extension))
	if err := os.WriteFile(tmpFile, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write temporary file: %w", err)
	}
	defer os.Remove(tmpFile)

	return c.runExternalCodeDirect(ctx, map[string]any{
		"language": lang,
		"path":     tmpDir,
		"file":     filepath.Base(tmpFile),
		"timeout":  float64(timeout.Seconds()),
	}, timeout)
}

// --- Go / Yaegi Execution ---
func (c *UtcpChainClient) runYaegiCode(ctx context.Context, step ChainStep, timeout time.Duration) (any, error) {
	code := fmt.Sprintf(`tools["%s"](%#v)`, step.ToolName, step.Inputs)
	tools := map[string]func(map[string]any) (any, error){
		step.ToolName: func(args map[string]any) (any, error) {
			if !step.Stream {
				return c.Client.CallTool(ctx, step.ToolName, args)
			}
			return c.Client.CallToolStream(ctx, step.ToolName, args)
		},
	}
	return c.CallToolCode(ctx, tools, code, timeout)
}

// --- Cross-language Execution (kept for backward compatibility with ToolCaller interface) ---
func (c *UtcpChainClient) runExternalCode(ctx context.Context, args map[string]any, timeout time.Duration) (any, error) {
	res, err := c.runExternalCodeDirect(ctx, args, timeout)

	// If the error is a context deadline or cancellation, convert it into a string
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		if strRes, ok := res.(string); ok && strRes != "" {
			return fmt.Sprintf("Timeout reached — killed process.\n\n%s", strRes), nil
		}
		return "Timeout reached — killed process.", nil
	}

	return res, err
}

// --- Command Runner with Server Detection ---
// Returns (output, error) where timeout and server detection return (message, nil)
func runCommandWithDetection(ctx context.Context, cmd *exec.Cmd, timeout time.Duration, cmdStr string) (string, error) {
	// Ensure the process has its own process group for proper killing
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	var buf bytes.Buffer
	bufMutex := &sync.Mutex{}

	// Channel to detect if server-like output appears
	serverDetected := make(chan struct{}, 1)
	// Channel to signal command completion
	done := make(chan error, 1)

	// Helper to read from stdout/stderr
	readFrom := func(r io.Reader) {
		tmp := make([]byte, 1024)
		for {
			n, err := r.Read(tmp)
			if n > 0 {
				chunk := string(tmp[:n])
				bufMutex.Lock()
				buf.WriteString(chunk)
				if looksLikeServerOutput(chunk) {
					select {
					case serverDetected <- struct{}{}:
					default:
					}
				}
				bufMutex.Unlock()
			}
			if err != nil {
				return
			}
		}
	}

	go readFrom(stdout)
	go readFrom(stderr)

	// Wait for command completion in a goroutine
	go func() { done <- cmd.Wait() }()

	select {
	case <-serverDetected:
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		bufMutex.Lock()
		out := buf.String()
		bufMutex.Unlock()
		// Return as success, not error
		return fmt.Sprintf("Server detected — stopped safely.\n\n%s", out), nil

	case <-time.After(timeout):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		bufMutex.Lock()
		out := buf.String()
		bufMutex.Unlock()
		// Return as success, not error
		return fmt.Sprintf("Timeout reached — killed process.\n\n%s", out), nil

	case err := <-done:
		bufMutex.Lock()
		out := buf.String()
		bufMutex.Unlock()
		if err != nil {
			return out, fmt.Errorf("command failed: %w", err)
		}
		return out, nil
	}

}

// --- Yaegi Tool Executor ---
func (c *UtcpChainClient) CallToolCode(
	ctx context.Context,
	tools map[string]func(map[string]any) (any, error),
	code string,
	timeout time.Duration,
) (any, error) {
	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)

	_, err := i.Eval(`package main
var tools map[string]func(map[string]any) (any, error)`)
	if err != nil {
		return nil, err
	}

	v, err := i.Eval("tools")
	if err != nil {
		return nil, err
	}
	v.Set(reflect.ValueOf(tools))

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultChan := make(chan any, 1)
	errChan := make(chan error, 1)
	go func() {
		_, err := i.Eval(fmt.Sprintf("_result, _err := %s", code))
		if err != nil {
			errChan <- err
			return
		}

		v, err := i.Eval("_result")
		if err != nil {
			errChan <- err
			return
		}
		res := v.Interface()

		ve, err := i.Eval("_err")
		if err != nil {
			errChan <- err
			return
		}
		if ve.Interface() != nil {
			errChan <- ve.Interface().(error)
			return
		}
		resultChan <- res
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("execution timed out")
	case err := <-errChan:
		return nil, err
	case res := <-resultChan:
		return res, nil
	}
}

// --- ToolCaller Interface ---
type ToolCaller interface {
	CallTool(ctx context.Context, toolName string, args map[string]any) (any, error)
	CallToolStream(ctx context.Context, toolName string, args map[string]any) (transports.StreamResult, error)
}
