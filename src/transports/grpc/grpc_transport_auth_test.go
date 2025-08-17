package grpc

import (
	"context"
	"encoding/base64"
	"net"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	"github.com/universal-tool-calling-protocol/go-utcp/src/grpcpb"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type authServer struct {
	grpcpb.UnimplementedUTCPServiceServer
}

func (s *authServer) GetManual(ctx context.Context, _ *grpcpb.Empty) (*grpcpb.Manual, error) {
	return &grpcpb.Manual{}, nil
}

func TestGRPCClientTransport_BasicAuth(t *testing.T) {
	const user = "u"
	const pass = "p"
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))

	interceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		if md == nil {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		auths := md.Get("authorization")
		if len(auths) == 0 || auths[0] != expected {
			return nil, status.Error(codes.Unauthenticated, "bad creds")
		}
		return handler(ctx, req)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen err: %v", err)
	}
	srv := grpc.NewServer(grpc.UnaryInterceptor(interceptor))
	grpcpb.RegisterUTCPServiceServer(srv, &authServer{})
	go srv.Serve(lis)
	defer srv.Stop()

	port := lis.Addr().(*net.TCPAddr).Port
	ba := &BasicAuth{AuthType: BasicType, Username: user, Password: pass}
	var a Auth = ba
	prov := &GRPCProvider{BaseProvider: BaseProvider{Name: "g", ProviderType: ProviderGRPC}, Host: "127.0.0.1", Port: port, Auth: &a}

	tr := NewGRPCClientTransport(nil)
	if _, err := tr.RegisterToolProvider(context.Background(), prov); err != nil {
		t.Fatalf("RegisterToolProvider err: %v", err)
	}
}
