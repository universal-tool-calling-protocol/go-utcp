package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/manual"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/cli"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// CliTransport is a CLI-based transport for UTCP client.
// It discovers and executes tools via external command-line calls.
type CliTransport struct {
	logger func(format string, args ...interface{})
}

// NewCliTransport creates a new CliTransport with an optional logger.
func NewCliTransport(logger func(format string, args ...interface{})) *CliTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &CliTransport{logger: logger}
}

func (t *CliTransport) logInfo(msg string) {
	t.logger("[CliTransport] %s", msg)
}

func (t *CliTransport) logError(msg string) {
	t.logger("[CliTransport Error] %s", msg)
}

// prepareEnv merges base environment with provider-specific variables.
func (t *CliTransport) prepareEnv(provider *CliProvider) []string {
	env := os.Environ()
	for k, v := range provider.EnvVars {
		env = append(env, k+"="+v)
	}
	return env
}

// executeCommand runs a command with timeout, working directory, and optional stdin.
func (t *CliTransport) executeCommand(
	ctx context.Context,
	cmdPath string,
	args []string,
	env []string,
	workDir string,
	input string,
) (string, string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cmdPath, args...)
	cmd.Env = env
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}

	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()
	retCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		retCode = exitErr.ExitCode()
	} else if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			t.logError("Command timed out: " + cmdPath + " " + strings.Join(args, " "))
		}
		return stdout, stderr, retCode, err
	}

	return stdout, stderr, retCode, nil
}

// RegisterToolProvider discovers tools by executing provider.CommandName and parsing UTCPManual JSON.
func (t *CliTransport) RegisterToolProvider(
	ctx context.Context,
	prov Provider,
) ([]Tool, error) {
	cliProv, ok := prov.(*CliProvider)
	if !ok || strings.TrimSpace(cliProv.CommandName) == "" {
		return nil, errors.New("invalid CliProvider or missing CommandName")
	}

	t.logInfo("Registering provider: " + cliProv.Name)

	parts := strings.Fields(cliProv.CommandName)
	if len(parts) == 0 {
		return nil, errors.New("invalid CliProvider or empty CommandName")
	}

	cmdPath := parts[0]
	cmdArgs := append([]string{}, parts[1:]...)

	// Discovery must call the provider's discovery command.
	// Your filesystem binary prints tools only when "list-tools" is passed.
	cmdArgs = append(cmdArgs, "list-tools")

	env := t.prepareEnv(cliProv)

	workDir := ""
	if cliProv.WorkingDir != nil {
		workDir = *cliProv.WorkingDir
	}

	stdout, stderr, code, err := t.executeCommand(ctx, cmdPath, cmdArgs, env, workDir, "")
	if err != nil {
		return nil, fmt.Errorf(
			"discover tools for provider %q failed: command=%q args=%v stderr=%q: %w",
			cliProv.Name,
			cmdPath,
			cmdArgs,
			strings.TrimSpace(stderr),
			err,
		)
	}

	if code != 0 {
		return nil, fmt.Errorf(
			"discover tools for provider %q exited with code %d: command=%q args=%v stdout=%q stderr=%q",
			cliProv.Name,
			code,
			cmdPath,
			cmdArgs,
			strings.TrimSpace(stdout),
			strings.TrimSpace(stderr),
		)
	}

	output := strings.TrimSpace(stdout)
	if output == "" {
		return nil, fmt.Errorf(
			"discover tools for provider %q returned empty stdout: command=%q args=%v stderr=%q",
			cliProv.Name,
			cmdPath,
			cmdArgs,
			strings.TrimSpace(stderr),
		)
	}

	tools, err := t.extractManual(output, cliProv.Name)
	if err != nil {
		return nil, fmt.Errorf(
			"parse discovered tools for provider %q failed: %w; stdout=%q stderr=%q",
			cliProv.Name,
			err,
			output,
			strings.TrimSpace(stderr),
		)
	}

	return tools, nil
}

