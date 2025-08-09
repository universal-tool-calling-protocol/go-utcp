package webrtc

import (
	"bytes"
	"context"
	"errors"
	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	webrtc "github.com/pion/webrtc/v3"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/webrtc"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

var newPeerConnection = webrtc.NewPeerConnection

// WebRTCClientTransport implements ClientTransport using WebRTC data channels.
type WebRTCClientTransport struct {
	pc      *webrtc.PeerConnection
	dc      *webrtc.DataChannel
	log     func(format string, args ...interface{})
	mu      sync.Mutex
	pending map[string]chan any
}

// NewWebRTCClientTransport creates a new transport instance.
func NewWebRTCClientTransport(logger func(format string, args ...interface{})) *WebRTCClientTransport {
	if logger == nil {
		logger = func(string, ...interface{}) {}
	}
	return &WebRTCClientTransport{log: logger, pending: make(map[string]chan any)}
}

func (t *WebRTCClientTransport) openConnection(ctx context.Context, prov *WebRTCProvider) ([]Tool, error) {
	if t.pc != nil {
		return nil, nil
	}
	config := webrtc.Configuration{}
	pc, err := newPeerConnection(config)
	if err != nil {
		return nil, err
	}

	// Send local ICE candidates to signaling server
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		cand := c.ToJSON()
		body, _ := json.Marshal(map[string]any{"peer_id": prov.PeerID, "candidate": cand})
		req, _ := http.NewRequest("POST", prov.SignalingServer+"/candidate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		go func() {
			client := &http.Client{Timeout: 10 * time.Second}
			if _, err := client.Do(req); err != nil {
				t.log("failed to send ICE candidate: %v", err)
			}
		}()
	})

	// Create data channel
	dc, err := pc.CreateDataChannel(prov.DataChannelName, nil)
	if err != nil {
		return nil, err
	}

	// Gather ICE and create offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return nil, err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return nil, err
	}
	// Wait for ICE gathering to complete
	<-webrtc.GatheringCompletePromise(pc)

	// Send SDP offer to signaling server
	body, _ := json.Marshal(map[string]string{"peer_id": prov.PeerID, "sdp": offer.SDP})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, prov.SignalingServer+"/connect", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Decode answer + any initial remote ICE candidates
	var ans struct {
		SDP        string                    `json:"sdp"`
		Tools      []Tool                    `json:"tools"`
		Candidates []webrtc.ICECandidateInit `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ans); err != nil {
		return nil, err
	}
	// Set remote SDP
	answer := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: ans.SDP}
	if err := pc.SetRemoteDescription(answer); err != nil {
		return nil, err
	}
	// Add any remote ICE candidates
	for _, ci := range ans.Candidates {
		if err := pc.AddICECandidate(ci); err != nil {
			t.log("failed to add ICE candidate: %v", err)
		}
	}

	// Wait for data channel open
	openCh := make(chan struct{})
	dc.OnOpen(func() { close(openCh) })
	select {
	case <-openCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, errors.New("timeout waiting for data channel to open")
	}

	// Store connection
	t.pc = pc
	t.dc = dc

	// Setup single OnMessage handler to dispatch by request ID
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var envelope map[string]any
		if err := json.Unmarshal(msg.Data, &envelope); err != nil {
			t.log("unmarshal msg: %v", err)
			return
		}
		id, _ := envelope["id"].(string)
		payload := envelope["result"]

		t.mu.Lock()
		ch, ok := t.pending[id]
		if ok {
			ch <- payload
			delete(t.pending, id)
		}
		t.mu.Unlock()
	})

	return ans.Tools, nil
}

func (t *WebRTCClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	rtcProv, ok := prov.(*WebRTCProvider)
	if !ok {
		return nil, errors.New("WebRTCClientTransport can only be used with WebRTCProvider")
	}
	return t.openConnection(ctx, rtcProv)
}

func (t *WebRTCClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	if t.dc != nil {
		_ = t.dc.Close()
	}
	if t.pc != nil {
		_ = t.pc.Close()
	}
	t.pc = nil
	t.dc = nil
	return nil
}

func (t *WebRTCClientTransport) CallTool(ctx context.Context, toolName string, args map[string]any, prov Provider, l *string) (any, error) {
	if t.dc == nil {
		return nil, errors.New("data channel not established")
	}
	// Create unique request ID
	id := uuid.NewString()
	env := map[string]any{"id": id, "tool": toolName, "args": args}
	payload, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}

	// Register response channel
	respCh := make(chan any, 1)
	t.mu.Lock()
	t.pending[id] = respCh
	t.mu.Unlock()

	// Send request
	if err := t.dc.SendText(string(payload)); err != nil {
		return nil, err
	}

	// Wait for response or context timeout
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-respCh:
		// Optionally update last-chunk pointer (for streaming)
		if str, ok := res.(string); ok && l != nil {
			*l = str
		}
		return res, nil
	}
}

func (t *WebRTCClientTransport) CallToolStream(
	ctx context.Context,
	toolName string,
	args map[string]any,
	p Provider,
) (transports.StreamResult, error) {
	return nil, errors.New("streaming not supported by WebRTCClientTransport")
}
