package webrtc

import (
	"context"
	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"
	"net/http"

	"net/http/httptest"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/webrtc"

	webrtc "github.com/pion/webrtc/v3"
)

type signalingServer struct {
	pc  *webrtc.PeerConnection
	srv *httptest.Server
}

func newSignalingServer(t *testing.T) *signalingServer {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	server := &signalingServer{pc: pc}
	mux := http.NewServeMux()
	mux.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: req["sdp"]}
		if err := pc.SetRemoteDescription(offer); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if err := pc.SetLocalDescription(answer); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		<-webrtc.GatheringCompletePromise(pc)
		resp := map[string]any{"sdp": pc.LocalDescription().SDP, "tools": []map[string]any{{"name": "echo"}}}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/candidate", func(w http.ResponseWriter, r *http.Request) {
		// ignore candidates
	})
	server.srv = httptest.NewServer(mux)
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			var env map[string]any
			json.Unmarshal(msg.Data, &env)
			id := env["id"].(string)
			args := env["args"].(map[string]any)
			out, _ := json.Marshal(map[string]any{"id": id, "result": map[string]any{"echo": args["msg"]}})
			dc.SendText(string(out))
		})
	})
	return server
}

func (s *signalingServer) close() { s.srv.Close(); s.pc.Close() }

func TestWebRTCTransport_RegisterAndCall(t *testing.T) {
	srv := newSignalingServer(t)
	defer srv.close()

	prov := &WebRTCProvider{BaseProvider: BaseProvider{Name: "w", ProviderType: ProviderWebRTC}, SignalingServer: srv.srv.URL, PeerID: "peer", DataChannelName: "data"}
	tr := NewWebRTCClientTransport(nil)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil || len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("register: %v tools:%v", err, tools)
	}
	res, err := tr.CallTool(ctx, "echo", map[string]any{"msg": "hi"}, prov, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok || m["echo"] != "hi" {
		t.Fatalf("bad result: %#v", res)
	}
	if err := tr.DeregisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("dereg: %v", err)
	}
}
