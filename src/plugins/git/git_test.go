package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo creates a throw-away git repo in a temp dir and returns a Service
// pointed at it.
func initRepo(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")

	svc, err := New(Config{RepoDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return svc, dir
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stage(t *testing.T, dir string, paths ...string) {
	t.Helper()
	args := append([]string{"add"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
}

func commit(t *testing.T, dir, msg string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func TestStatus(t *testing.T) {
	svc, dir := initRepo(t)
	writeFile(t, dir, "hello.txt", "hi\n")

	entries, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "hello.txt" {
		t.Fatalf("unexpected status: %+v", entries)
	}
}

func TestDiffUnstaged(t *testing.T) {
	svc, dir := initRepo(t)
	writeFile(t, dir, "a.txt", "line1\n")
	stage(t, dir, "a.txt")
	commit(t, dir, "initial")

	writeFile(t, dir, "a.txt", "line1\nline2\n")

	res, err := svc.Diff(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Diff, "line2") {
		t.Fatalf("expected diff to contain 'line2', got:\n%s", res.Diff)
	}
}

func TestDiffStaged(t *testing.T) {
	svc, dir := initRepo(t)
	writeFile(t, dir, "b.txt", "original\n")
	stage(t, dir, "b.txt")
	commit(t, dir, "initial")

	writeFile(t, dir, "b.txt", "modified\n")
	stage(t, dir, "b.txt")

	res, err := svc.Diff(context.Background(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Diff, "modified") {
		t.Fatalf("expected staged diff to contain 'modified', got:\n%s", res.Diff)
	}
}

func TestApplyPatch(t *testing.T) {
	svc, dir := initRepo(t)
	writeFile(t, dir, "patch.txt", "line1\nline2\n")
	stage(t, dir, "patch.txt")
	commit(t, dir, "initial")

	patch := `--- a/patch.txt
+++ b/patch.txt
@@ -1,2 +1,3 @@
 line1
 line2
+line3
`
	res, err := svc.ApplyPatch(context.Background(), patch)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("apply failed: %s", res.Message)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "patch.txt"))
	if !strings.Contains(string(data), "line3") {
		t.Fatalf("expected patched file to contain 'line3', got: %s", data)
	}
}

func TestApplyPatchConflict(t *testing.T) {
	svc, dir := initRepo(t)
	writeFile(t, dir, "c.txt", "foo\n")
	stage(t, dir, "c.txt")
	commit(t, dir, "initial")

	// Patch references context that doesn't exist → should fail gracefully.
	badPatch := `--- a/c.txt
+++ b/c.txt
@@ -1,2 +1,2 @@
-nonexistent line
+replacement
`
	res, err := svc.ApplyPatch(context.Background(), badPatch)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected apply to fail for bad patch")
	}
}

func TestCommitMessage(t *testing.T) {
	svc, dir := initRepo(t)
	writeFile(t, dir, "src/foo.go", "package main\n")
	stage(t, dir, "src/foo.go")

	res, err := svc.CommitMessage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) == 0 {
		t.Fatal("expected at least one file in CommitMessageResult")
	}
	if res.Suggestion == "" {
		t.Fatal("expected a non-empty suggestion")
	}
}

func TestCommitMessageNoStaged(t *testing.T) {
	svc, _ := initRepo(t)

	_, err := svc.CommitMessage(context.Background())
	if err == nil {
		t.Fatal("expected error when nothing is staged")
	}
}

func TestNewInvalidDir(t *testing.T) {
	_, err := New(Config{RepoDir: t.TempDir()}) // not a git repo
	if err == nil {
		t.Fatal("expected error for non-repo dir")
	}
}

func TestBuildSuggestion(t *testing.T) {
	cases := []struct {
		files []string
		want  string
	}{
		{[]string{"src/foo.go"}, "feat(src): update foo"},
		{[]string{"README.md"}, "docs: update README"},
		{[]string{"foo_test.go"}, "test: update foo_test"},
		{[]string{"a/x.go", "b/y.go"}, "feat(.): update 2 files"},
		{[]string{"pkg/a.go", "pkg/b.go"}, "feat(pkg): update 2 files"},
	}
	for _, c := range cases {
		got := buildSuggestion(c.files)
		if got != c.want {
			t.Errorf("buildSuggestion(%v) = %q, want %q", c.files, got, c.want)
		}
	}
}
