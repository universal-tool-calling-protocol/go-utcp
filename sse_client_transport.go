package UTCP

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SSEClientTransport implements Server-Sent Events over HTTP for UTCP tools.
type SSEClientTransport struct {
	client *http.Client
	logger func(format string, args ...interface{})
}

// decodeToolsResponse parses a common tools discovery response.
func decodeToolsResponse(r io.ReadCloser) ([]Tool, error) {
	defer r.Close()
	var resp struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		return nil, err
	}
	return resp.Tools, nil
}

// NewSSETransport constructs a new SSEClientTransport.
func NewSSETransport(logger func(format string, args ...interface{})) *SSEClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &SSEClientTransport{
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

// RegisterToolProvider registers an SSE-based provider by fetching its tool list.
func (t *SSEClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	sseProv, ok := prov.(*SSEProvider)
	if !ok {
		return nil, errors.New("SSEClientTransport can only be used with SSEProvider")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sseProv.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range sseProv.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	return decodeToolsResponse(resp.Body)
}

// DeregisterToolProvider cleans up any resources (no-op for SSE).
func (t *SSEClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	sseProv, ok := prov.(*SSEProvider)
	if !ok {
		return errors.New("SSEClientTransport can only be used with SSEProvider")
	}

	t.logger("Deregistered SSE provider '%s'", sseProv.Name)
	return nil
}

// CallTool invokes a named tool over SSE by POSTing inputs and decoding JSON output.
func (t *SSEClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov Provider, l *string) (interface{}, error) {
	sseProv, ok := prov.(*SSEProvider)
	if !ok {
		return nil, errors.New("SSEClientTransport can only be used with SSEProvider")
	}
	url := fmt.Sprintf("%s/%s", sseProv.URL, toolName)
	payload := args
	if sseProv.BodyField != nil {
		payload = map[string]interface{}{*sseProv.BodyField: args}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range sseProv.Headers {
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
