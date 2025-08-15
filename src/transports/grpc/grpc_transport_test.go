package grpc

import (
	"context"
	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"
	"net"
	"testing"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/universal-tool-calling-protocol/go-utcp/src/grpcpb"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"

	"google.golang.org/grpc"
)

type dummyGRPCServer struct {
	grpcpb.UnimplementedUTCPServiceServer
}

func (s *dummyGRPCServer) GetManual(ctx context.Context, _ *grpcpb.Empty) (*grpcpb.Manual, error) {
	return &grpcpb.Manual{Version: "1.0", Tools: []*grpcpb.Tool{{Name: "ping"}}}, nil
}

func (s *dummyGRPCServer) CallTool(ctx context.Context, req *grpcpb.ToolCallRequest) (*grpcpb.ToolCallResponse, error) {
	var m map[string]any
	_ = json.Unmarshal([]byte(req.ArgsJson), &m)
	b, _ := json.Marshal(map[string]any{"pong": m["msg"]})
	return &grpcpb.ToolCallResponse{ResultJson: string(b)}, nil
}

func startDummyGRPC(t *testing.T) (*grpc.Server, *GRPCProvider) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	grpcpb.RegisterUTCPServiceServer(srv, &dummyGRPCServer{})
	go srv.Serve(lis)
	prov := &GRPCProvider{BaseProvider: BaseProvider{Name: "g", ProviderType: ProviderGRPC}, Host: "127.0.0.1", Port: lis.Addr().(*net.TCPAddr).Port}
	return srv, prov
}

func TestGRPCTransport_RegisterAndCall(t *testing.T) {
	srv, prov := startDummyGRPC(t)
	defer srv.Stop()
	tr := NewGRPCClientTransport(nil)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil || len(tools) != 1 || tools[0].Name != "ping" {
		t.Fatalf("register error: %v tools:%v", err, tools)
	}
	res, err := tr.CallTool(ctx, "ping", map[string]any{"msg": "hi"}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok || m["pong"] != "hi" {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestGRPCTransport_Errors(t *testing.T) {
	tr := NewGRPCClientTransport(nil)
	badProv := &GRPCProvider{BaseProvider: BaseProvider{Name: "g", ProviderType: ProviderGRPC}, Host: "127.0.0.1", Port: 1, UseSSL: true}
	_, err := tr.RegisterToolProvider(context.Background(), badProv)
	if err == nil {
		t.Fatal("expected error for SSL")
	}
	if err := tr.DeregisterToolProvider(context.Background(), &HttpProvider{}); err == nil {
		t.Fatal("expected type error")
	}
	if _, err := tr.CallTool(context.Background(), "ping", nil, &HttpProvider{}, nil); err == nil {
		t.Fatal("expected type error")
	}
}

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
	if err := stream.Send(resp); err != nil {
		return err
	}
	return nil
}

func startDummyGNMI(t *testing.T) (*grpc.Server, *GRPCProvider) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	gnmi.RegisterGNMIServer(srv, &dummyGNMIServer{})
	go srv.Serve(lis)
	prov := &GRPCProvider{BaseProvider: BaseProvider{Name: "g", ProviderType: ProviderGRPC}, Host: "127.0.0.1", Port: lis.Addr().(*net.TCPAddr).Port, ServiceName: "gnmi.gNMI", MethodName: "Subscribe"}
	return srv, prov
}

func TestGRPCTransport_GNMISubscribe(t *testing.T) {
	srv, prov := startDummyGNMI(t)
	defer srv.Stop()
	tr := NewGRPCClientTransport(nil)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "gnmi_subscribe" {
		t.Fatalf("expected gnmi_subscribe tool, got %v", tools)
	}
	stream, err := tr.CallToolStream(ctx, "gnmi_subscribe", map[string]any{"path": "/interfaces/interface/eth0", "mode": "STREAM"}, prov)
	if err != nil {
		t.Fatalf("call stream error: %v", err)
	}
	defer stream.Close()
	item, err := stream.Next()
	if err != nil {
		t.Fatalf("next error: %v", err)
	}
	m, ok := item.(map[string]any)
	if !ok {
		t.Fatalf("unexpected type: %T", item)
	}
	if _, ok := m["update"]; !ok {
		t.Fatalf("expected update field in response: %#v", m)
	}
}
