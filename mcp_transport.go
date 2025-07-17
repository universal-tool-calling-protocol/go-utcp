package UTCP

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"
)

// LoggerFunc is the signature for an optional logger.
type LoggerFunc func(format string, args ...interface{})

// defaultLogger just forwards to the standard library.
func defaultLogger(format string, args ...interface{}) {
	log.Printf(format, args...)
}

// MCPTransport implements ClientTransport over MCPProvider.
type MCPTransport struct {
	client *http.Client
	logger LoggerFunc
}

// NewMCPTransport initializes the transport with a 30 s timeout.
func NewMCPTransport(logger LoggerFunc) *MCPTransport {
	if logger == nil {
		logger = defaultLogger
	}
	return &MCPTransport{
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

// RegisterToolProvider only accepts *MCPProvider; logs its Name().
func (t *MCPTransport) RegisterToolProvider(
	ctx context.Context,
	provider Provider,
) ([]Tool, error) {
	prov, ok := provider.(*MCPProvider)
	if !ok {
		return nil, errors.New("can only be used with MCPProvider")
	}
	t.logger("Registered MCP provider '%s'", prov.Name)
	return nil, nil
}

// DeregisterToolProvider only accepts *MCPProvider; logs its Name().
func (t *MCPTransport) DeregisterToolProvider(
	ctx context.Context,
	provider Provider,
) error {
	prov, ok := provider.(*MCPProvider)
	if !ok {
		return errors.New("can only be used with MCPProvider")
	}
	t.logger("Deregistered MCP provider '%s'", prov.Name)
	return nil
}

// notImplErr is a custom error so errors.Is() works.
type notImplErr struct{}

func (e notImplErr) Error() string {
	return "MCP transport invocation not implemented yet"
}

func (e notImplErr) Is(target error) bool {
	return target != nil && target.Error() == e.Error()
}

// CallTool only accepts *MCPProvider and returns the “not implemented” error.
func (t *MCPTransport) CallTool(
	ctx context.Context,
	toolName string,
	params map[string]any,
	provider Provider,
	version *string,
) (any, error) {
	if _, ok := provider.(*MCPProvider); !ok {
		return nil, errors.New("can only be used with MCPProvider")
	}
	return nil, notImplErr{}
}
