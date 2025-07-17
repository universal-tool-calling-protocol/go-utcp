package UTCP

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"
)

// LoggerFunc defines the signature for optional logging callbacks.
// It matches fmt.Printf to ease integration with standard loggers.
type LoggerFunc func(format string, args ...interface{})

// defaultLogger just forwards to the standard library.
func defaultLogger(format string, args ...interface{}) {
	log.Printf(format, args...)
}

var (
	// ErrMCPProviderRequired indicates a function was called with the wrong provider type.
	ErrMCPProviderRequired = errors.New("can only be used with MCPProvider")
	// ErrToolCallingNotImplemented is returned by CallTool as this transport has no implementation yet.
	ErrToolCallingNotImplemented = errors.New("tool calling not implemented yet")
)

// MCPTransport implements ClientTransport over MCPProvider.
type MCPTransport struct {
	client *http.Client
	logger LoggerFunc
}

// NewMCPTransport initializes the transport with a 30 s timeout.
func NewMCPTransport(logger LoggerFunc) *MCPTransport {
	return NewMCPTransportWithClient(nil, logger)
}

// NewMCPTransportWithClient allows injecting a custom HTTP client.
func NewMCPTransportWithClient(client *http.Client, logger LoggerFunc) *MCPTransport {
	if logger == nil {
		logger = defaultLogger
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &MCPTransport{client: client, logger: logger}
}

// RegisterToolProvider only accepts *MCPProvider; logs its Name().
func (t *MCPTransport) RegisterToolProvider(
	ctx context.Context,
	provider Provider,
) ([]Tool, error) {
	prov, ok := provider.(*MCPProvider)
	if !ok {
		return nil, ErrMCPProviderRequired
	}
	t.logger("Registered MCP provider '%s'", prov.Name())
	return nil, nil
}

// DeregisterToolProvider only accepts *MCPProvider; logs its Name().
func (t *MCPTransport) DeregisterToolProvider(
	ctx context.Context,
	provider Provider,
) error {
	prov, ok := provider.(*MCPProvider)
	if !ok {
		return ErrMCPProviderRequired
	}
	t.logger("Deregistered MCP provider '%s'", prov.Name())
	return nil
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
		return nil, ErrMCPProviderRequired
	}
	return nil, ErrToolCallingNotImplemented
}
