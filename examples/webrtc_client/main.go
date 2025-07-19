package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	webrtc "github.com/pion/webrtc/v3"
	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

var (
	peers   = map[string]*webrtc.PeerConnection{}
	peersMu sync.Mutex
)

func startServer(addr, dcName string) {
	http.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ PeerID, SDP string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		local := []webrtc.ICECandidateInit{}
		pc.OnICECandidate(func(c *webrtc.ICECandidate) {
			if c != nil {
				local = append(local, c.ToJSON())
			}
		})
		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			if dc.Label() != dcName {
				return
			}
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				var env map[string]any
				if err := json.Unmarshal(msg.Data, &env); err == nil {
					id, _ := env["id"].(string)
					tool, _ := env["tool"].(string)
					args, _ := env["args"].(map[string]any)
					if tool == "echo" {
						resp := map[string]any{"id": id, "result": args["msg"]}
						b, _ := json.Marshal(resp)
						dc.SendText(string(b))
					}
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
		peersMu.Lock()
		peers[req.PeerID] = pc
		peersMu.Unlock()
		tools := []utcp.Tool{{Name: "echo", Description: "Echo tool"}}
		resp := struct {
			SDP        string                    `json:"sdp"`
			Tools      []utcp.Tool               `json:"tools"`
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
	go startServer(":8080", dcName)
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
	log.Printf("tools: %v", tools)

	res, err := client.CallTool(ctx, "webrtc.echo", map[string]any{"msg": "Hello"})
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	log.Printf("Result: %#v", res)
}