// DeregisterToolProvider is a no-op for CLI transport.
func (t *CliTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	return nil
}

// formatArguments converts a map of args to CLI flags.
func (t *CliTransport) formatArguments(args map[string]interface{}) []string {
	var parts []string

	// Gather and sort keys to ensure deterministic ordering
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build the argument slice in key order
	for _, k := range keys {
		v := args[k]
		switch val := v.(type) {
		case bool:
			if val {
				parts = append(parts, "--"+k)
			}
		case []interface{}:
			for _, item := range val {
				parts = append(parts, "--"+k, fmt.Sprint(item))
			}
		default:
			parts = append(parts, "--"+k, fmt.Sprint(val))
		}
	}
	return parts
}

func (t *CliTransport) extractManual(output, name string) ([]Tool, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, fmt.Errorf("empty discovery output")
	}

	// Format 1: UTCP manual object: {"tools":[...]}
	var manual UtcpManual
	if err := json.Unmarshal([]byte(output), &manual); err == nil && len(manual.Tools) > 0 {
		return manual.Tools, nil
	}

	// Format 2: plain array: [{"name":"fs.list","description":"..."}]
	var tools []Tool
	if err := json.Unmarshal([]byte(output), &tools); err == nil && len(tools) > 0 {
		return tools, nil
	}

	// Format 3: newline-delimited objects.
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var lineManual UtcpManual
		if err := json.Unmarshal([]byte(line), &lineManual); err == nil && len(lineManual.Tools) > 0 {
			tools = append(tools, lineManual.Tools...)
			continue
		}

		var tool Tool
		if err := json.Unmarshal([]byte(line), &tool); err == nil && tool.Name != "" {
			tools = append(tools, tool)
			continue
		}
	}

	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools found in discovery output")
	}

	return tools, nil
}

// CallTool executes a registered CLI tool with arguments.
func (t *CliTransport) CallTool(
	ctx context.Context,
	toolName string,
	args map[string]interface{},
	prov Provider,
	l *string,
) (interface{}, error) {
	cliProv, ok := prov.(*CliProvider)
	if !ok || cliProv.CommandName == "" {
		return nil, errors.New("invalid CliProvider or missing CommandName")
	}

	// Prepare the JSON payload for the tool
	payloadBytes, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal args: %w", err)
	}
	input := string(payloadBytes)

	// Build command args: call <provider> <tool> [--flags]
	parts := strings.Fields(cliProv.CommandName)
	cmdPath := parts[0]
	cmdArgs := []string{"call", cliProv.Name, toolName}
	cmdArgs = append(cmdArgs, t.formatArguments(args)...)

	env := t.prepareEnv(cliProv)

	workDir := ""
	if cliProv.WorkingDir != nil {
		workDir = *cliProv.WorkingDir
	}
	stdout, stderr, code, err := t.executeCommand(ctx, cmdPath, cmdArgs, env, workDir, input)
	output := stdout
	if err != nil {
		t.logError(fmt.Sprintf("Error calling tool %s: %v", toolName, err))
		if code != 0 {
			output = stderr
		}
	} else if code != 0 {
		t.logError(fmt.Sprintf("Tool %s returned non-zero exit code: %d", toolName, code))
		output = stderr
	}
	if strings.TrimSpace(output) == "" {
		return "", nil
	}
	var result interface{}
	if err := json.Unmarshal([]byte(output), &result); err == nil {
		t.logInfo("Returning JSON from tool: " + toolName)
		return result, nil
	}
	return strings.TrimSpace(output), nil
}

// Close cleans up resources (no-op).
func (t *CliTransport) Close() error {
	return nil
}

func (t *CliTransport) CallToolStream(
	ctx context.Context,
	toolName string,
	args map[string]any,
	p Provider,
) (transports.StreamResult, error) {
	return nil, errors.New("streaming not supported by CliTransport")
}
