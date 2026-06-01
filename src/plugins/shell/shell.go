package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrCommandNotAllowed = errors.New("shell: command not in allowlist")

type Config struct {
	Allowlist []string // permitted base command names, e.g. ["ls","grep","cat"]
	WorkDir   string   // working directory; defaults to "."
}

type Service struct {
	allowlist map[string]bool
	workDir   string
}

type RunResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func New(cfg Config) (*Service, error) {
	if len(cfg.Allowlist) == 0 {
		return nil, errors.New("shell: allowlist must not be empty")
	}
	allow := make(map[string]bool, len(cfg.Allowlist))
	for _, cmd := range cfg.Allowlist {
		allow[filepath.Base(cmd)] = true
	}
	wd := cfg.WorkDir
	if wd == "" {
		wd = "."
	}
	return &Service{allowlist: allow, workDir: wd}, nil
}

// Run executes command (looked up by base name from PATH) only if it is in the
// allowlist. A non-zero exit code is surfaced in RunResult, not as an error.
func (s *Service) Run(ctx context.Context, command string, args []string) (RunResult, error) {
	base := filepath.Base(command)
	if !s.allowlist[base] {
		return RunResult{}, fmt.Errorf("%w: %q", ErrCommandNotAllowed, base)
	}

	// Resolve through PATH so callers cannot inject an arbitrary binary path.
	resolved, err := exec.LookPath(base)
	if err != nil {
		return RunResult{}, fmt.Errorf("shell: command %q not found in PATH", base)
	}

	cmd := exec.CommandContext(ctx, resolved, args...)
	cmd.Dir = s.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			return RunResult{}, runErr
		}
	}

	return RunResult{
		Stdout:   stdout.String(),
		Stderr:   strings.TrimRight(stderr.String(), "\n"),
		ExitCode: code,
	}, nil
}
