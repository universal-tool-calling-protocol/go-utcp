package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	fsplugin "github.com/universal-tool-calling-protocol/go-utcp/src/plugins/filesystem"
)

func main() {
	var (
		root        = flag.String("root", ".", "sandbox root directory")
		tool        = flag.String("tool", "", "tool name")
		path        = flag.String("path", "", "filesystem path")
		content     = flag.String("content", "", "content for write/append")
		query       = flag.String("query", "", "search query")
		maxResults  = flag.Int("max-results", 50, "maximum search results")
		maxRead     = flag.Int64("max-read-bytes", 1<<20, "maximum readable file size")
		allowWrite  = flag.Bool("allow-write", false, "enable writes")
		allowDelete = flag.Bool("allow-delete", false, "enable deletes")
		allowHidden = flag.Bool("allow-hidden", false, "show hidden files")
	)
	flag.Parse()

	svc, err := fsplugin.New(fsplugin.Config{
		RootDir:      *root,
		MaxReadBytes: *maxRead,
		AllowWrite:   *allowWrite,
		AllowDelete:  *allowDelete,
		AllowHidden:  *allowHidden,
	})
	if err != nil {
		exitErr(err)
	}

	ctx := context.Background()

	var result any

	switch *tool {
	case "fs.list":
		result, err = svc.List(ctx, *path)
	case "fs.read":
		result, err = svc.Read(ctx, *path)
	case "fs.write":
		err = svc.Write(ctx, *path, *content)
		result = map[string]any{"ok": err == nil}
	case "fs.append":
		err = svc.Append(ctx, *path, *content)
		result = map[string]any{"ok": err == nil}
	case "fs.mkdir":
		err = svc.Mkdir(ctx, *path)
		result = map[string]any{"ok": err == nil}
	case "fs.remove":
		err = svc.Remove(ctx, *path)
		result = map[string]any{"ok": err == nil}
	case "fs.stat":
		result, err = svc.Stat(ctx, *path)
	case "fs.search":
		result, err = svc.Search(ctx, *query, *maxResults)
	default:
		exitErr(fmt.Errorf("unknown or missing tool: %s", *tool))
	}

	if err != nil {
		exitErr(err)
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		exitErr(err)
	}
}

func exitErr(err error) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"error": err.Error(),
	})
	os.Exit(1)
}
