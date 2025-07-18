package UTCP

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

	return decodeToolsResponse(resp.Body)
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
	defer resp.Body.Close()

	// Use a buffered reader to handle NDJSON or JSON Sequence
	reader := bufio.NewReader(resp.Body)
	dec := json.NewDecoder(reader)

	// Collect parsed chunks
	var results []interface{}
	for {
		// Peek to see if there's any data left
		b, err := reader.Peek(1)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
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
			return nil, err
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
			return nil, err
		}
		results = append(results, obj)
	}

	// Return single element directly
	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}

// DeregisterToolProvider clears any streaming-specific state (no-op).
func (t *StreamableHTTPClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	return nil
}
