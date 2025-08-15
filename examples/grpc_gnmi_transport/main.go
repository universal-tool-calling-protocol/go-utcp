package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"time"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	transports "github.com/universal-tool-calling-protocol/go-utcp/src/transports/grpc"
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

	logger := func(format string, args ...interface{}) { log.Printf(format, args...) }
	tr := transports.NewGRPCClientTransport(logger)
	prov := &providers.GRPCProvider{BaseProvider: BaseProvider{Name: "g", ProviderType: ProviderGRPC}, Host: "127.0.0.1", Port: 9339, ServiceName: "gnmi.gNMI", MethodName: "Subscribe"}

	ctx := context.Background()
	if _, err := tr.RegisterToolProvider(ctx, prov); err != nil {
		log.Fatalf("register: %v", err)
	}

	stream, err := tr.CallToolStream(ctx, "gnmi_subscribe", map[string]any{"path": "/interfaces/interface/eth0", "mode": "STREAM"}, prov)
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
