// UTCP/streamable_transport.go
package UTCP

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// StreamableHTTPClientTransport implements HTTP with streaming support.
type StreamableHTTPClientTransport struct {
	client *http.Client
	logger func(format string, args ...interface{})
}

// NewStreamableHTTPTransport constructs a new StreamableHTTPClientTransport.
func NewStreamableHTTPTransport(logger func(format string, args ...interface{})) *StreamableHTTPClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &StreamableHTTPClientTransport{
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

// RegisterToolProvider fetches and returns the list of tools from the provider.
func (t *StreamableHTTPClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	streamProv, ok := prov.(*StreamableHttpProvider)
	if !ok {
		return nil, errors.New("StreamableHTTPClientTransport can only be used with StreamableHttpProvider")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamProv.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range streamProv.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeToolsResponse(resp.Body)
}

// CallTool invokes a named tool via HTTP POST and returns its result.
func (t *StreamableHTTPClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov Provider, l *string) (interface{}, error) {
	streamProv, ok := prov.(*StreamableHttpProvider)
	if !ok {
		return nil, errors.New("StreamableHTTPClientTransport can only be used with StreamableHttpProvider")
	}
	url := fmt.Sprintf("%s/%s", streamProv.URL, toolName)
	data, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range streamProv.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeregisterToolProvider clears any streaming-specific state (no-op).
func (t *StreamableHTTPClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	return nil
}
