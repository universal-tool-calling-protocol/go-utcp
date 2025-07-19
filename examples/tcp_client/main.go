package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

type toolRequest struct {
	Action string                 `json:"action,omitempty"`
	Tool   string                 `json:"tool,omitempty"`
	Args   map[string]interface{} `json:"args,omitempty"`
}

type tcpServer struct{ ln net.Listener }

func newServer(addr string) *tcpServer {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	s := &tcpServer{ln: ln}
	go s.accept()
	return s
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
		manual := utcp.UtcpManual{Version: "1.0", Tools: []utcp.Tool{{Name: "ping", Description: "Ping"}}}
		json.NewEncoder(c).Encode(manual)
		return
	}
	if req.Tool == "ping" {
		json.NewEncoder(c).Encode(map[string]any{"pong": true})
	}
}

func main() {
	srv := newServer("127.0.0.1:9090")
	defer srv.ln.Close()
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	tools, err := client.SearchTools(ctx, "", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	fmt.Printf("tools: %v\n", tools)

	res, err := client.CallTool(ctx, "tcp.ping", map[string]any{})
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	fmt.Printf("Result: %#v\n", res)
}
