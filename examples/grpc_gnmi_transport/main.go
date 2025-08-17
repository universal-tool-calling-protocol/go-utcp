package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
	auth "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	"github.com/universal-tool-calling-protocol/go-utcp/src/grpcpb"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	transports "github.com/universal-tool-calling-protocol/go-utcp/src/transports/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type UnifiedServer struct {
	gnmi.UnimplementedGNMIServer
	grpcpb.UnimplementedUTCPServiceServer
}

const (
	user = "alice"
	pass = "secret"
)

func authFromContext(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("missing metadata")
	}
	u := md.Get("username")
	p := md.Get("password")
	if len(u) == 0 || len(p) == 0 || u[0] != user || p[0] != pass {
		return fmt.Errorf("unauthorized")
	}
	return nil
}

func unaryAuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if err := authFromContext(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func streamAuthInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := authFromContext(ss.Context()); err != nil {
		return err
	}
	return handler(srv, ss)
}

func (s *UnifiedServer) CallTool(ctx context.Context, req *grpcpb.ToolCallRequest) (*grpcpb.ToolCallResponse, error) {
	// Simple implementation - could be expanded based on tool name
	return &grpcpb.ToolCallResponse{
		ResultJson: `{"status": "not implemented for non-streaming"}`,
	}, nil
}

func (s *UnifiedServer) CallToolStream(req *grpcpb.ToolCallRequest, stream grpcpb.UTCPService_CallToolStreamServer) error {
	ctx := stream.Context()

	if req.Tool == "gnmi_subscribe" {
		// Parse args from JSON
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(req.ArgsJson), &args); err != nil {
			return err
		}

		// Create a mock gNMI-like streaming response
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		counter := 0
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				counter++

				// Create a mock update response
				update := map[string]interface{}{
					"timestamp":          time.Now().UnixNano(),
					"path":               args["path"],
					"value":              fmt.Sprintf("mock_value_%d", counter),
					"mode":               args["mode"],
					"sub_mode":           args["sub_mode"],
					"sample_interval_ns": args["sample_interval_ns"],
				}

				updateJson, err := json.Marshal(update)
				if err != nil {
					return err
				}

				response := &grpcpb.ToolCallResponse{
					ResultJson: string(updateJson),
				}

				if err := stream.Send(response); err != nil {
					return err
				}

				// For demo purposes, send a few updates then stop
				if counter >= 5 {
					return nil
				}
			}
		}
	}

	// For other tools, return an error or empty response
	return fmt.Errorf("tool %s not supported for streaming", req.Tool)
}

func (s *UnifiedServer) Capabilities(ctx context.Context, req *gnmi.CapabilityRequest) (*gnmi.CapabilityResponse, error) {
	return &gnmi.CapabilityResponse{}, nil
}

func (s *UnifiedServer) GetManual(ctx context.Context, e *grpcpb.Empty) (*grpcpb.Manual, error) {
	return &grpcpb.Manual{
		Version: "1.2",
		Tools: []*grpcpb.Tool{
			{Name: "gnmi_subscribe", Description: "gNMI Subscribe stream"},
		},
	}, nil
}

func (s *UnifiedServer) Subscribe(stream gnmi.GNMI_SubscribeServer) error {
	ctx := stream.Context()

	if _, err := stream.Recv(); err != nil {
		return err
	}
	resp := &gnmi.SubscribeResponse{
		Response: &gnmi.SubscribeResponse_Update{
			Update: &gnmi.Notification{Update: []*gnmi.Update{{
				Path: &gnmi.Path{Elem: []*gnmi.PathElem{
					{Name: "interfaces"},
					{Name: "interface", Key: map[string]string{"name": "eth0"}},
				}},
				Val: &gnmi.TypedValue{Value: &gnmi.TypedValue_StringVal{StringVal: "UP"}},
			}}},
		},
	}
	return stream.Send(resp)
}

func startGNMIServer(addr string) *grpc.Server {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(unaryAuthInterceptor),
		grpc.StreamInterceptor(streamAuthInterceptor),
	)
	gnmi.RegisterGNMIServer(srv, &UnifiedServer{})
	grpcpb.RegisterUTCPServiceServer(srv, &UnifiedServer{})
	go srv.Serve(lis)
	return srv
}

func main() {
	srv := startGNMIServer("127.0.0.1:9339")
	defer srv.Stop()
	time.Sleep(200 * time.Millisecond)

	logger := func(format string, args ...interface{}) { log.Printf(format, args...) }
	tr := transports.NewGRPCClientTransport(logger)
	var a auth.Auth = auth.NewBasicAuth(user, pass)
	prov := &providers.GRPCProvider{BaseProvider: BaseProvider{Name: "g", ProviderType: ProviderGRPC}, Host: "127.0.0.1", Port: 9339, ServiceName: "gnmi.gNMI", MethodName: "Subscribe", Auth: &a}

	ctx := context.Background()
	if _, err := tr.RegisterToolProvider(ctx, prov); err != nil {
		log.Fatalf("register: %v", err)
	}

	stream, err := tr.CallToolStream(ctx, "gnmi_subscribe", map[string]any{"path": "/interfaces/interface[name=eth0]", "mode": "STREAM", "sub_mode": "SAMPLE", "sample_interval_ns": 500000000}, prov)
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
