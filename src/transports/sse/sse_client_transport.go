// sse_client_transport.go
package sse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/concepts"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers"
)

// SSEClientTransport implements Server-Sent Events over HTTP for UTCP tools.
type SSEClientTransport struct {
	client *http.Client
	logger func(format string, args ...interface{})
}

// NewSSETransport constructs a new SSEClientTransport without a built-in timeout,
// allowing long-lived streams to be managed by context.
func NewSSETransport(logger func(format string, args ...interface{})) *SSEClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &SSEClientTransport{
		client: &http.Client{},
		logger: logger,
	}
}

// RegisterToolProvider registers an SSE-based provider by fetching its tool list.
func (t *SSEClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	sseProv, ok := prov.(*SSEProvider)
	if !ok {
		return nil, errors.New("SSEClientTransport can only be used with SSEProvider")
	}
	url := sseProv.URL

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	// Fail fast on non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("register provider %q error: %s: %s", sseProv.Name, resp.Status, string(body))
	}
	return DecodeToolsResponse(resp.Body)
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

// CallTool invokes a named tool, using SSE if available.
func (t *SSEClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov Provider, lastEventID *string) (interface{}, error) {
	sseProv, ok := prov.(*SSEProvider)
	if !ok {
		return nil, errors.New("SSEClientTransport can only be used with SSEProvider")
	}
	// build URL for the tool
	url := fmt.Sprintf("%s/%s", sseProv.URL, toolName)

	// prepare payload
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
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID != nil {
		req.Header.Set("Last-Event-ID", *lastEventID)
	}
	for k, v := range sseProv.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}

	// Fail fast on non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("tool %q error: %s: %s", toolName, resp.Status, string(body))
	}

	// detect SSE vs JSON
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "event-stream") {
		return t.handleSSE(resp.Body)
	}

	// fallback to JSON
	defer resp.Body.Close()
	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// handleSSE reads and parses an event-stream, returning slices of parsed JSON events.
func (t *SSEClientTransport) handleSSE(body io.ReadCloser) ([]interface{}, error) {
	defer body.Close()
	reader := bufio.NewReader(body)
	var events []interface{}
	var dataBuf strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return events, err
		}
		line = strings.TrimRight(line, "\r\n")

		// capture event-id for reconnect support
		if strings.HasPrefix(line, "id: ") {
			lastID := line[len("id: "):]
			t.logger("SSE last-event-id now: %s", lastID)
			continue
		}

		// end of event, decode accumulated data
		if line == "" {
			if dataBuf.Len() > 0 {
				var evt interface{}
				if err := json.Unmarshal([]byte(dataBuf.String()), &evt); err != nil {
					t.logger("failed to unmarshal SSE data: %v", err)
				} else {
					events = append(events, evt)
					t.logger("Received SSE event: %v", evt)
				}
				dataBuf.Reset()
			}
			continue
		}

		// accumulate data lines, preserving literal newlines between them
		if strings.HasPrefix(line, "data: ") {
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(line[len("data: "):])
		}
	}
	return events, nil
}
