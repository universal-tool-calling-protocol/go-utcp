package grpc

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/universal-tool-calling-protocol/go-utcp/src/grpcpb"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"

	"google.golang.org/grpc"
)

// ------------------------------------------------------------
// Unified gRPC test server: UTCPService + gNMI
// ------------------------------------------------------------

type UnifiedServer struct {
	grpcpb.UnimplementedUTCPServiceServer
	gnmi.UnimplementedGNMIServer

	firstPrefixTarget atomic.Value // string
}

func (s *UnifiedServer) Capabilities(ctx context.Context, _ *gnmi.CapabilityRequest) (*gnmi.CapabilityResponse, error) {
	return &gnmi.CapabilityResponse{}, nil
}

func (s *UnifiedServer) GetManual(ctx context.Context, _ *grpcpb.Empty) (*grpcpb.Manual, error) {
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

// CallToolStream implements server-streaming UTCP tool responses for tests.
// It supports:
//   - tool "gnmi_subscribe": emits JSON objects resembling interface oper-status updates
//   - any other tool: emits simple ping-style JSON with a seq counter
//
// Args supported (optional):
//   - mode: "ONCE" | "STREAM" | "POLL" (default STREAM)
//   - count: number of messages for ONCE/POLL (default 3; ONCE always 1)
//   - interval_ms: delay between messages (default 30ms)
func (s *UnifiedServer) CallToolStream(req *grpcpb.ToolCallRequest, stream grpcpb.UTCPService_CallToolStreamServer) error {
	ctx := stream.Context()
	var args map[string]any
	_ = json.Unmarshal([]byte(req.ArgsJson), &args)

	// Special handling: emulate gNMI Subscribe semantics when tool == "gnmi_subscribe".
	if req.Tool == "gnmi_subscribe" {
		path, _ := args["path"].(string)
		if path == "" {
			path = "/interfaces/interface[name=eth0]/state/oper-status"
		}
		mode, _ := args["mode"].(string)
		if mode == "" {
			mode = "STREAM"
		}

		// Prefer poll_every_ms for POLL; otherwise fall back to interval_ms.
		pollEvery := 0
		if v, ok := args["poll_every_ms"].(float64); ok && v > 0 {
			pollEvery = int(v)
		}
		interval := 30 * time.Millisecond
		if pollEvery > 0 {
			interval = time.Duration(pollEvery) * time.Millisecond
		} else if v, ok := args["interval_ms"].(float64); ok && v > 0 {
			interval = time.Duration(int(v)) * time.Millisecond
		}

		send := func(i int) error {
			payload := map[string]any{
				"seq":   i,
				"path":  path,
				"value": "UP",
				"ts":    time.Now().UnixNano(),
			}
			b, _ := json.Marshal(payload)
			return stream.Send(&grpcpb.ToolCallResponse{ResultJson: string(b)})
		}

		switch mode {
		case "ONCE":
			return send(0)
		case "POLL":
			// Emit periodic polled updates until client cancels (the client/transport may be pumping POLL).
			t := time.NewTicker(interval)
			defer t.Stop()
			for i := 0; ; i++ {
				if err := send(i); err != nil {
					return err
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-t.C:
				}
			}
		default: // STREAM
			t := time.NewTicker(interval)
			defer t.Stop()
			for i := 0; ; i++ {
				if err := send(i); err != nil {
					return err
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-t.C:
				}
			}
		}
	}

	// Generic fallback for other tools: ping-style JSON with seq counter.
	mode, _ := args["mode"].(string)
	if mode == "" {
		mode = "STREAM"
	}
	cnt := 3
	if v, ok := args["count"].(float64); ok && v > 0 {
		cnt = int(v)
	}
	interval := 30 * time.Millisecond
	if v, ok := args["interval_ms"].(float64); ok && v > 0 {
		interval = time.Duration(int(v)) * time.Millisecond
	}

	send := func(i int) error {
		payload := map[string]any{"seq": i, "tool": req.Tool, "ts": time.Now().UnixNano()}
		b, _ := json.Marshal(payload)
		return stream.Send(&grpcpb.ToolCallResponse{ResultJson: string(b)})
	}

	switch mode {
	case "ONCE":
		return send(0)
	case "POLL":
		if cnt < 2 {
			cnt = 2
		}
		t := time.NewTicker(interval)
		defer t.Stop()
		for i := 0; i < cnt; i++ {
			if err := send(i); err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
			}
		}
		return nil
	default: // STREAM
		t := time.NewTicker(interval)
		defer t.Stop()
		for i := 0; ; i++ {
			if err := send(i); err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
			}
		}
	}
}

