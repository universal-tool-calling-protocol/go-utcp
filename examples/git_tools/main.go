package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	gitplugin "github.com/universal-tool-calling-protocol/go-utcp/src/plugins/git"
)

func main() {
	var (
		tool   = flag.String("tool", "", "tool name")
		repo   = flag.String("repo", ".", "path to git repository")
		path   = flag.String("path", "", "file path filter for git.diff")
		staged = flag.Bool("staged", false, "show staged diff (git.diff)")
		patch  = flag.String("patch", "", "unified diff patch content (git.apply_patch)")
	)
	flag.Parse()

	svc, err := gitplugin.New(gitplugin.Config{RepoDir: *repo})
	if err != nil {
		exitErr(err)
	}

	ctx := context.Background()

	switch *tool {
	case "git.diff":
		res, e := svc.Diff(ctx, *path, *staged)
		if e != nil {
			exitErr(e)
		}
		encode(res)

	case "git.status":
		res, e := svc.Status(ctx)
		if e != nil {
			exitErr(e)
		}
		encode(res)

	case "git.apply_patch":
		if *patch == "" {
			exitErr(fmt.Errorf("--patch is required for git.apply_patch"))
		}
		res, e := svc.ApplyPatch(ctx, *patch)
		if e != nil {
			exitErr(e)
		}
		encode(res)

	case "git.commit_message":
		res, e := svc.CommitMessage(ctx)
		if e != nil {
			exitErr(e)
		}
		encode(res)

	default:
		exitErr(fmt.Errorf("unknown or missing tool: %q", *tool))
	}
}

func encode(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		exitErr(err)
	}
}

func exitErr(err error) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"error": err.Error()})
	os.Exit(1)
}
