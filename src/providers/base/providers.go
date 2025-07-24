package base

type ProviderType string

const (
	ProviderHTTP       ProviderType = "http"
	ProviderSSE        ProviderType = "sse"
	ProviderHTTPStream ProviderType = "http_stream"
	ProviderCLI        ProviderType = "cli"
	ProviderWebSocket  ProviderType = "websocket"
	ProviderGRPC       ProviderType = "grpc"
	ProviderGraphQL    ProviderType = "graphql"
	ProviderTCP        ProviderType = "tcp"
	ProviderUDP        ProviderType = "udp"
	ProviderWebRTC     ProviderType = "webrtc"
	ProviderMCP        ProviderType = "mcp"
)

// Provider is implemented by all concrete provider types.
type Provider interface {
	// Type returns the discriminator.
	Type() ProviderType
}

// BaseProvider holds fields common to every provider.
type BaseProvider struct {
	Name         string       `json:"name"`
	ProviderType ProviderType `json:"provider_type"`
}

func (b *BaseProvider) Type() ProviderType {
	return b.ProviderType
}
