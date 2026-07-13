package sse

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
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

type closeCountingReader struct {
	io.Reader
	closed chan struct{}
}

func (r *closeCountingReader) Close() error {
	r.closed <- struct{}{}
	return nil
}

func TestSSEStreamCloseUnblocksFullBuffer(t *testing.T) {
	closed := make(chan struct{}, 2)
	body := &closeCountingReader{
		Reader: strings.NewReader(strings.Repeat("data: {\"n\":1}\n\n", 32)),
		closed: closed,
	}
	stream, err := NewSSETransport(nil).handleSSEStream(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("stream producer did not exit after Close")
		}
	}
}
