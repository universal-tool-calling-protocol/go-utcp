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
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"

	auth "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	"github.com/universal-tool-calling-protocol/go-utcp/src/grpcpb"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

type basicPerRPCCreds struct {
	username   string
	password   string
	requireTLS bool
}

func (b basicPerRPCCreds) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		"username": b.username,
		"password": b.password,
	}, nil
}

func (b basicPerRPCCreds) RequireTransportSecurity() bool { return b.requireTLS }

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

// addTargetToContext adds the target as gRPC metadata if specified
func (t *GRPCClientTransport) addTargetToContext(ctx context.Context, prov *GRPCProvider) context.Context {
	if prov.Target != "" {
		// Add target as gRPC metadata - common pattern for gNMI and similar services
		md := metadata.Pairs("target", prov.Target)
		ctx = metadata.NewOutgoingContext(ctx, md)
		t.logger("Added target '%s' to gRPC metadata", prov.Target)
	}
	return ctx
}

func (t *GRPCClientTransport) dial(ctx context.Context, prov *GRPCProvider) (*grpc.ClientConn, error) {
	addr := fmt.Sprintf("%s:%d", prov.Host, prov.Port)
	var opts []grpc.DialOption

	// Add target as dial option if specified (some gRPC services use this)
	if prov.Target != "" {
		// Some services expect the target in the dial context
		opts = append(opts, grpc.WithAuthority(prov.Target))
		t.logger("Using target '%s' as gRPC authority", prov.Target)
	}

	if prov.UseSSL {
		// In this example we just use insecure when UseSSL is false.
		// Real implementation would configure TLS credentials.
		return nil, errors.New("SSL not implemented")
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		t.logger("Using insecure gRPC transport for %s, suitable only for non-production", addr)
	}

	if prov.Auth != nil {
		switch a := (*prov.Auth).(type) {
		case *auth.BasicAuth:
			opts = append(opts, grpc.WithPerRPCCredentials(basicPerRPCCreds{
				username:   a.Username,
				password:   a.Password,
				requireTLS: prov.UseSSL,
			}))
		}
	}

	return grpc.DialContext(ctx, addr, opts...)
}

// RegisterToolProvider fetches the manual from the remote UTCPService.
func (t *GRPCClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	gp, ok := prov.(*GRPCProvider)
	if !ok {
		return nil, errors.New("GRPCClientTransport can only be used with GRPCProvider")
	}

	// Add target to context if specified
	ctx = t.addTargetToContext(ctx, gp)

	conn, err := t.dial(ctx, gp)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

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

	// Add target to context if specified
	ctx = t.addTargetToContext(ctx, gp)

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

// CallToolStream implements streaming tool calls with two pathways:
// A) Direct gNMI Subscribe for gNMI providers
// B) UTCP server streaming via UTCPService.CallToolStream
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

	// Route to appropriate streaming implementation
	if gp.ServiceName == "gnmi.gNMI" && gp.MethodName == "Subscribe" {
		return t.callGNMISubscribe(ctx, args, gp)
	}

	return t.callUTCPToolStream(ctx, toolName, args, gp)
}

// callGNMISubscribe handles direct gNMI Subscribe streaming
func (t *GRPCClientTransport) callGNMISubscribe(
	ctx context.Context,
	args map[string]any,
	gp *GRPCProvider,
) (transports.StreamResult, error) {
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

	// Build and send initial subscribe request
	subReq, err := t.buildSubscribeRequest(args, gp)
	if err != nil {
		cancel()
		conn.Close()
		return nil, err
	}

	if err := stream.Send(subReq); err != nil {
		cancel()
		conn.Close()
		return nil, err
	}

	ch := make(chan any, 16)

	// Start polling if needed
	pollStop := t.startPollingIfNeeded(ctx, stream, args, subReq.GetSubscribe().Mode, ch)

	// Start receive loop
	t.startGNMIReceiveLoop(ctx, stream, ch, cancel, conn, pollStop)

	return transports.NewChannelStreamResult(ch, func() error {
		cancel()
		return nil
	}), nil
}

// buildSubscribeRequest constructs a gNMI SubscribeRequest from arguments
func (t *GRPCClientTransport) buildSubscribeRequest(args map[string]any, gp *GRPCProvider) (*gnmi.SubscribeRequest, error) {
	pathStr, _ := args["path"].(string)
	listModeStr, _ := args["mode"].(string) // ONCE | POLL | STREAM

	// List (outer) mode
	listMode := gnmi.SubscriptionList_STREAM
	switch strings.ToUpper(listModeStr) {
	case "ONCE":
		listMode = gnmi.SubscriptionList_ONCE
	case "POLL":
		listMode = gnmi.SubscriptionList_POLL
	}

	// Per-subscription mode
	subMode := gnmi.SubscriptionMode_SAMPLE
	if v, ok := args["sub_mode"].(string); ok {
		switch strings.ToUpper(v) {
		case "SAMPLE":
			subMode = gnmi.SubscriptionMode_SAMPLE
		case "ON_CHANGE":
			subMode = gnmi.SubscriptionMode_ON_CHANGE
		case "TARGET_DEFINED":
			subMode = gnmi.SubscriptionMode_TARGET_DEFINED
		}
	}

	// Optional intervals / flags
	toUint64 := func(x any) uint64 {
		switch n := x.(type) {
		case int:
			return uint64(n)
		case int64:
			return uint64(n)
		case float64:
			return uint64(n)
		case uint64:
			return n
		default:
			return 0
		}
	}
	sampleInterval := toUint64(args["sample_interval_ns"])
	heartbeatInterval := toUint64(args["heartbeat_interval_ns"])
	suppressRedundant, _ := args["suppress_redundant"].(bool)

	// Path and subscription
	path := parseGNMIPath(pathStr)
	sub := &gnmi.Subscription{
		Path:              path,
		Mode:              subMode,
		SampleInterval:    sampleInterval,
		HeartbeatInterval: heartbeatInterval,
		SuppressRedundant: suppressRedundant,
	}

	req := &gnmi.SubscribeRequest{
		Request: &gnmi.SubscribeRequest_Subscribe{
			Subscribe: &gnmi.SubscriptionList{
				Mode:         listMode,
				Subscription: []*gnmi.Subscription{sub},
			},
		},
	}

	if gp.Target != "" {
		req.GetSubscribe().Prefix = &gnmi.Path{Target: gp.Target}
	}
	return req, nil
}

