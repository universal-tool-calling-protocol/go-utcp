package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/universal-tool-calling-protocol/go-utcp/src/grpcpb"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"

	"google.golang.org/grpc"
)

// --- UnifiedServer for tests ---
type UnifiedServer struct {
	grpcpb.UnimplementedUTCPServiceServer
	gnmi.UnimplementedGNMIServer
}

func (s *UnifiedServer) Capabilities(ctx context.Context, req *gnmi.CapabilityRequest) (*gnmi.CapabilityResponse, error) {
	return &gnmi.CapabilityResponse{}, nil
}

func (s *UnifiedServer) GetManual(ctx context.Context, e *grpcpb.Empty) (*grpcpb.Manual, error) {
	return &grpcpb.Manual{
		Version: "test-1.0",
		Tools: []*grpcpb.Tool{
			{Name: "ping", Description: "simple echo"},
			{Name: "gnmi_subscribe", Description: "gNMI Subscribe stream"},
		},
	}, nil
}

func (s *UnifiedServer) CallTool(ctx context.Context, req *grpcpb.ToolCallRequest) (*grpcpb.ToolCallResponse, error) {
	var m map[string]any
	_ = json.Unmarshal([]byte(req.ArgsJson), &m)
	b, _ := json.Marshal(map[string]any{"pong": m["msg"]})
	return &grpcpb.ToolCallResponse{ResultJson: string(b)}, nil
}

func (s *UnifiedServer) Subscribe(stream gnmi.GNMI_SubscribeServer) error {
	ctx := stream.Context()
	out := make(chan *gnmi.SubscribeResponse, 8)
	defer close(out)

	// sender goroutine
	errCh := make(chan error, 1)
	go func() {
		for msg := range out {
			if err := stream.Send(msg); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	sendInterface := func(state string) {
		out <- &gnmi.SubscribeResponse{
			Response: &gnmi.SubscribeResponse_Update{
				Update: &gnmi.Notification{
					Timestamp: time.Now().UnixNano(),
					Update: []*gnmi.Update{{
						Path: &gnmi.Path{Element: []string{"interfaces", "interface", "eth0"}},
						Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_StringVal{StringVal: state}},
					}},
				},
			},
		}
	}

	mode := gnmi.SubscriptionList_STREAM

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		default:
		}

		req, err := stream.Recv()
		if err != nil {
			return err
		}
		switch r := req.Request.(type) {
		case *gnmi.SubscribeRequest_Subscribe:
			if r.Subscribe != nil {
				mode = r.Subscribe.Mode
			}
			sendInterface("UP") // ack
		case *gnmi.SubscribeRequest_Poll:
			if mode == gnmi.SubscriptionList_POLL {
				sendInterface("UP")
			}
		}
	}
}

func startUnifiedServer(t *testing.T) (*grpc.Server, *GRPCProvider) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	us := &UnifiedServer{}
	gnmi.RegisterGNMIServer(srv, us)
	grpcpb.RegisterUTCPServiceServer(srv, us)
	go srv.Serve(lis)
	prov := &GRPCProvider{
		BaseProvider: BaseProvider{Name: "g", ProviderType: ProviderGRPC},
		Host:         "127.0.0.1",
		Port:         lis.Addr().(*net.TCPAddr).Port,
		ServiceName:  "gnmi.gNMI",
		MethodName:   "Subscribe",
	}
	return srv, prov
}

// --- Tests ---

func TestGRPCTransport_RegisterAndCall(t *testing.T) {
	srv, prov := startUnifiedServer(t)
	defer srv.Stop()

	tr := NewGRPCClientTransport(nil)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	foundPing := false
	for _, tool := range tools {
		if tool.Name == "ping" {
			foundPing = true
			break
		}
	}
	if !foundPing {
		t.Fatalf("ping tool not found in %v", tools)
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
	badProv := &GRPCProvider{
		BaseProvider: BaseProvider{Name: "g", ProviderType: ProviderGRPC},
		Host:         "127.0.0.1",
		Port:         1,
		UseSSL:       true,
	}
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

func TestUnifiedServer_GNMISubscribe(t *testing.T) {
	srv, prov := startUnifiedServer(t)
	defer srv.Stop()

	tr := NewGRPCClientTransport(nil)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	found := false
	for _, tool := range tools {
		if tool.Name == "gnmi_subscribe" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected gnmi_subscribe tool, got %v", tools)
	}

	stream, err := tr.CallToolStream(ctx, "gnmi_subscribe",
		map[string]any{"path": "/interfaces/interface/eth0", "mode": "STREAM"}, prov)
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
