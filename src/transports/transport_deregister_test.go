package transports

import (
	"context"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/sse"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/streamable"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/sse"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/streamable"
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
