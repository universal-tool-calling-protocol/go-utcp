package grpc

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/universal-tool-calling-protocol/go-utcp/grpcpb"

	. "github.com/universal-tool-calling-protocol/go-utcp/internal/providers"
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