func (s *UnifiedServer) Subscribe(stream gnmi.GNMI_SubscribeServer) error {
	ctx := stream.Context()
	mode := gnmi.SubscriptionList_STREAM
	first := true

	send := func(state string) error {
		resp := &gnmi.SubscribeResponse{
			Response: &gnmi.SubscribeResponse_Update{
				Update: &gnmi.Notification{
					Timestamp: time.Now().UnixNano(),
					Update: []*gnmi.Update{{
						Path: &gnmi.Path{Elem: []*gnmi.PathElem{{Name: "interfaces"}, {Name: "interface", Key: map[string]string{"name": "eth0"}}, {Name: "state"}, {Name: "oper-status"}}},
						Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_StringVal{StringVal: state}},
					}},
				},
			},
		}
		return stream.Send(resp)
	}

	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		switch r := req.Request.(type) {
		case *gnmi.SubscribeRequest_Subscribe:
			if r.Subscribe != nil {
				mode = r.Subscribe.Mode
				if first {
					first = false
					if pfx := r.Subscribe.Prefix; pfx != nil && pfx.Target != "" {
						s.firstPrefixTarget.Store(pfx.Target)
					}
				}
			}
			// Send an initial update immediately.
			if err := send("UP"); err != nil {
				return err
			}

			switch mode {
			case gnmi.SubscriptionList_ONCE:
				// End the stream after the first batch, like a device would after SyncResponse.
				return nil

			case gnmi.SubscriptionList_STREAM:
				// Keep sending periodic updates until the client cancels.
				t := time.NewTicker(30 * time.Millisecond)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-t.C:
						if err := send("UP"); err != nil {
							return err
						}
					}
				}

			case gnmi.SubscriptionList_POLL:
				// Do nothing here; wait for Poll messages handled below.
			}

		case *gnmi.SubscribeRequest_Poll:
			if mode == gnmi.SubscriptionList_POLL {
				if err := send("UP"); err != nil {
					return err
				}
			}
		}
	}
}

func startUnifiedServer(t *testing.T) (*grpc.Server, *GRPCProvider, *UnifiedServer) {
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
	return srv, prov, us
}

// ------------------------------------------------------------
// Tests: CallToolStream (gNMI path)
// ------------------------------------------------------------

func TestGNMI_CallToolStream_ONCE_Ends(t *testing.T) {
	srv, prov, _ := startUnifiedServer(t)
	defer srv.Stop()

	tr := NewGRPCClientTransport(nil)
	ctx := context.Background()

	stream, err := tr.CallToolStream(ctx, "gnmi_subscribe", map[string]any{
		"path": "/interfaces/interface[name=eth0]/state/oper-status",
		"mode": "ONCE",
	}, prov)
	if err != nil {
		t.Fatalf("CallToolStream: %v", err)
	}
	defer stream.Close()

	if _, err := stream.Next(); err != nil {
		t.Fatalf("first Next: %v", err)
	}
	// After ONCE, subsequent Next should return an error/EOF promptly.
	if _, err := stream.Next(); err == nil {
		t.Fatalf("expected EOF/error after ONCE completion")
	}
}

func TestGNMI_CallToolStream_STREAM_MultipleAndCancel(t *testing.T) {
	srv, prov, _ := startUnifiedServer(t)
	defer srv.Stop()

	tr := NewGRPCClientTransport(nil)
	ctx := context.Background()

	stream, err := tr.CallToolStream(ctx, "gnmi_subscribe", map[string]any{
		"path": "/interfaces/interface[name=eth0]/state/oper-status",
		"mode": "STREAM",
	}, prov)
	if err != nil {
		t.Fatalf("CallToolStream: %v", err)
	}

	// Expect multiple updates
	for i := 0; i < 3; i++ {
		if _, err := stream.Next(); err != nil {
			t.Fatalf("Next %d: %v", i, err)
		}
	}

	// Cancel and ensure stream ends
	_ = stream.Close()
	if _, err := stream.Next(); err == nil {
		t.Fatalf("expected error after Close() on stream")
	}
}

func TestGNMI_CallToolStream_POLL_Pumps(t *testing.T) {
	srv, prov, _ := startUnifiedServer(t)
	defer srv.Stop()

	tr := NewGRPCClientTransport(nil)
	ctx := context.Background()

	stream, err := tr.CallToolStream(ctx, "gnmi_subscribe", map[string]any{
		"path":          "/interfaces/interface[name=eth0]/state/oper-status",
		"mode":          "POLL",
		"poll_every_ms": 25,
	}, prov)
	if err != nil {
		t.Fatalf("CallToolStream: %v", err)
	}
	defer stream.Close()

	if _, err := stream.Next(); err != nil {
		t.Fatalf("first Next: %v", err)
	}
	if _, err := stream.Next(); err != nil {
		t.Fatalf("second Next: %v", err)
	}
}

func TestGNMI_CallToolStream_TargetPrefixPropagated(t *testing.T) {
	srv, prov, us := startUnifiedServer(t)
	defer srv.Stop()

	prov.Target = "edge-sw-01"
	tr := NewGRPCClientTransport(nil)
	ctx := context.Background()

	stream, err := tr.CallToolStream(ctx, "gnmi_subscribe", map[string]any{
		"path": "/interfaces/interface[name=eth0]/state/oper-status",
		"mode": "ONCE",
	}, prov)
	if err != nil {
		t.Fatalf("CallToolStream: %v", err)
	}
	defer stream.Close()
	_, _ = stream.Next() // drain one

	// wait briefly for server to record target
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if v := us.firstPrefixTarget.Load(); v != nil {
			if v.(string) != "edge-sw-01" {
				t.Fatalf("Prefix.Target mismatch: got %q, want %q", v.(string), "edge-sw-01")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server did not observe Prefix.Target in time")
}
