package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net"
	"time"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/tcp"

	transports "github.com/universal-tool-calling-protocol/go-utcp/src/transports/tcp"
)

type toolRequest struct {
	Action string                 `json:"action,omitempty"`
	Tool   string                 `json:"tool,omitempty"`
	Args   map[string]interface{} `json:"args,omitempty"`
}

type tcpServer struct {
	ln net.Listener
}

func newServer(addr string) (*tcpServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &tcpServer{ln: ln}
	go s.accept()
	return s, nil
}

func (s *tcpServer) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *tcpServer) handle(c net.Conn) {
	defer c.Close()
	dec := json.NewDecoder(bufio.NewReader(c))
	var req toolRequest
	if err := dec.Decode(&req); err != nil {
		return
	}
	if req.Action == "list" {
		manual := map[string]interface{}{
			"version": "1.0",
			"tools": []map[string]interface{}{
				{
					"name":        "ping",
					"description": "Ping tool that responds with pong",
				},
			},
		}
		json.NewEncoder(c).Encode(manual)
		return
	}

	if req.Tool == "ping" {
		json.NewEncoder(c).Encode(map[string]any{"pong": true})
	}
}

func main() {
	srv, err := newServer("127.0.0.1:9090")
	if err != nil {
		log.Fatalf("server error: %v", err)
	}
	defer srv.ln.Close()

	time.Sleep(200 * time.Millisecond)

	logger := func(format string, args ...interface{}) { log.Printf(format, args...) }
	transport := transports.NewTCPClientTransport(logger)
	prov := &providers.TCPProvider{BaseProvider: BaseProvider{Name: "tcp", ProviderType: ProviderTCP}, Host: "127.0.0.1", Port: 9090, Timeout: 1000}

	ctx := context.Background()
	tools, err := transport.RegisterToolProvider(ctx, prov)
	if err != nil {
		log.Fatalf("register error: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	res, err := transport.CallTool(ctx, "ping", map[string]any{}, prov, false)
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	log.Printf("Result: %#v", res)

	_ = transport.DeregisterToolProvider(ctx, prov)
}
