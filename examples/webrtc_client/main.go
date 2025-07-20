package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"

	webrtc "github.com/pion/webrtc/v3"
)

var (
	peers   = map[string]*webrtc.PeerConnection{}
	peersMu sync.Mutex
)

func waitForConnection(ctx context.Context, client *utcp.UtcpClient, maxWait time.Duration) error {
	start := time.Now()
	for time.Since(start) < maxWait {
		// Try to ping or check connection status
		// This is provider-specific, but we can try a simple tool call
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Small delay between checks
			time.Sleep(100 * time.Millisecond)
		}

		// Check if we can discover tools (indicates connection is ready)
		tools, err := client.SearchTools("", 1)
		if err == nil && len(tools) > 0 {
			log.Printf("Connection appears ready, found %d tools", len(tools))
			return nil
		}

		if time.Since(start) > maxWait {
			break
		}
	}
	return nil // Continue anyway after timeout
}

func main() {
	const dcName = "data"

	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create HTTP server
	server := &http.Server{Addr: ":8080"}

	// Set up the handler
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

		local := []webrtc.ICECandidateInit{}
		pc.OnICECandidate(func(c *webrtc.ICECandidate) {
			if c != nil {
				local = append(local, c.ToJSON())
			}
		})

		// Add connection state change handler
		pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
			log.Printf("Peer connection state changed: %s", state.String())
		})

		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			log.Printf("Data channel received: %s", dc.Label())
			if dc.Label() != dcName {
				return
			}

			dc.OnOpen(func() {
				log.Printf("Data channel opened: %s", dc.Label())
			})

			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				log.Printf("Received message: %s", string(msg.Data))
				var env map[string]any
				if err := json.Unmarshal(msg.Data, &env); err != nil {
					log.Printf("Failed to unmarshal message: %v", err)
					return
				}
				id, _ := env["id"].(string)
				tool, _ := env["tool"].(string)
				args, _ := env["args"].(map[string]any)

				log.Printf("Processing tool call: %s with args: %+v", tool, args)

				if tool == "echo" || tool == "webrtc.echo" {
					resp := map[string]any{
						"id": id,
						"result": map[string]any{
							"content": []map[string]any{
								{
									"type": "text",
									"text": args["msg"],
								},
							},
						},
					}
					b, err := json.Marshal(resp)
					if err != nil {
						log.Printf("Failed to marshal response: %v", err)
						return
					}
					log.Printf("Sending response: %s", string(b))
					if err := dc.SendText(string(b)); err != nil {
						log.Printf("Failed to send response: %v", err)
					} else {
						log.Printf("Response sent successfully")
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

		// Wait for ICE gathering to complete
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

	// Start server in goroutine
	go func() {
		log.Printf("Signaling server on :8080")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	// Give server time to start
	time.Sleep(500 * time.Millisecond)

	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	// Wait a bit longer for WebRTC connection to establish
	log.Println("Waiting for WebRTC connection to establish...")
	if err := waitForConnection(ctx, client, 5*time.Second); err != nil {
		log.Printf("Warning: Connection wait failed: %v", err)
	}

	// Additional delay to ensure data channel is ready
	time.Sleep(1 * time.Second)

	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	log.Printf("Discovered tools: %+v", tools)

	if len(tools) == 0 {
		log.Fatal("No tools discovered")
	}

	log.Println("Attempting to call echo tool...")
	res, err := client.CallTool(ctx, "webrtc.echo", map[string]any{"msg": "Hello, WebRTC!"})
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	log.Printf("Echo result: %v", res)

	log.Println("Program completed successfully")

	// Shutdown server gracefully
	log.Println("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	} else {
		log.Println("Server shut down successfully")
	}

	// Close peer connections
	peersMu.Lock()
	for id, pc := range peers {
		log.Printf("Closing peer connection: %s", id)
		pc.Close()
	}
	peersMu.Unlock()
}
