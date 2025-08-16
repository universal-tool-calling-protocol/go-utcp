package grpcpb

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

type dummyServer struct {
	UnimplementedUTCPServiceServer
}

func (d *dummyServer) GetManual(context.Context, *Empty) (*Manual, error) {
	return &Manual{}, nil
}
func (d *dummyServer) CallTool(context.Context, *ToolCallRequest) (*ToolCallResponse, error) {
	return &ToolCallResponse{}, nil
}

func TestCoverage(t *testing.T) {
	e := &Empty{}
	e.Reset()
	e.ProtoMessage()
	_ = e.String()
	_ = e.ProtoReflect()
	_, _ = e.Descriptor()

	tl := &Tool{Name: "n", Description: "d"}
	tl.Reset()
	tl.ProtoMessage()
	_ = tl.String()
	_ = tl.ProtoReflect()
	_, _ = tl.Descriptor()
	_ = tl.GetName()
	_ = tl.GetDescription()

	m := &Manual{Version: "1", Tools: []*Tool{tl}}
	m.Reset()
	m.ProtoMessage()
	_ = m.String()
	_ = m.ProtoReflect()
	_, _ = m.Descriptor()
	_ = m.GetVersion()
	_ = m.GetTools()

	req := &ToolCallRequest{Tool: "ping", ArgsJson: "{}"}
	req.Reset()
	req.ProtoMessage()
	_ = req.String()
	_ = req.ProtoReflect()
	_, _ = req.Descriptor()
	_ = req.GetTool()
	_ = req.GetArgsJson()

	resp := &ToolCallResponse{ResultJson: "{}"}
	resp.Reset()
	resp.ProtoMessage()
	_ = resp.String()
	_ = resp.ProtoReflect()
	_, _ = resp.Descriptor()
	_ = resp.GetResultJson()

	srv := grpc.NewServer()
	RegisterUTCPServiceServer(srv, &dummyServer{})
	conn := fakeConn{}
	c := NewUTCPServiceClient(conn)
	c.GetManual(context.Background(), &Empty{})
	c.CallTool(context.Background(), &ToolCallRequest{})
	_, _ = _UTCPService_GetManual_Handler(&dummyServer{}, context.Background(), func(v interface{}) error { return nil }, nil)
	_, _ = _UTCPService_CallTool_Handler(&dummyServer{}, context.Background(), func(v interface{}) error { return nil }, nil)
}

type fakeConn struct{}

func (fakeConn) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	return nil
}
func (fakeConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, nil
}
