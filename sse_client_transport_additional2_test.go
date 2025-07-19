package utcp

import (
	"io"
	"strings"
	"testing"
)

func TestHandleSSE(t *testing.T) {
	data := "id:1\n" +
		"data: {\"a\":1}\n\n" +
		"data: {\"b\":2}\n\n"
	tr := NewSSETransport(nil)
	events, err := tr.handleSSE(io.NopCloser(strings.NewReader(data)))
	if err != nil {
		t.Fatalf("handleSSE error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}
