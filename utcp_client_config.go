package UTCP

import (
	"fmt"

	"github.com/joho/godotenv"
)

// UtcpVariableNotFound is returned when a requested variable isn't present.
type UtcpVariableNotFound struct {
	VariableName string
}

func (e *UtcpVariableNotFound) Error() string {
	return fmt.Sprintf(
		"Variable %q referenced in provider configuration not found. "+
			"Please add it to the environment variables or to your UTCP configuration.",
		e.VariableName,
	)
}

// UtcpVariablesConfig is the interface for any variable‐loading strategy.
type UtcpVariablesConfig interface {
	// Load returns all variables available from this provider.
	Load() (map[string]string, error)
	// Get returns a single variable value or an error if not present.
	Get(key string) (string, error)
}

// UtcpDotEnv implements UtcpVariablesConfig by loading a .env file.
type UtcpDotEnv struct {
	EnvFilePath string
}

func NewDotEnv(path string) *UtcpDotEnv {
	return &UtcpDotEnv{EnvFilePath: path}
}

// Load reads the .env file and returns a map of key→value.
func (u *UtcpDotEnv) Load() (map[string]string, error) {
	return godotenv.Read(u.EnvFilePath)
}

// Get loads the file and looks up a single key.
func (u *UtcpDotEnv) Get(key string) (string, error) {
	vars, err := u.Load()
	if err != nil {
		return "", err
	}
	if val, ok := vars[key]; ok {
		return val, nil
	}
	return "", &UtcpVariableNotFound{VariableName: key}
}

// UtcpClientConfig holds your resolved variables and provider settings.
type UtcpClientConfig struct {
	// Variables explicitly passed in (takes precedence)
	Variables map[string]string

	// Optional path to a providers‐definition file
	ProvidersFilePath string

	// A list of providers to load from (e.g. .env, AWS SSM, Vault, etc.)
	LoadVariablesFrom []UtcpVariablesConfig
}

// NewClientConfig constructs a config with sensible defaults.
func NewClientConfig() *UtcpClientConfig {
	return &UtcpClientConfig{
		Variables:         make(map[string]string),
		ProvidersFilePath: "",
		LoadVariablesFrom: nil,
	}
}
