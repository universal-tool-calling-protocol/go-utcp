package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

type udpServer struct {
	conn *net.UDPConn
}

func startServer(addr string) (*udpServer, error) {
	a, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	c, err := net.ListenUDP("udp", a)
	if err != nil {
		return nil, err
	}
	s := &udpServer{conn: c}
	go s.loop()
	return s, nil
}

func (s *udpServer) loop() {
	buf := make([]byte, 65535)
	for {
		n, remote, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		data := buf[:n]
		if string(data) == "DISCOVER" {
			manual := utcp.UtcpManual{Version: "1.0", Tools: []utcp.Tool{{Name: "udp_echo", Description: "Echo"}}}
			out, _ := json.Marshal(manual)
			s.conn.WriteToUDP(out, remote)
			continue
		}
		var req map[string]any
		if err := json.Unmarshal(data, &req); err == nil {
			if req["tool"] == "udp_echo" {
				args := req["args"].(map[string]any)
				resp, _ := json.Marshal(map[string]any{"result": args["msg"]})
				s.conn.WriteToUDP(resp, remote)
			}
		}
	}
}

func main() {
	server, err := startServer("127.0.0.1:9091")
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	defer server.conn.Close()

	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	res, err := client.CallTool(ctx, "udp.udp_echo", map[string]any{"msg": "hi"})
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	log.Printf("Result: %#v", res)
}
