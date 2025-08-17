package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"path/filepath"
	"strings"
	"time"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/grpcpb"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type UnifiedServer struct {
	grpcpb.UnimplementedUTCPServiceServer
	gnmi.UnimplementedGNMIServer
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
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return fmt.Errorf("unauthorized")
	}
	parts := strings.SplitN(vals[0], " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
		return fmt.Errorf("unauthorized")
	}
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("unauthorized")
	}
	up := strings.SplitN(string(decoded), ":", 2)
	if len(up) != 2 || up[0] != user || up[1] != pass {
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

func (s *UnifiedServer) Subscribe(stream gnmi.GNMI_SubscribeServer) error {
	ctx := stream.Context()

	// Single-sender to keep gRPC Send safe.
	out := make(chan *gnmi.SubscribeResponse, 32)
	defer close(out)

	sendDone := make(chan error, 1)
	go func() {
		for msg := range out {
			if err := stream.Send(msg); err != nil {
				sendDone <- err
				return
			}
		}
		sendDone <- nil
	}()

	// Helper to push a synthetic update.
	sendInterface := func(state string) {
		out <- &gnmi.SubscribeResponse{
			Response: &gnmi.SubscribeResponse_Update{
				Update: &gnmi.Notification{
					Timestamp: time.Now().UnixNano(),
					Update: []*gnmi.Update{{
						Path: &gnmi.Path{Elem: []*gnmi.PathElem{
							{Name: "interfaces"},
							{Name: "interface", Key: map[string]string{"name": "eth0"}},
						}},
						Val: &gnmi.TypedValue{Value: &gnmi.TypedValue_StringVal{StringVal: state}},
					}},
				},
			},
		}
	}

	mode := gnmi.SubscriptionList_STREAM // now actually used below

	var ticker *time.Ticker
	stopTicker := func() {
		if ticker != nil {
			ticker.Stop()
			ticker = nil
		}
	}

	for {
		select {
		case <-ctx.Done():
			stopTicker()
			return ctx.Err()
		case err := <-sendDone:
			stopTicker()
			return err
		default:
		}

		req, err := stream.Recv()
		if err != nil {
			stopTicker()
			return err
		}

		switch r := req.Request.(type) {
		case *gnmi.SubscribeRequest_Subscribe:
			// Set mode from the request (prevents "unused" and implements behavior).
			if r.Subscribe != nil {
				mode = r.Subscribe.Mode
			}

			// STREAM: periodic pushes; POLL: no ticker.
			if mode == gnmi.SubscriptionList_STREAM {
				if ticker == nil {
					ticker = time.NewTicker(500 * time.Millisecond)
					go func() {
						for {
							select {
							case <-ctx.Done():
								return
							case <-ticker.C:
								sendInterface("UP")
							}
						}
					}()
				}
			} else {
				// ONCE or POLL
				stopTicker()
			}

			// Acknowledge (re)subscription with an immediate update.
			sendInterface("UP")

		case *gnmi.SubscribeRequest_Poll:
			// POLL: send one update per poll.
			if mode == gnmi.SubscriptionList_POLL {
				sendInterface("UP")
			}

		default:
			// Ignore other request kinds for this demo.
		}
	}
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

	ctx := context.Background()
	repo := repository.NewInMemoryToolRepository()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: filepath.Join("examples", "grpc_gnmi_client", "provider.json")}
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
		"path":               "/interfaces/interface[name=eth0]",
		"mode":               "STREAM",
		"sub_mode":           "SAMPLE",
		"sample_interval_ns": 500000000,
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
