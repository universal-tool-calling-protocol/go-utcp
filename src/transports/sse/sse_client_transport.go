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

	. "github.com/universal-tool-calling-protocol/go-utcp/src/helpers"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/sse"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
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
		return t.handleSSEStream(ctx, resp.Body)
	}

	// fallback to JSON
	defer resp.Body.Close()
	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// handleSSEStream creates a streaming result using NewChannelStreamResult
func (t *SSEClientTransport) handleSSEStream(ctx context.Context, body io.ReadCloser) (transports.StreamResult, error) {
	eventChan := make(chan any, 10) // buffered channel to prevent blocking

	// Start a goroutine to read SSE events and send them to the channel
	go func() {
		defer close(eventChan)
		defer body.Close()

		reader := bufio.NewReader(body)
		var dataBuf strings.Builder

		for {
			select {
			case <-ctx.Done():
				// Context cancelled, send error and return
				eventChan <- ctx.Err()
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return
				}
				eventChan <- err
				return
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
						eventChan <- err
						return
					} else {
						t.logger("Received SSE event: %v", evt)
						eventChan <- evt
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
	}()

	// Create a close function that will close the body if needed
	closeFn := func() error {
		// The body will be closed by the goroutine, but we can cancel context here if needed
		return nil
	}

	return transports.NewChannelStreamResult(eventChan, closeFn), nil
}

// Legacy method for backward compatibility - now returns slice from stream
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

func (t *SSEClientTransport) CallToolStream(
	ctx context.Context,
	toolName string,
	args map[string]any,
	p Provider,
) (transports.StreamResult, error) {
	return nil, errors.New("streaming is supported by SSEClientTransport, use CallTool")
}
