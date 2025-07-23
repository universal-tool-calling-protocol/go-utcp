package cli

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/internal/providers"
)

// TestPrepareEnv verifies that provider‑specific environment variables are merged
// with the base process environment.
func TestPrepareEnv(t *testing.T) {
	// Arrange
	tr := NewCliTransport(nil)
	cliProv := &CliProvider{
		EnvVars: map[string]string{"FOO": "BAR", "HELLO": "WORLD"},
		// CommandName and WorkingDir are not needed for this test
	}

	// Act
	env := tr.prepareEnv(cliProv)

	// Assert – every key=value from provider.EnvVars must be present.
	envMap := make(map[string]bool)
	for _, kv := range env {
		envMap[kv] = true
	}
	for k, v := range cliProv.EnvVars {
		kv := k + "=" + v
		if !envMap[kv] {
			t.Fatalf("expected %s to be in resulting env", kv)
		}
	}
}

// TestFormatArguments makes sure various argument types are converted
// into the expected slice of CLI flags.
func TestFormatArguments(t *testing.T) {
	tr := NewCliTransport(nil)
	args := map[string]interface{}{
		"flag":    true,
		"name":    "value",
		"numbers": []interface{}{1, 2, 3},
	}
	want := []string{"--flag", "--name", "value", "--numbers", "1", "--numbers", "2", "--numbers", "3"}

	got := tr.formatArguments(args)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("formatArguments mismatch\nwant: %v\ngot:  %v", want, got)
	}
}

// TestExtractManual covers the two parsing branches: a full UTCPManual
// JSON blob, and single‑tool JSON embedded in otherwise noisy output.
func TestExtractManual(t *testing.T) {
	tr := NewCliTransport(nil)

	t.Run("full manual", func(t *testing.T) {
		jsonBlob := `{"tools":[{"name":"hello","description":"desc"}]}`
		tools := tr.extractManual(jsonBlob, "dummy-provider")
		if len(tools) != 1 || tools[0].Name != "hello" {
			t.Fatalf("unexpected tools: %+v", tools)
		}
	})

	t.Run("mixed output with single tool", func(t *testing.T) {
		noisy := strings.Join([]string{
			"some log before",
			`{"name":"world","description":"desc"}`,
			"after noise"}, "\n")
		tools := tr.extractManual(noisy, "dummy-provider")
		if len(tools) != 1 || tools[0].Name != "world" {
			t.Fatalf("unexpected tools from noisy output: %+v", tools)
		}
	})
}

// TestExecuteCommandSuccess executes a lightweight command (the Go tool itself)
// and expects a zero exit status along with non‑empty stdout.
func TestExecuteCommandSuccess(t *testing.T) {
	// Using `go version` because every Go test environment has the `go` binary
	tr := NewCliTransport(nil)
	ctx := context.Background()

	stdout, stderr, code, err := tr.executeCommand(ctx, "go", []string{"version"}, os.Environ(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("go version returned non‑zero exit code: %d, stderr: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatalf("expected non‑empty stdout from go version")
	}
}
