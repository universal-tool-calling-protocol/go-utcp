package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	. "github.com/universal-tool-calling-protocol/go-utcp/internal/concepts"
	"github.com/universal-tool-calling-protocol/go-utcp/internal/grpcpb"
	. "github.com/universal-tool-calling-protocol/go-utcp/internal/providers"
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
