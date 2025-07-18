package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	webrtc "github.com/pion/webrtc/v3"
	utcp "github.com/universal-tool-calling-protocol/UTCP"
)

// signaling server and WebRTC peer
func startServer(addr string, dcName string) {
	http.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			PeerID string `json:"peer_id"`
			SDP    string `json:"sdp"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			if dc.Label() != dcName {
				return
			}
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				var call struct {
					Tool string         `json:"tool"`
					Args map[string]any `json:"args"`
				}
				if err := json.Unmarshal(msg.Data, &call); err != nil {
					return
				}
				if call.Tool == "echo" {
					out, _ := json.Marshal(map[string]any{"result": call.Args["msg"]})
					dc.SendText(string(out))
				}
			})
		})

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

		manual := utcp.UtcpManual{Version: "1.0", Tools: []utcp.Tool{{Name: "echo", Description: "Echo"}}}
		resp := map[string]any{"sdp": pc.LocalDescription().SDP, "manual": manual}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	log.Printf("WebRTC signaling server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func main() {
	const dcName = "data"
	go startServer(":8080", dcName)
	time.Sleep(200 * time.Millisecond)

	logger := func(format string, args ...interface{}) { log.Printf(format, args...) }
	transport := utcp.NewWebRTCClientTransport(logger)
	prov := &utcp.WebRTCProvider{SignalingServer: "http://localhost:8080", PeerID: "client", DataChannelName: dcName}

	ctx := context.Background()
	tools, err := transport.RegisterToolProvider(ctx, prov)
	if err != nil {
		log.Fatalf("register: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	res, err := transport.CallTool(ctx, "echo", map[string]any{"msg": "hello"}, prov, nil)
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	log.Printf("Result: %#v", res)

	_ = transport.DeregisterToolProvider(ctx, prov)
}
