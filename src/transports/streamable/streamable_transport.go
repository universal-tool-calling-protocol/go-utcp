package streamable

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"
	"io"
	"net/http"
	"time"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/helpers"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/streamable"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

// StreamableHTTPClientTransport implements HTTP with streaming support.
// It can stream NDJSON or JSON Sequence responses, emitting each chunk via logger
// and updating the last-chunk pointer if provided.
type StreamableHTTPClientTransport struct {
	client  *http.Client
	logger  func(format string, args ...interface{})
	logging bool
}

// NewStreamableHTTPTransport constructs a new StreamableHTTPClientTransport.
func NewStreamableHTTPTransport(logger func(format string, args ...interface{})) *StreamableHTTPClientTransport {
	logging := logger != nil
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &StreamableHTTPClientTransport{
		client:  &http.Client{Timeout: 30 * time.Second},
		logger:  logger,
		logging: logging,
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("register provider %q: %s: %s", streamProv.Name, resp.Status, body)
	}

	return DecodeToolsResponse(resp.Body)
}

// CallTool invokes a named tool via HTTP POST and supports streaming NDJSON or JSON Sequence.
// It returns either the single parsed JSON result or a slice of results if multiple chunks are received.
// It logs each chunk via the logger, and if "l" is non-nil, sets *l to the last raw JSON chunk.
func (t *StreamableHTTPClientTransport) CallTool(
	ctx context.Context,
	toolName string,
	args map[string]interface{},
	prov Provider,
	l *string,
) (interface{}, error) {
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("call tool %q: %s: %s", toolName, resp.Status, body)
	}

	dec := json.NewDecoder(resp.Body)
	streamCtx, cancel := context.WithCancel(ctx)
	resultCh := make(chan any, 10)

	go func() {
		defer close(resultCh)
		defer resp.Body.Close()
		defer cancel()
		send := func(value any) bool {
			select {
			case resultCh <- value:
				return true
			case <-streamCtx.Done():
				return false
			}
		}

		for {
			select {
			case <-streamCtx.Done():
				select {
				case resultCh <- streamCtx.Err():
				default:
				}
				return
			default:
			}

			var obj interface{}
			if t.logging || l != nil {
				var raw json.RawMessage
				if err := dec.Decode(&raw); err != nil {
					if err == io.EOF {
						return
					}
					send(err)
					return
				}
				rawString := string(bytes.TrimSpace(raw))
				if t.logging {
					t.logger("received chunk: %s", rawString)
				}
				if l != nil {
					*l = rawString
				}
				if err := json.Unmarshal(raw, &obj); err != nil {
					send(err)
					return
				}
			} else if err := dec.Decode(&obj); err != nil {
				if err == io.EOF {
					return
				}
				send(err)
				return
			}
			if !send(obj) {
				return
			}
		}
	}()

	streamResult := transports.NewChannelStreamResult(resultCh, func() error {
		cancel()
		return resp.Body.Close()
	})

	return streamResult, nil
}

// CallToolStream invokes a tool and returns its streamed results.
func (t *StreamableHTTPClientTransport) CallToolStream(
	ctx context.Context,
	toolName string,
	args map[string]interface{},
	prov Provider) (transports.StreamResult, error) {
	result, err := t.CallTool(ctx, toolName, args, prov, nil)
	if err != nil {
		return nil, err
	}

	// If CallTool returned a StreamResult, return it directly
	if streamResult, ok := result.(transports.StreamResult); ok {
		return streamResult, nil
	}

	// Otherwise, wrap single result in a SliceStreamResult for compatibility
	return transports.NewSliceStreamResult([]any{result}, nil), nil
}

// DeregisterToolProvider clears any streaming-specific state (no-op).
func (t *StreamableHTTPClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	return nil
}
