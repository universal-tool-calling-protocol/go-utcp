package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/universal-tool-calling-protocol/go-utcp/src"
	"github.com/universal-tool-calling-protocol/go-utcp/src/providers"
	utcp "github.com/universal-tool-calling-protocol/go-utcp/src/transports/udp"
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
			manual := src.UtcpManual{Version: "1.0", Tools: []src.Tool{{Name: "udp_echo", Description: "Echo"}}}
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
	server, err := startServer("127.0.0.1:0")
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	defer server.conn.Close()
	_, portStr, _ := net.SplitHostPort(server.conn.LocalAddr().String())
	port, _ := strconv.Atoi(portStr)

	time.Sleep(200 * time.Millisecond)

	logger := func(format string, args ...interface{}) { log.Printf(format, args...) }
	transport := utcp.NewUDPTransport(logger)
	prov := &providers.UDPProvider{BaseProvider: providers.BaseProvider{Name: "udp", ProviderType: providers.ProviderUDP}, Host: "127.0.0.1", Port: port, Timeout: 1000}

	ctx := context.Background()
	tools, err := transport.RegisterToolProvider(ctx, prov)
	if err != nil {
		log.Fatalf("register: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	res, err := transport.CallTool(ctx, "udp_echo", map[string]any{"msg": "hi"}, prov, nil)
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	log.Printf("Result: %#v", res)

	_ = transport.DeregisterToolProvider(ctx, prov)
}
