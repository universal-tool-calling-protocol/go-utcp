package UTCP

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
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
		// kill handled by CommandContext
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
	if !ok || cliProv.CommandName == "" {
		return nil, errors.New("invalid CliProvider or missing CommandName")
	}

	t.logInfo("Registering provider: " + cliProv.Name)

	parts := strings.Fields(cliProv.CommandName)
	cmdPath := parts[0]
	cmdArgs := parts[1:]
	env := t.prepareEnv(cliProv)

	workDir := ""
	if cliProv.WorkingDir != nil {
		workDir = *cliProv.WorkingDir
	}
	stdout, stderr, code, err := t.executeCommand(ctx, cmdPath, cmdArgs, env, workDir, "")
	if err != nil && code != 0 {
		return nil, err
	}

	output := stdout
	if code != 0 {
		output = stderr
	}
	if strings.TrimSpace(output) == "" {
		t.logInfo("No output from discovery command")
		return nil, nil
	}

	tools := t.extractManual(output, cliProv.Name)
	return tools, nil
}

// DeregisterToolProvider is a no-op for CLI transport.
func (t *CliTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	// stateless
	return nil
}

// formatArguments converts a map of args to CLI flags.
func (t *CliTransport) formatArguments(args map[string]interface{}) []string {
	var parts []string
	for k, v := range args {
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

// extractManual parses UTCPManual JSON from output.
func (t *CliTransport) extractManual(output, name string) []Tool {
	var manuals UtcpManual
	// try full output
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &manuals); err == nil {
		return manuals.Tools
	}
	// scan lines
	var tools []Tool
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			var single map[string]interface{}
			if err := json.Unmarshal([]byte(line), &single); err == nil {
				if _, ok := single["tools"]; ok {
					b, _ := json.Marshal(single)
					var m UtcpManual
					if err2 := json.Unmarshal(b, &m); err2 == nil {
						tools = append(tools, m.Tools...)
					}
				} else if single["name"] != nil && single["description"] != nil {
					b, _ := json.Marshal(single)
					var tdef Tool
					if err2 := json.Unmarshal(b, &tdef); err2 == nil {
						tools = append(tools, tdef)
					}
				}
			}
		}
	}
	return tools
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

	parts := strings.Fields(cliProv.CommandName)
	cmdPath := parts[0]
	cmdArgs := parts[1:]
	cmdArgs = append(cmdArgs, t.formatArguments(args)...)
	env := t.prepareEnv(cliProv)

	workDir := ""
	if cliProv.WorkingDir != nil {
		workDir = *cliProv.WorkingDir
	}
	stdout, stderr, code, err := t.executeCommand(ctx, cmdPath, cmdArgs, env, workDir, "")
	output := stdout
	if err != nil {
		t.logError(fmt.Sprintf("Error calling tool %s: %v", toolName, err))
		if code != 0 {
			output = stderr
		}
	} else if code != 0 {
		t.logError(fmt.Sprintf("Tool %s returned non-zero exit code: %d", toolName, code))
	}
	if code != 0 {
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
