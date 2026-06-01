package shell

import (
	"context"
	"errors"
	"testing"
)

func TestRunAllowed(t *testing.T) {
	svc, err := New(Config{Allowlist: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.Run(context.Background(), "echo", []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", res.ExitCode)
	}
	if res.Stdout != "hello\n" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}

func TestRunNotAllowed(t *testing.T) {
	svc, err := New(Config{Allowlist: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Run(context.Background(), "rm", []string{"-rf", "/"})
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("expected ErrCommandNotAllowed, got %v", err)
	}
}

func TestRunNonZeroExitIsNotError(t *testing.T) {
	svc, err := New(Config{Allowlist: []string{"sh"}})
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.Run(context.Background(), "sh", []string{"-c", "exit 2"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res.ExitCode != 2 {
		t.Fatalf("expected exit 2, got %d", res.ExitCode)
	}
}

func TestEmptyAllowlistErrors(t *testing.T) {
	_, err := New(Config{Allowlist: nil})
	if err == nil {
		t.Fatal("expected error for empty allowlist")
	}
}

func TestPathInjectionPrevented(t *testing.T) {
	// "../../bin/echo" has base "echo" — still runs echo, not a path-injected binary.
	// But a command whose base is not in the allowlist must be rejected.
	svc, err := New(Config{Allowlist: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Run(context.Background(), "../../bin/rm", nil)
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("expected ErrCommandNotAllowed, got %v", err)
	}
}
