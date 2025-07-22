package utcp

import (
	"context"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/concepts/providers"

	. "github.com/universal-tool-calling-protocol/go-utcp/concepts/transports/sse"
	. "github.com/universal-tool-calling-protocol/go-utcp/concepts/transports/streamable"
)

func TestSSEAndStreamableDeregister(t *testing.T) {
	sse := NewSSETransport(nil)
	sh := &SSEProvider{BaseProvider: BaseProvider{Name: "s", ProviderType: ProviderSSE}}
	if err := sse.DeregisterToolProvider(context.Background(), sh); err != nil {
		t.Fatalf("sse deregister error: %v", err)
	}

	stream := NewStreamableHTTPTransport(nil)
	sth := &StreamableHttpProvider{BaseProvider: BaseProvider{Name: "h", ProviderType: ProviderHTTPStream}}
	if err := stream.DeregisterToolProvider(context.Background(), sth); err != nil {
		t.Fatalf("stream deregister error: %v", err)
	}
}
