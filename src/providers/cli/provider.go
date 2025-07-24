package cli

import (
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// CliProvider represents a CLI tool.
type CliProvider struct {
	BaseProvider
	CommandName string            `json:"command_name"`
	EnvVars     map[string]string `json:"env_vars,omitempty"`
	WorkingDir  *string           `json:"working_dir,omitempty"`
	// auth is always nil
}
