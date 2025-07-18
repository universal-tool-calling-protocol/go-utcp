package utcp

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
)

type udpServer struct {
	conn   *net.UDPConn
	doneCh chan struct{}
}

func startUDPServer(handler func([]byte) []byte) (*udpServer, error) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	s := &udpServer{conn: conn, doneCh: make(chan struct{})}
	go func() {
		buf := make([]byte, 65535)
		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-s.doneCh:
					return
				default:
					continue
				}
			}
			resp := handler(buf[:n])
			if resp != nil {
				conn.WriteToUDP(resp, remote)
			}
		}
	}()
	return s, nil
}

func (s *udpServer) addr() string {
	return s.conn.LocalAddr().String()
}

func (s *udpServer) close() {
	close(s.doneCh)
	s.conn.Close()
}

func TestUDPTransport_RegisterAndCall(t *testing.T) {
	server, err := startUDPServer(func(b []byte) []byte {
		if string(b) == "DISCOVER" {
			return []byte(`{"version":"1.0","tools":[{"name":"udp_echo","description":"Echo"}]}`)
		}
		var req map[string]any
		_ = json.Unmarshal(b, &req)
		if req["tool"] == "udp_echo" {
			args := req["args"].(map[string]any)
			out, _ := json.Marshal(map[string]any{"result": args["msg"]})
			return out
		}
		return nil
	})
	if err != nil {
		t.Fatalf("server error: %v", err)
	}
	defer server.close()

	host, portStr, _ := net.SplitHostPort(server.addr())
	port, _ := strconv.Atoi(portStr)
	prov := &UDPProvider{BaseProvider: BaseProvider{Name: "udp", ProviderType: ProviderUDP}, Host: host, Port: port, Timeout: 1000}

	tr := NewUDPTransport(nil)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "udp_echo" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	res, err := tr.CallTool(ctx, "udp_echo", map[string]any{"msg": "hi"}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok || m["result"] != "hi" {
		t.Fatalf("unexpected result: %#v", res)
	}
}
