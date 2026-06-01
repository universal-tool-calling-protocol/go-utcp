package filesystem

import (
	"context"
	"errors"
	"testing"
)

func TestReadWriteListStatSearch(t *testing.T) {
	ctx := context.Background()

	svc, err := New(Config{
		RootDir:    t.TempDir(),
		AllowWrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Write(ctx, "notes/todo.txt", "build go-utcp filesystem tools\nship tests"); err != nil {
		t.Fatal(err)
	}

	content, err := svc.Read(ctx, "notes/todo.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "build go-utcp filesystem tools\nship tests" {
		t.Fatalf("unexpected content: %q", content)
	}

	entries, err := svc.List(ctx, "notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "todo.txt" {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	stat, err := svc.Stat(ctx, "notes/todo.txt")
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size == 0 || stat.IsDir {
		t.Fatalf("unexpected stat: %+v", stat)
	}

	matches, err := svc.Search(ctx, "go-utcp", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Line != 1 {
		t.Fatalf("unexpected matches: %+v", matches)
	}
}

func TestPathEscapesRoot(t *testing.T) {
	svc, err := New(Config{
		RootDir:    t.TempDir(),
		AllowWrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Write(context.Background(), "../evil.txt", "nope")
	if !errors.Is(err, ErrPathEscapesRoot) {
		t.Fatalf("expected ErrPathEscapesRoot, got %v", err)
	}
}

func TestWriteDisabled(t *testing.T) {
	svc, err := New(Config{
		RootDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Write(context.Background(), "x.txt", "hello")
	if err == nil {
		t.Fatal("expected write-disabled error")
	}
}

func TestDeleteDisabled(t *testing.T) {
	svc, err := New(Config{
		RootDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Remove(context.Background(), "x.txt")
	if err == nil {
		t.Fatal("expected delete-disabled error")
	}
}

func TestMaxReadBytes(t *testing.T) {
	ctx := context.Background()

	svc, err := New(Config{
		RootDir:      t.TempDir(),
		AllowWrite:   true,
		MaxReadBytes: 4,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Write(ctx, "big.txt", "12345"); err != nil {
		t.Fatal(err)
	}

	_, err = svc.Read(ctx, "big.txt")
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
}
