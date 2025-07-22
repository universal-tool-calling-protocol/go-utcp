package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"strconv"
	"time"

	providers "github.com/universal-tool-calling-protocol/go-utcp/internal/providers"
	utcp "github.com/universal-tool-calling-protocol/go-utcp/internal/transports/grpc"

	"github.com/universal-tool-calling-protocol/go-utcp/internal/grpcpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type server struct {
	grpcpb.UnimplementedUTCPServiceServer
}

func (s *server) GetManual(ctx context.Context, e *grpcpb.Empty) (*grpcpb.Manual, error) {
	return &grpcpb.Manual{Version: "1.0", Tools: []*grpcpb.Tool{{Name: "echo", Description: "Echo"}}}, nil
}

func (s *server) CallTool(ctx context.Context, req *grpcpb.ToolCallRequest) (*grpcpb.ToolCallResponse, error) {
	var args map[string]any
	_ = json.Unmarshal([]byte(req.ArgsJson), &args)
	msg, _ := args["msg"].(string)
	out, _ := json.Marshal(map[string]any{"result": msg})
	return &grpcpb.ToolCallResponse{ResultJson: string(out)}, nil
}

func startServer() (net.Listener, *grpc.Server) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	grpcpb.RegisterUTCPServiceServer(s, &server{})
	reflection.Register(s)
	go s.Serve(lis)
	return lis, s
}

func main() {
	lis, srv := startServer()
	defer srv.Stop()
	_, port, _ := net.SplitHostPort(lis.Addr().String())

	time.Sleep(200 * time.Millisecond)

	logger := func(format string, args ...interface{}) { log.Printf(format, args...) }
	transport := utcp.NewGRPCClientTransport(logger)
	prov := &providers.GRPCProvider{BaseProvider: providers.BaseProvider{Name: "grpc", ProviderType: providers.ProviderGRPC}, Host: "127.0.0.1", Port: atoi(port)}

	ctx := context.Background()
	tools, err := transport.RegisterToolProvider(ctx, prov)
	if err != nil {
		log.Fatalf("register: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	res, err := transport.CallTool(ctx, "echo", map[string]any{"msg": "hi"}, prov, nil)
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	log.Printf("Result: %#v", res)

	_ = transport.DeregisterToolProvider(ctx, prov)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
