package streamable

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	// Use a buffered reader to handle NDJSON or JSON Sequence
	reader := bufio.NewReader(resp.Body)
	dec := json.NewDecoder(reader)

	// Create a channel for streaming results
	resultCh := make(chan any, 10) // Buffer to prevent blocking

	// Start a goroutine to process the stream
	go func() {
		defer close(resultCh)
		defer resp.Body.Close()

		for {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				resultCh <- ctx.Err()
				return
			default:
			}

			// Peek to see if there's any data left
			b, err := reader.Peek(1)
			if err == io.EOF {
				break
			} else if err != nil {
				resultCh <- err
				return
			}
			// Skip empty whitespace
			if len(b) == 1 && (b[0] == '\n' || b[0] == ' ' || b[0] == '\t' || b[0] == '\r') {
				reader.ReadByte()
				continue
			}

			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				if err == io.EOF {
					break
				}
				resultCh <- err
				return
			}

			// Log the raw JSON chunk
			rawStr := string(bytes.TrimSpace(raw))
			t.logger("received chunk: %s", rawStr)
			if l != nil {
				*l = rawStr
			}

			// Unmarshal into interface{}
			var obj interface{}
			if err := json.Unmarshal(raw, &obj); err != nil {
				resultCh <- err
				return
			}
			resultCh <- obj
		}
	}()

	// Return a ChannelStreamResult
	streamResult := transports.NewChannelStreamResult(resultCh, func() error {
		// The goroutine will handle closing the response body
		return nil
	})

	return streamResult, nil
}

// CallToolStream is a new method that explicitly returns a StreamResult for streaming use cases.
// This provides a cleaner API for consumers who want to handle streaming results.
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
