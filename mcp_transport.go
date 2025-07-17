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

var (
	// ErrMCPProviderRequired indicates a function was called with the wrong provider type.
	ErrMCPProviderRequired = errors.New("can only be used with MCPProvider")
	// ErrNotImplemented is returned by CallTool as this transport has no implementation yet.
	ErrNotImplemented = errors.New("MCP transport invocation not implemented yet")
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

// provider ensures the given Provider is an MCPProvider.
func (t *MCPTransport) provider(p Provider) (*MCPProvider, error) {
	prov, ok := p.(*MCPProvider)
	if !ok {
		return nil, ErrMCPProviderRequired
	}
	return prov, nil
}

// RegisterToolProvider only accepts *MCPProvider; logs its Name().
func (t *MCPTransport) RegisterToolProvider(
	ctx context.Context,
	provider Provider,
) ([]Tool, error) {
	prov, err := t.provider(provider)
	if err != nil {
		return nil, err
	}
	t.logger("Registered MCP provider '%s'", prov.Name())
	return nil, nil
}

// DeregisterToolProvider only accepts *MCPProvider; logs its Name().
func (t *MCPTransport) DeregisterToolProvider(
	ctx context.Context,
	provider Provider,
) error {
	prov, err := t.provider(provider)
	if err != nil {
		return err
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
	if _, err := t.provider(provider); err != nil {
		return nil, err
	}
	return nil, ErrNotImplemented
}
