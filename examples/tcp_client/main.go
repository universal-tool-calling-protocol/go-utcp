package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/universal-tool-calling-protocol/go-utcp/src/providers"
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
		log.Printf("decode error: %v", err)
		return
	}

	log.Printf("Received request: %+v", req)

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
		if err := json.NewEncoder(c).Encode(manual); err != nil {
			log.Printf("encode manual error: %v", err)
		}
		return
	}

	if req.Tool == "ping" {
		result := map[string]any{"pong": true}
		if err := json.NewEncoder(c).Encode(result); err != nil {
			log.Printf("encode result error: %v", err)
		}
		return
	}

	// Unknown tool
	log.Printf("Unknown tool: %s", req.Tool)
}

func main() {
	srv, err := newServer("127.0.0.1:9090")
	if err != nil {
		log.Fatalf("server error: %v", err)
	}
	defer srv.ln.Close()

	time.Sleep(200 * time.Millisecond)

	logger := func(format string, args ...interface{}) { log.Printf("[CLIENT] "+format, args...) }
	transport := transports.NewTCPClientTransport(logger)
	prov := &providers.TCPProvider{
		BaseProvider: providers.BaseProvider{
			Name:         "tcp",
			ProviderType: providers.ProviderTCP,
		},
		Host:    "127.0.0.1",
		Port:    9090,
		Timeout: 1000,
	}

	ctx := context.Background()
	tools, err := transport.RegisterToolProvider(ctx, prov)
	if err != nil {
		log.Fatalf("register error: %v", err)
	}

	log.Printf("Discovered %d tools:", len(tools))
	for _, t := range tools {
		log.Printf(" - %s: %s", t.Name, t.Description)
	}

	if len(tools) == 0 {
		log.Fatal("No tools discovered!")
	}

	res, err := transport.CallTool(ctx, "ping", map[string]any{}, prov, nil)
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	log.Printf("Result: %#v", res)

	_ = transport.DeregisterToolProvider(ctx, prov)
}
