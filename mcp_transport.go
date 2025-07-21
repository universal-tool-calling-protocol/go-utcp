package utcp

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
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

	// Fetch the tool list from a UTCP discovery endpoint.
	// If the provider name looks like a URL, use it; otherwise fall back to a local server.
	url := prov.Name()
	if !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
		url = "http://localhost:8000"
	}
	if !strings.HasSuffix(url, "/utcp") {
		if strings.HasSuffix(url, "/") {
			url += "utcp"
		} else {
			url += "/utcp"
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	manual := NewUtcpManualFromMap(raw)
	return manual.Tools, nil
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
	t.logger("Invoking MCP tool '%s'", toolName)
	// No real MCP protocol implementation exists yet. Return the
	// parameters for demonstration purposes so examples can run.
	return map[string]any{"tool": toolName, "params": params}, nil
}
