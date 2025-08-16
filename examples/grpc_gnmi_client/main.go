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
	"github.com/universal-tool-calling-protocol/go-utcp/src/grpcpb"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"

	"google.golang.org/grpc"
)

type UnifiedServer struct {
	grpcpb.UnimplementedUTCPServiceServer
	gnmi.UnimplementedGNMIServer
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
						Path: &gnmi.Path{Element: []string{"interfaces", "interface", "eth0"}},
						Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_StringVal{StringVal: state}},
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
	srv := grpc.NewServer()
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
