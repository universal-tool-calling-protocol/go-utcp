package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	src "github.com/universal-tool-calling-protocol/go-utcp/internal"
	"github.com/universal-tool-calling-protocol/go-utcp/internal/providers"
	utcp "github.com/universal-tool-calling-protocol/go-utcp/internal/transports/webrtc"

	webrtc "github.com/pion/webrtc/v3"
)

// ---- Signaling Server ----
var (
	peers   = map[string]*webrtc.PeerConnection{}
	peersMu sync.Mutex
)

func startServer(addr, dcName string) {
	http.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			PeerID string `json:"peer_id"`
			SDP    string `json:"sdp"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Create peer
		pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// collect ICE
		local := []webrtc.ICECandidateInit{}
		pc.OnICECandidate(func(c *webrtc.ICECandidate) {
			if c != nil {
				local = append(local, c.ToJSON())
			}
		})
		// handle data channel and echo tool
		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			if dc.Label() != dcName {
				return
			}
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				var env map[string]any
				if err := json.Unmarshal(msg.Data, &env); err != nil {
					return
				}
				id, _ := env["id"].(string)
				tool, _ := env["tool"].(string)
				args, _ := env["args"].(map[string]any)
				if tool == "echo" {
					resp := map[string]any{"id": id, "result": args["msg"]}
					b, _ := json.Marshal(resp)
					dc.SendText(string(b))
				}
			})
		})
		// complete handshake
		offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: req.SDP}
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
		// store
		peersMu.Lock()
		peers[req.PeerID] = pc
		peersMu.Unlock()
		// respond
		tools := []src.Tool{{Name: "echo", Description: "Echo tool"}}
		resp := struct {
			SDP        string                    `json:"sdp"`
			Tools      []src.Tool                `json:"tools"`
			Candidates []webrtc.ICECandidateInit `json:"candidates"`
		}{pc.LocalDescription().SDP, tools, local}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	log.Printf("Signaling server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func main() {
	const dcName = "data"
	// start signaling
	go startServer(":8080", dcName)
	// allow server to start
	time.Sleep(200 * time.Millisecond)

	// client setup
	ctx := context.Background()
	transport := utcp.NewWebRTCClientTransport(log.Printf)
	prov := &providers.WebRTCProvider{SignalingServer: "http://localhost:8080", PeerID: "client", DataChannelName: dcName}
	// register & discover
	tools, err := transport.RegisterToolProvider(ctx, prov)
	if err != nil {
		log.Fatalf("register error: %v", err)
	}
	log.Printf("Discovered tools: %+v", tools)

	// call echo
	res, err := transport.CallTool(ctx, "echo", map[string]any{"msg": "Hello, WebRTC!"}, prov, nil)
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	log.Printf("Echo result: %v", res)

	transport.DeregisterToolProvider(ctx, prov)
}
