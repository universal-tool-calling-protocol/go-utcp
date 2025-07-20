package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/grpcpb"
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

func startServer(addr string) *grpc.Server {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	grpcpb.RegisterUTCPServiceServer(s, &server{})
	reflection.Register(s)
	go s.Serve(lis)
	return s
}

func main() {
	srv := startServer("127.0.0.1:9090")
	defer srv.Stop()
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	res, err := client.CallTool(ctx, "grpc.echo", map[string]any{"msg": "hi"})
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	log.Printf("Result: %#v", res)
}