// startPollingIfNeeded starts a polling goroutine for POLL mode subscriptions
func (t *GRPCClientTransport) startPollingIfNeeded(
	ctx context.Context,
	stream gnmi.GNMI_SubscribeClient,
	args map[string]any,
	mode gnmi.SubscriptionList_Mode,
	ch chan any,
) chan struct{} {
	if mode != gnmi.SubscriptionList_POLL {
		return nil
	}

	var pollEveryMs int64
	switch v := args["poll_every_ms"].(type) {
	case int:
		pollEveryMs = int64(v)
	case int64:
		pollEveryMs = v
	case float64:
		pollEveryMs = int64(v)
	}

	if pollEveryMs <= 0 {
		return nil
	}

	pollStop := make(chan struct{})
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
				if err := stream.Send(&gnmi.SubscribeRequest{
					Request: &gnmi.SubscribeRequest_Poll{Poll: &gnmi.Poll{}},
				}); err != nil {
					ch <- err
					return
				}
			}
		}
	}()

	return pollStop
}

// startGNMIReceiveLoop starts the goroutine that receives gNMI responses
func (t *GRPCClientTransport) startGNMIReceiveLoop(
	ctx context.Context,
	stream gnmi.GNMI_SubscribeClient,
	ch chan any,
	cancel context.CancelFunc,
	conn *grpc.ClientConn,
	pollStop chan struct{},
) {
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

			obj, err := t.convertGNMIResponseToJSON(resp)
			if err != nil {
				ch <- err
				return
			}

			ch <- obj
		}
	}()
}

// convertGNMIResponseToJSON converts a gNMI response to JSON object
func (t *GRPCClientTransport) convertGNMIResponseToJSON(resp *gnmi.SubscribeResponse) (any, error) {
	b, err := protojson.Marshal(resp)
	if err != nil {
		return nil, err
	}

	var obj any
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// callUTCPToolStream handles UTCP server streaming via UTCPService.CallToolStream
func (t *GRPCClientTransport) callUTCPToolStream(
	ctx context.Context,
	toolName string,
	args map[string]any,
	gp *GRPCProvider,
) (transports.StreamResult, error) {
	ctx, cancel := context.WithCancel(ctx)

	conn, err := t.dial(ctx, gp)
	if err != nil {
		cancel()
		return nil, err
	}

	client := grpcpb.NewUTCPServiceClient(conn)

	payload, err := json.Marshal(args)
	if err != nil {
		cancel()
		conn.Close()
		return nil, err
	}

	req := &grpcpb.ToolCallRequest{
		Tool:     toolName,
		ArgsJson: string(payload),
	}

	stream, err := client.CallToolStream(ctx, req)
	if err != nil {
		cancel()
		conn.Close()
		return nil, err
	}

	ch := make(chan any, 16)
	t.startUTCPReceiveLoop(ctx, stream, ch, cancel, conn)

	return transports.NewChannelStreamResult(ch, func() error {
		cancel()
		return nil
	}), nil
}

// startUTCPReceiveLoop starts the goroutine that receives UTCP streaming responses
func (t *GRPCClientTransport) startUTCPReceiveLoop(
	ctx context.Context,
	stream grpcpb.UTCPService_CallToolStreamClient,
	ch chan any,
	cancel context.CancelFunc,
	conn *grpc.ClientConn,
) {
	go func() {
		defer func() {
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

			// Each message is JSON in ResultJson – pass as []byte
			ch <- []byte(resp.GetResultJson())
		}
	}()
}

// parseGNMIPath parses a path string into a gNMI Path
func parseGNMIPath(p string) *gnmi.Path {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return &gnmi.Path{}
	}
	p = strings.TrimPrefix(p, "/")
	segs := strings.Split(p, "/")

	elems := make([]*gnmi.PathElem, 0, len(segs))
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		name := seg
		keys := map[string]string{}

		if i := strings.IndexRune(seg, '['); i >= 0 {
			name = seg[:i]
			rest := seg[i:]
			for len(rest) > 0 {
				if rest[0] != '[' {
					break
				}
				end := strings.IndexRune(rest, ']')
				if end <= 1 {
					break
				}
				kv := rest[1:end]
				rest = rest[end+1:]
				if eq := strings.IndexRune(kv, '='); eq > 0 && eq < len(kv)-1 {
					k := kv[:eq]
					v := kv[eq+1:]
					keys[k] = v
				}
			}
		}

		elems = append(elems, &gnmi.PathElem{Name: name, Key: keys})
	}

	return &gnmi.Path{Elem: elems}
}
