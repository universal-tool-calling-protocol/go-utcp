package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	shellplugin "github.com/universal-tool-calling-protocol/go-utcp/src/plugins/shell"
)

func main() {
	var (
		tool    = flag.String("tool", "", "tool name (shell.run)")
		command = flag.String("command", "", "command to execute")
		argsRaw = flag.String("args", "[]", "JSON array of command arguments")
		allow   = flag.String("allow", "", "comma-separated allowlist of command names")
		workDir = flag.String("workdir", ".", "working directory")
	)
	flag.Parse()

	if *allow == "" {
		exitErr(fmt.Errorf("--allow is required"))
	}

	var cmdArgs []string
	if err := json.Unmarshal([]byte(*argsRaw), &cmdArgs); err != nil {
		exitErr(fmt.Errorf("--args must be a JSON array: %w", err))
	}

	allowlist := strings.Split(*allow, ",")
	svc, err := shellplugin.New(shellplugin.Config{
		Allowlist: allowlist,
		WorkDir:   *workDir,
	})
	if err != nil {
		exitErr(err)
	}

	ctx := context.Background()

	switch *tool {
	case "shell.run":
		res, runErr := svc.Run(ctx, *command, cmdArgs)
		if runErr != nil {
			exitErr(runErr)
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
