package helpers

import (
	"encoding/json"
	"fmt"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/cli"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/graphql"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/sse"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/streamable"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/tcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/text"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/udp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/webrtc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/websocket"
)

// UnmarshalProvider inspects "provider_type" and returns the right struct.
func UnmarshalProvider(data []byte) (Provider, error) {
	var base struct {
		ProviderType ProviderType `json:"provider_type"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, err
	}

	switch base.ProviderType {
	case ProviderHTTP:
		return http.UnmarshalHttpProvider(data)
	case ProviderSSE:
		return UnmarshalSSEProvider(data)
	case ProviderHTTPStream:
		return UnmarshalStreamableHttpProvider(data)
	case ProviderCLI:
		p := &CliProvider{}
		if err := json.Unmarshal(data, p); err != nil {
			return nil, err
		}
		return p, nil
	case ProviderWebSocket:
		return UnmarshalWebSocketProvider(data)
	case ProviderGRPC:
		return UnmarshalGRPCProvider(data)
	case ProviderGraphQL:
		return UnmarshalGraphQLProvider(data)
	case ProviderTCP:
		p := &TCPProvider{}
		if err := json.Unmarshal(data, p); err != nil {
			return nil, err
		}
		return p, nil
	case ProviderUDP:
		p := &UDPProvider{}
		if err := json.Unmarshal(data, p); err != nil {
			return nil, err
		}
		return p, nil
	case ProviderWebRTC:
		p := &WebRTCProvider{}
		if err := json.Unmarshal(data, p); err != nil {
			return nil, err
		}
		return p, nil
	case ProviderMCP:
		p := &MCPProvider{}
		if err := json.Unmarshal(data, p); err != nil {
			return nil, err
		}
		return p, nil
	case ProviderText:
		return UnmarshalTextProvider(data)
	default:
		return nil, fmt.Errorf("unsupported provider_type %q", base.ProviderType)
	}
}
