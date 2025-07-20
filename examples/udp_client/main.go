package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

type udpServer struct {
	conn *net.UDPConn
}

func startServer(addr string) (*udpServer, error) {
	// Load tools.json into a UtcpManual
	data, err := os.ReadFile("tools.json")
	if err != nil {
		return nil, fmt.Errorf("reading tools.json: %w", err)
	}

	var manual utcp.UtcpManual
	if err := json.Unmarshal(data, &manual); err != nil {
		return nil, fmt.Errorf("parsing tools.json: %w", err)
	}

	a, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	c, err := net.ListenUDP("udp", a)
	if err != nil {
		return nil, err
	}
	s := &udpServer{conn: c}
	go s.loop(manual)
	return s, nil
}

func (s *udpServer) loop(manual utcp.UtcpManual) {
	buf := make([]byte, 65535)
	for {
		n, remote, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		data := buf[:n]

		// Discovery request
		if string(data) == "DISCOVER" {
			out, _ := json.Marshal(manual)
			s.conn.WriteToUDP(out, remote)
			continue
		}

		var req map[string]any
		if err := json.Unmarshal(data, &req); err == nil {
			if raw, ok := req["tool"].(string); ok {
				parts := strings.Split(raw, ".")
				base := parts[len(parts)-1]
				if base == "udp_echo" {
					args := req["args"].(map[string]any)
					// Echo back the message
					resp, _ := json.Marshal(map[string]any{"result": args["msg"]})
					s.conn.WriteToUDP(resp, remote)
				}
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

	// Discover available tools via SearchTools
	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	// Call the udp_echo tool
	res, err := client.CallTool(ctx, "udp.udp_echo", map[string]any{"msg": "hi"})
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	log.Printf("Result: %#v", res)
}
