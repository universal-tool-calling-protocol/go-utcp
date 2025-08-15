package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/universal-tool-calling-protocol/go-utcp/src/grpcpb"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// GRPCClientTransport implements ClientTransport over gRPC using the UTCPService.
// It expects the remote server to implement the grpcpb.UTCPService service.
type GRPCClientTransport struct {
	logger func(format string, args ...interface{})
}

// NewGRPCClientTransport creates a new GRPCClientTransport with optional logger.
func NewGRPCClientTransport(logger func(format string, args ...interface{})) *GRPCClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &GRPCClientTransport{logger: logger}
}

func (t *GRPCClientTransport) dial(ctx context.Context, prov *GRPCProvider) (*grpc.ClientConn, error) {
	addr := fmt.Sprintf("%s:%d", prov.Host, prov.Port)
	var opts []grpc.DialOption
	if prov.UseSSL {
		// In this example we just use insecure when UseSSL is false.
		// Real implementation would configure TLS credentials.
		return nil, errors.New("SSL not implemented")
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	return grpc.DialContext(ctx, addr, opts...)
}

// RegisterToolProvider fetches the manual from the remote UTCPService.
func (t *GRPCClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	gp, ok := prov.(*GRPCProvider)
	if !ok {
		return nil, errors.New("GRPCClientTransport can only be used with GRPCProvider")
	}
	conn, err := t.dial(ctx, gp)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Handle gNMI providers by issuing a Capabilities request and returning
	// a built-in Subscribe tool so the UTCP client can resolve
	// "gnmi.gnmi_subscribe" during CallToolStream.
	if gp.ServiceName == "gnmi.gNMI" {
		client := gnmi.NewGNMIClient(conn)
		if _, err := client.Capabilities(ctx, &gnmi.CapabilityRequest{}); err != nil {
			return nil, err
		}
		return []Tool{{Name: "gnmi_subscribe", Description: "gNMI Subscribe stream"}}, nil
	}

	client := grpcpb.NewUTCPServiceClient(conn)
	resp, err := client.GetManual(ctx, &grpcpb.Empty{})
	if err != nil {
		return nil, err
	}
	tools := make([]Tool, len(resp.Tools))
	for i, tl := range resp.Tools {
		tools[i] = Tool{Name: tl.Name, Description: tl.Description}
	}
	return tools, nil
}

// DeregisterToolProvider is a no-op for gRPC transport.
func (t *GRPCClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	if _, ok := prov.(*GRPCProvider); !ok {
		return errors.New("GRPCClientTransport can only be used with GRPCProvider")
	}
	return nil
}

// CallTool invokes the CallTool RPC on the UTCPService.
func (t *GRPCClientTransport) CallTool(ctx context.Context, toolName string, args map[string]any, prov Provider, l *string) (any, error) {
	gp, ok := prov.(*GRPCProvider)
	if !ok {
		return nil, errors.New("GRPCClientTransport can only be used with GRPCProvider")
	}
	conn, err := t.dial(ctx, gp)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	client := grpcpb.NewUTCPServiceClient(conn)
	payload, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	resp, err := client.CallTool(ctx, &grpcpb.ToolCallRequest{Tool: toolName, ArgsJson: string(payload)})
	if err != nil {
		return nil, err
	}
	var result any
	if resp.ResultJson != "" {
		_ = json.Unmarshal([]byte(resp.ResultJson), &result)
	}
	return result, nil
}

// Close cleans up (no-op).
func (t *GRPCClientTransport) Close() error { return nil }

// in: src/transports/grpc/grpc_transport.go
// func (t *GRPCClientTransport) CallToolStream(...) (transports.StreamResult, error) { ... }

func (t *GRPCClientTransport) CallToolStream(
	ctx context.Context,
	toolName string,
	args map[string]any,
	p Provider,
) (transports.StreamResult, error) {
	gp, ok := p.(*GRPCProvider)
	if !ok {
		return nil, errors.New("GRPCClientTransport can only be used with GRPCProvider")
	}
	if gp.ServiceName != "gnmi.gNMI" || gp.MethodName != "Subscribe" {
		return nil, errors.New("streaming not supported by GRPCClientTransport")
	}

	ctx, cancel := context.WithCancel(ctx)
	conn, err := t.dial(ctx, gp)
	if err != nil {
		cancel()
		return nil, err
	}

	client := gnmi.NewGNMIClient(conn)
	stream, err := client.Subscribe(ctx)
	if err != nil {
		cancel()
		conn.Close()
		return nil, err
	}

	// --- Build SubscribeRequest from args ---
	pathStr, _ := args["path"].(string)
	modeStr, _ := args["mode"].(string)

	subMode := gnmi.SubscriptionList_STREAM
	switch strings.ToUpper(modeStr) {
	case "ONCE":
		subMode = gnmi.SubscriptionList_ONCE
	case "POLL":
		subMode = gnmi.SubscriptionList_POLL
	}

	path := parseGNMIPath(pathStr)
	subReq := &gnmi.SubscribeRequest{
		Request: &gnmi.SubscribeRequest_Subscribe{
			Subscribe: &gnmi.SubscriptionList{
				Mode:         subMode,
				Subscription: []*gnmi.Subscription{{Path: path}},
			},
		},
	}
	if gp.Target != "" {
		subReq.GetSubscribe().Prefix = &gnmi.Path{Target: gp.Target}
	}

	// Send initial Subscribe request
	if err := stream.Send(subReq); err != nil {
		cancel()
		conn.Close()
		return nil, err
	}

	// Channel of decoded updates/errors for the UTCP client
	ch := make(chan any, 16)

	// --- NEW: optional client->server POLL pump for true duplex ---
	var pollStop chan struct{}
	if subMode == gnmi.SubscriptionList_POLL {
		pollEveryMs := int64(0)
		switch v := args["poll_every_ms"].(type) {
		case int:
			pollEveryMs = int64(v)
		case int64:
			pollEveryMs = v
		case float64:
			pollEveryMs = int64(v)
		}
		if pollEveryMs > 0 {
			pollStop = make(chan struct{})
			go func() {
				ticker := time.NewTicker(time.Duration(pollEveryMs) * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-pollStop:
						return
					case <-ticker.C:
						// Actively send a Poll request (client -> server)
						if err := stream.Send(&gnmi.SubscribeRequest{
							Request: &gnmi.SubscribeRequest_Poll{Poll: &gnmi.Poll{}},
						}); err != nil {
							// Surface send errors to consumer and stop polling
							ch <- err
							return
						}
					}
				}
			}()
		}
	}

	// --- Receive loop (server -> client) ---
	go func() {
		defer func() {
			if pollStop != nil {
				close(pollStop)
			}
			close(ch)
			cancel()
			conn.Close()
		}()
		for {
			resp, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					ch <- err
				}
				return
			}
			// Convert protobuf to generic JSON object
			b, err := protojson.Marshal(resp)
			if err != nil {
				ch <- err
				return
			}
			var obj any
			if err := json.Unmarshal(b, &obj); err != nil {
				ch <- err
				return
			}
			ch <- obj
		}
	}()

	return transports.NewChannelStreamResult(ch, func() error {
		cancel()
		// conn is closed by receive goroutine
		return nil
	}), nil
}

func parseGNMIPath(p string) *gnmi.Path {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return &gnmi.Path{}
	}
	elems := strings.Split(p, "/")
	return &gnmi.Path{Element: elems}
}
