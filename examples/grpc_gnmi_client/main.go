package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/grpcpb"
	_ "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	_ "github.com/universal-tool-calling-protocol/go-utcp/src/transports/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnifiedServer implements both gNMI and UTCPService servers
type UnifiedServer struct {
	grpcpb.UnimplementedUTCPServiceServer
	gnmi.UnimplementedGNMIServer
}

// === Tool service handlers ===

func (s *UnifiedServer) Capabilities(ctx context.Context, req *gnmi.CapabilityRequest) (*gnmi.CapabilityResponse, error) {
	return &gnmi.CapabilityResponse{}, nil
}

func (s *UnifiedServer) GetManual(ctx context.Context, _ *grpcpb.Empty) (*grpcpb.Manual, error) {
	return &grpcpb.Manual{
		Version: "1.2",
		Tools: []*grpcpb.Tool{
			{Name: "gnmi_subscribe", Description: "gNMI Subscribe stream"},
		},
	}, nil
}

func (s *UnifiedServer) CallTool(ctx context.Context, req *grpcpb.ToolCallRequest) (*grpcpb.ToolCallResponse, error) {
	return &grpcpb.ToolCallResponse{
		ResultJson: `{"status":"not implemented for non-streaming"}`,
	}, nil
}

func (s *UnifiedServer) CallToolStream(req *grpcpb.ToolCallRequest, stream grpcpb.UTCPService_CallToolStreamServer) error {
	ctx := stream.Context()
	if strings.Contains(req.Tool, "gnmi_subscribe") {
		var args map[string]any
		if err := json.Unmarshal([]byte(req.ArgsJson), &args); err != nil {
			return err
		}

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for i := 1; i <= 5; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				update := map[string]any{
					"timestamp": time.Now().UnixNano(),
					"path":      args["path"],
					"value":     fmt.Sprintf("mock_value_%d", i),
					"mode":      args["mode"],
				}
				b, _ := json.Marshal(update)
				if err := stream.Send(&grpcpb.ToolCallResponse{ResultJson: string(b)}); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return fmt.Errorf("tool %s not supported for streaming", req.Tool)
}

func (s *UnifiedServer) Subscribe(stream gnmi.GNMI_SubscribeServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	resp := &gnmi.SubscribeResponse{
		Response: &gnmi.SubscribeResponse_Update{
			Update: &gnmi.Notification{Update: []*gnmi.Update{{
				Path: &gnmi.Path{Element: []string{"interfaces", "interface", "eth0"}},
				Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_StringVal{StringVal: "UP"}},
			}}},
		},
	}
	return stream.Send(resp)
}

// === Auth helpers (HTTP Basic in gRPC metadata) ===

func parseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return "", "", false
	}
	s := string(decoded)
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

func first(vals []string) string {
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func validateAuth(ctx context.Context, expectedUser, expectedPass string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("missing metadata")
	}

	// Standard Authorization header
	if hs := md.Get("authorization"); len(hs) > 0 {
		if u, p, ok := parseBasicAuth(hs[0]); ok && u == expectedUser && p == expectedPass {
			return nil
		}
	}

	// Fallback metadata keys (for older clients/transports)
	if first(md.Get("username")) == expectedUser && first(md.Get("password")) == expectedPass {
		return nil
	}

	return fmt.Errorf("unauthenticated: invalid credentials")
}

// Bypass auth for GetManual to allow discovery
func shouldBypassAuth(fullMethod string) bool {
	return strings.HasSuffix(fullMethod, "/GetManual")
}

// Unary interceptor
func authUnaryInterceptor(expectedUser, expectedPass string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if shouldBypassAuth(info.FullMethod) {
			return handler(ctx, req)
		}
		if err := validateAuth(ctx, expectedUser, expectedPass); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// Stream interceptor
func authStreamInterceptor(expectedUser, expectedPass string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if shouldBypassAuth(info.FullMethod) {
			return handler(srv, ss)
		}
		if err := validateAuth(ss.Context(), expectedUser, expectedPass); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

// === Port readiness helper ===

// waitForPort dials until the TCP port is accepting connections or ctx times out.
func waitForPort(ctx context.Context, network, address string) error {
	d := net.Dialer{Timeout: 200 * time.Millisecond}
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()

	for {
		conn, err := d.DialContext(ctx, network, address)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waitForPort %s/%s: %w", network, address, ctx.Err())
		case <-t.C:
		}
	}
}

// === Server startup ===

func startGNMIServer(addr string) *grpc.Server {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	expectedUser := "testuser" // must match provider.json
	expectedPass := "testpass"

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(authUnaryInterceptor(expectedUser, expectedPass)),
		grpc.StreamInterceptor(authStreamInterceptor(expectedUser, expectedPass)),
	)

	serverImpl := &UnifiedServer{}
	gnmi.RegisterGNMIServer(srv, serverImpl)
	grpcpb.RegisterUTCPServiceServer(srv, serverImpl)

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Printf("gRPC server stopped: %v", err)
		}
	}()
	return srv
}

// === main ===

func main() {
	const serverAddr = "127.0.0.1:9339"

	srv := startGNMIServer(serverAddr)
	defer srv.Stop()

	// Block until the port is accepting connections (no arbitrary sleeps).
	ctxWait, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := waitForPort(ctxWait, "tcp", serverAddr); err != nil {
		log.Fatalf("server not reachable: %v", err)
	}

	ctx := context.Background()
	repo := repository.NewInMemoryToolRepository()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}

	client, err := utcp.NewUTCPClient(ctx, cfg, repo, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	// Tiny pause so provider registration finishes before first query.
	time.Sleep(300 * time.Millisecond)

	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search tools error: %v", err)
	}
	fmt.Println("Tools:")
	for _, t := range tools {
		fmt.Println(t.Name)
	}

	stream, err := client.CallToolStream(ctx, tools[0].Name, map[string]any{
		"path": "/interfaces/interface/eth0",
		"mode": "STREAM",
	})
	if err != nil {
		log.Fatalf("call stream: %v", err)
	}
	defer stream.Close()

	item, err := stream.Next()
	if err != nil {
		log.Fatalf("next: %v", err)
	}
	b, _ := json.MarshalIndent(item, "", "  ")
	log.Printf("Update: %s", b)
}
