package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"

	"google.golang.org/grpc"
)

type dummyGNMIServer struct {
	gnmi.UnimplementedGNMIServer
}

func (s *dummyGNMIServer) Capabilities(ctx context.Context, req *gnmi.CapabilityRequest) (*gnmi.CapabilityResponse, error) {
	return &gnmi.CapabilityResponse{}, nil
}

func (s *dummyGNMIServer) Subscribe(stream gnmi.GNMI_SubscribeServer) error {
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

func startGNMIServer(addr string) *grpc.Server {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	gnmi.RegisterGNMIServer(srv, &dummyGNMIServer{})
	go srv.Serve(lis)
	return srv
}

func main() {
	srv := startGNMIServer("127.0.0.1:9339")
	defer srv.Stop()
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	repo := repository.NewInMemoryToolRepository()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, repo, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}
	tools, err := client.SearchTools("", 10)
	fmt.Println("Tools:")
	for _, tool := range tools {
		fmt.Println(tool.Name)
	}

	stream, err := client.CallToolStream(ctx, "gnmi.gnmi_subscribe", map[string]any{
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
