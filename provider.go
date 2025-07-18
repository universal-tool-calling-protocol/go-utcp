package utcp

import (
	"encoding/json"
	"fmt"
)

// ProviderType is the kind of provider.
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
	ProviderText       ProviderType = "text"
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

// HttpProvider represents RESTful HTTP/HTTPS API.
type HttpProvider struct {
	BaseProvider
	HTTPMethod   string            `json:"http_method"` // GET, POST, PUT, DELETE, PATCH
	URL          string            `json:"url"`
	ContentType  string            `json:"content_type"` // default application/json
	Auth         *Auth             `json:"auth,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyField    *string           `json:"body_field,omitempty"` // name of the single input field
	HeaderFields []string          `json:"header_fields,omitempty"`
}

// SSEProvider represents Server-Sent Events.
type SSEProvider struct {
	BaseProvider
	URL          string            `json:"url"`
	EventType    *string           `json:"event_type,omitempty"`
	Reconnect    bool              `json:"reconnect"`     // default true
	RetryTimeout int               `json:"retry_timeout"` // ms, default 30000
	Auth         *Auth             `json:"auth,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyField    *string           `json:"body_field,omitempty"`
	HeaderFields []string          `json:"header_fields,omitempty"`
}

// StreamableHttpProvider is HTTP chunked transfer encoding.
type StreamableHttpProvider struct {
	BaseProvider
	URL          string            `json:"url"`
	HTTPMethod   string            `json:"http_method"`  // GET, POST
	ContentType  string            `json:"content_type"` // default application/octet-stream
	ChunkSize    int               `json:"chunk_size"`   // bytes, default 4096
	Timeout      int               `json:"timeout"`      // ms, default 60000
	Headers      map[string]string `json:"headers,omitempty"`
	Auth         *Auth             `json:"auth,omitempty"`
	BodyField    *string           `json:"body_field,omitempty"`
	HeaderFields []string          `json:"header_fields,omitempty"`
}

// CliProvider represents a CLI tool.
type CliProvider struct {
	BaseProvider
	CommandName string            `json:"command_name"`
	EnvVars     map[string]string `json:"env_vars,omitempty"`
	WorkingDir  *string           `json:"working_dir,omitempty"`
	// auth is always nil
}

// WebSocketProvider represents a WebSocket connection.
type WebSocketProvider struct {
	BaseProvider
	URL          string            `json:"url"`
	Protocol     *string           `json:"protocol,omitempty"`
	KeepAlive    bool              `json:"keep_alive"`
	Auth         *Auth             `json:"auth,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	HeaderFields []string          `json:"header_fields,omitempty"`
}

// GRPCProvider represents a gRPC service.
type GRPCProvider struct {
	BaseProvider
	Host        string `json:"host"`
	Port        int    `json:"port"`
	ServiceName string `json:"service_name"`
	MethodName  string `json:"method_name"`
	UseSSL      bool   `json:"use_ssl"`
	Auth        *Auth  `json:"auth,omitempty"`
}

// GraphQLProvider represents a GraphQL endpoint.
type GraphQLProvider struct {
	BaseProvider
	URL           string            `json:"url"`
	OperationType string            `json:"operation_type"` // query, mutation, subscription
	OperationName *string           `json:"operation_name,omitempty"`
	Auth          *Auth             `json:"auth,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	HeaderFields  []string          `json:"header_fields,omitempty"`
}

// TCPProvider represents a raw TCP socket.
type TCPProvider struct {
	BaseProvider
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Timeout int    `json:"timeout"` // ms, default 30000
	// auth always nil
}

// UDPProvider represents a UDP socket.
type UDPProvider struct {
	BaseProvider
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Timeout int    `json:"timeout"`
	// auth always nil
}

// WebRTCProvider represents a WebRTC data channel.
type WebRTCProvider struct {
	BaseProvider
	SignalingServer string `json:"signaling_server"`
	PeerID          string `json:"peer_id"`
	DataChannelName string `json:"data_channel_name"`
	// auth always nil
}

// McpStdioServer config for stdio transport.
type McpStdioServer struct {
	Transport string            `json:"transport"` // always "stdio"
	Command   string            `json:"command"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// McpHttpServer config for HTTP transport.
type McpHttpServer struct {
	Transport string `json:"transport"` // always "http"
	URL       string `json:"url"`
}

// McpServer is a union of the two MCP transports.
type McpServer interface{}
type McpConfig struct {
	McpServers map[string]McpServer `json:"mcpServers"`
}

// TextProvider reads tool defs from a file.
type TextProvider struct {
	BaseProvider
	FilePath string `json:"file_path"`
	// auth always nil
}

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
		return unmarshalHttpProvider(data)
	case ProviderSSE:
		return unmarshalSSEProvider(data)
	case ProviderHTTPStream:
		return unmarshalStreamableHttpProvider(data)
	case ProviderCLI:
		p := &CliProvider{}
		if err := json.Unmarshal(data, p); err != nil {
			return nil, err
		}
		return p, nil
	case ProviderWebSocket:
		return unmarshalWebSocketProvider(data)
	case ProviderGRPC:
		return unmarshalGRPCProvider(data)
	case ProviderGraphQL:
		return unmarshalGraphQLProvider(data)
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
		p := &TextProvider{}
		if err := json.Unmarshal(data, p); err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unsupported provider_type %q", base.ProviderType)
	}
}

func unmarshalHttpProvider(data []byte) (*HttpProvider, error) {
	type Alias HttpProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&HttpProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	hp := (*HttpProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := unmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		hp.Auth = &auth
	}
	return hp, nil
}

func unmarshalSSEProvider(data []byte) (*SSEProvider, error) {
	type Alias SSEProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&SSEProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	sp := (*SSEProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := unmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		sp.Auth = &auth
	}
	return sp, nil
}

func unmarshalStreamableHttpProvider(data []byte) (*StreamableHttpProvider, error) {
	type Alias StreamableHttpProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&StreamableHttpProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	sp := (*StreamableHttpProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := unmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		sp.Auth = &auth
	}
	return sp, nil
}

func unmarshalWebSocketProvider(data []byte) (*WebSocketProvider, error) {
	type Alias WebSocketProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&WebSocketProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	wp := (*WebSocketProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := unmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		wp.Auth = &auth
	}
	return wp, nil
}

func unmarshalGRPCProvider(data []byte) (*GRPCProvider, error) {
	type Alias GRPCProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&GRPCProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	gp := (*GRPCProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := unmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		gp.Auth = &auth
	}
	return gp, nil
}

func unmarshalGraphQLProvider(data []byte) (*GraphQLProvider, error) {
	type Alias GraphQLProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&GraphQLProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	gp := (*GraphQLProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := unmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		gp.Auth = &auth
	}
	return gp, nil
}

func unmarshalAuth(data []byte) (Auth, error) {
	var base struct {
		AuthType AuthType `json:"auth_type"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, err
	}
	switch base.AuthType {
	case APIKeyType:
		var a ApiKeyAuth
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, err
		}
		return &a, nil
	case BasicType:
		var a BasicAuth
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, err
		}
		return &a, nil
	case OAuth2Type:
		var a OAuth2Auth
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, err
		}
		return &a, nil
	default:
		return nil, fmt.Errorf("unsupported auth_type %q", base.AuthType)
	}
}

// (Assuming Auth, ApiKeyAuth, BasicAuth, OAuth2Auth come from your shared/auth package)
