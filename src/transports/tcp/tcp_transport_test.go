package tcp

import (
	"context"
	"encoding/json"
	"net"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/tcp"

	"testing"
)

type tcpTestServer struct {
	ln net.Listener
}

func newTCPTestServer(t *testing.T) (*tcpTestServer, int) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen err: %v", err)
	}
	srv := &tcpTestServer{ln: ln}
	go srv.accept()
	port := ln.Addr().(*net.TCPAddr).Port
	return srv, port
}

func (s *tcpTestServer) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *tcpTestServer) handle(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	var req map[string]interface{}
	if err := dec.Decode(&req); err != nil {
		return
	}
	if req["action"] == "list" {
		resp := map[string]interface{}{
			"version": "1.0",
			"tools":   []map[string]interface{}{{"name": "ping", "description": "Ping"}},
		}
		json.NewEncoder(conn).Encode(resp)
		return
	}
	if name, ok := req["tool"].(string); ok && name == "ping" {
		json.NewEncoder(conn).Encode(map[string]any{"pong": true})
	}
}

func (s *tcpTestServer) close() {
	s.ln.Close()
}

func TestTCPClientTransport_RegisterAndCall(t *testing.T) {
	srv, port := newTCPTestServer(t)
	defer srv.close()

	prov := &TCPProvider{BaseProvider: BaseProvider{Name: "tcp", ProviderType: ProviderTCP}, Host: "127.0.0.1", Port: port}
	tr := NewTCPClientTransport(nil)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "ping" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	res, err := tr.CallTool(ctx, "ping", map[string]any{}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	m, ok := res.(map[string]interface{})
	if !ok || m["pong"] != true {
		t.Fatalf("unexpected result: %#v", res)
	}
}
