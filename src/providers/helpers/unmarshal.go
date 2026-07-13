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

// ProviderName returns the configured name for a built-in provider.
func ProviderName(provider Provider) (string, bool) {
	switch p := provider.(type) {
	case *CliProvider:
		return p.Name, true
	case *http.HttpProvider:
		return p.Name, true
	case *SSEProvider:
		return p.Name, true
	case *StreamableHttpProvider:
		return p.Name, true
	case *WebSocketProvider:
		return p.Name, true
	case *GRPCProvider:
		return p.Name, true
	case *GraphQLProvider:
		return p.Name, true
	case *TCPProvider:
		return p.Name, true
	case *UDPProvider:
		return p.Name, true
	case *WebRTCProvider:
		return p.Name, true
	case *MCPProvider:
		return p.Name, true
	case *TextProvider:
		return p.Name, true
	default:
		return "", false
	}
}

// SetProviderName updates the configured name of a built-in provider.
func SetProviderName(provider Provider, name string) bool {
	switch p := provider.(type) {
	case *CliProvider:
		p.Name = name
	case *http.HttpProvider:
		p.Name = name
	case *SSEProvider:
		p.Name = name
	case *StreamableHttpProvider:
		p.Name = name
	case *WebSocketProvider:
		p.Name = name
	case *GRPCProvider:
		p.Name = name
	case *GraphQLProvider:
		p.Name = name
	case *TCPProvider:
		p.Name = name
	case *UDPProvider:
		p.Name = name
	case *WebRTCProvider:
		p.Name = name
	case *MCPProvider:
		p.Name = name
	case *TextProvider:
		p.Name = name
	default:
		return false
	}
	return true
}

// NewProvider returns an empty built-in provider for the requested type.
func NewProvider(providerType ProviderType) (Provider, error) {
	base := BaseProvider{ProviderType: providerType}
	switch providerType {
	case ProviderHTTP:
		return &http.HttpProvider{BaseProvider: base}, nil
	case ProviderSSE:
		return &SSEProvider{BaseProvider: base}, nil
	case ProviderHTTPStream:
		return &StreamableHttpProvider{BaseProvider: base}, nil
	case ProviderCLI:
		return &CliProvider{BaseProvider: base}, nil
	case ProviderWebSocket:
		return &WebSocketProvider{BaseProvider: base}, nil
	case ProviderGRPC:
		return &GRPCProvider{BaseProvider: base}, nil
	case ProviderGraphQL:
		return &GraphQLProvider{BaseProvider: base}, nil
	case ProviderTCP:
		return &TCPProvider{BaseProvider: base}, nil
	case ProviderUDP:
		return &UDPProvider{BaseProvider: base}, nil
	case ProviderWebRTC:
		return &WebRTCProvider{BaseProvider: base}, nil
	case ProviderMCP:
		return &MCPProvider{}, nil
	case ProviderText:
		return &TextProvider{BaseProvider: base}, nil
	default:
		return nil, fmt.Errorf("unsupported provider type %q", providerType)
	}
}
