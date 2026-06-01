package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	RepoDir string
}

type Service struct {
	repoDir string
}

type DiffResult struct {
	Diff string `json:"diff"`
}

type StatusEntry struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

type ApplyResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

type CommitMessageResult struct {
	Suggestion string   `json:"suggestion"`
	Files      []string `json:"files"`
	Stats      string   `json:"stats"`
}

func New(cfg Config) (*Service, error) {
	if cfg.RepoDir == "" {
		return nil, errors.New("git: repo dir is required")
	}
	abs, err := filepath.Abs(cfg.RepoDir)
	if err != nil {
		return nil, err
	}
	// Verify this is actually a git repo.
	cmd := exec.Command("git", "-C", abs, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git: %q does not appear to be a git repository", abs)
	}
	return &Service{repoDir: abs}, nil
}

// Diff returns the unified diff for the working tree (or staged changes when
// staged is true). An optional path restricts the diff to that file.
func (s *Service) Diff(ctx context.Context, path string, staged bool) (DiffResult, error) {
	args := []string{"diff"}
	if staged {
		args = append(args, "--staged")
	}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := s.git(ctx, args...)
	if err != nil {
		return DiffResult{}, err
	}
	return DiffResult{Diff: out}, nil
}

// Status returns the short-format status of the working tree.
func (s *Service) Status(ctx context.Context) ([]StatusEntry, error) {
	out, err := s.git(ctx, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	var entries []StatusEntry
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		entries = append(entries, StatusEntry{
			Status: strings.TrimSpace(line[:2]),
			Path:   strings.TrimSpace(line[3:]),
		})
	}
	return entries, nil
}

// ApplyPatch applies a unified diff patch to the repository via `git apply`.
// Errors from git apply (e.g. conflicts) are returned as ApplyResult{OK:false}
// rather than a Go error so callers can inspect the message.
func (s *Service) ApplyPatch(ctx context.Context, patch string) (ApplyResult, error) {
	cmd := exec.CommandContext(ctx, "git", "apply", "--whitespace=fix", "-")
	cmd.Dir = s.repoDir
	cmd.Stdin = strings.NewReader(patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ApplyResult{OK: false, Message: strings.TrimSpace(stderr.String())}, nil
		}
		return ApplyResult{}, err
	}
	return ApplyResult{OK: true}, nil
}

// CommitMessage inspects the staged diff and returns a conventional-commit
// suggestion together with the affected file list and diff stats.
func (s *Service) CommitMessage(ctx context.Context) (CommitMessageResult, error) {
	filesOut, err := s.git(ctx, "diff", "--staged", "--name-only")
	if err != nil {
		return CommitMessageResult{}, err
	}

	var files []string
	for _, f := range strings.Split(strings.TrimRight(filesOut, "\n"), "\n") {
		if f != "" {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return CommitMessageResult{}, errors.New("git: no staged changes found")
	}

	statsOut, err := s.git(ctx, "diff", "--staged", "--stat")
	if err != nil {
		return CommitMessageResult{}, err
	}

	return CommitMessageResult{
		Suggestion: buildSuggestion(files),
		Files:      files,
		Stats:      strings.TrimSpace(statsOut),
	}, nil
}

// buildSuggestion generates a conventional-commit style prefix from the list of
// staged file paths.
func buildSuggestion(files []string) string {
	kind := inferKind(files)

	var scope string
	if len(files) == 1 {
		// Use the top-level directory component, but skip when the file is at root.
		dir := filepath.ToSlash(filepath.Dir(files[0]))
		if dir != "." {
			scope = strings.SplitN(dir, "/", 2)[0]
		}
	} else {
		// "." means files span multiple top-level dirs — still include as scope.
		scope = commonTopDir(files)
	}

	var noun string
	if len(files) == 1 {
		base := filepath.Base(files[0])
		noun = strings.TrimSuffix(base, filepath.Ext(base))
	} else {
		noun = fmt.Sprintf("%d files", len(files))
	}

	if scope != "" {
		return fmt.Sprintf("%s(%s): update %s", kind, scope, noun)
	}
	return fmt.Sprintf("%s: update %s", kind, noun)
}

func inferKind(files []string) string {
	for _, f := range files {
		switch {
		case strings.HasSuffix(f, "_test.go"),
			strings.Contains(f, "_test."),
			strings.HasSuffix(f, ".test.ts"),
			strings.HasSuffix(f, ".spec.ts"):
			return "test"
		case strings.HasSuffix(f, ".md"),
			strings.HasSuffix(f, ".txt"),
			strings.HasSuffix(f, ".rst"):
			return "docs"
		}
	}
	return "feat"
}

// commonTopDir returns the top-level directory shared by all paths, or "." if
// the files span multiple top-level directories.
func commonTopDir(files []string) string {
	if len(files) == 0 {
		return ""
	}
	top := func(p string) string {
		parts := strings.SplitN(filepath.ToSlash(p), "/", 2)
		return parts[0]
	}
	first := top(files[0])
	for _, f := range files[1:] {
		if top(f) != first {
			return "."
		}
	}
	return first
}

func (s *Service) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.repoDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
