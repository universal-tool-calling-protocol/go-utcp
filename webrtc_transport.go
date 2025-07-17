package UTCP

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	webrtc "github.com/pion/webrtc/v3"
)

// WebRTCClientTransport implements ClientTransport using WebRTC data channels.
type WebRTCClientTransport struct {
	pc  *webrtc.PeerConnection
	dc  *webrtc.DataChannel
	log func(format string, args ...interface{})
}

// NewWebRTCClientTransport creates a new transport instance.
func NewWebRTCClientTransport(logger func(format string, args ...interface{})) *WebRTCClientTransport {
	if logger == nil {
		logger = func(string, ...interface{}) {}
	}
	return &WebRTCClientTransport{log: logger}
}

func (t *WebRTCClientTransport) openConnection(ctx context.Context, prov *WebRTCProvider) ([]Tool, error) {
	if t.pc != nil {
		return nil, nil
	}
	config := webrtc.Configuration{}
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}
	dc, err := pc.CreateDataChannel(prov.DataChannelName, nil)
	if err != nil {
		return nil, err
	}
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return nil, err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return nil, err
	}
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
	var ans struct {
		SDP    string     `json:"sdp"`
		Manual UtcpManual `json:"manual"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ans); err != nil {
		return nil, err
	}
	answer := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: ans.SDP}
	if err := pc.SetRemoteDescription(answer); err != nil {
		return nil, err
	}
	t.pc = pc
	t.dc = dc
	return ans.Manual.Tools, nil
}

// RegisterToolProvider connects to the remote peer and returns its tools.
func (t *WebRTCClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	rtcProv, ok := prov.(*WebRTCProvider)
	if !ok {
		return nil, errors.New("WebRTCClientTransport can only be used with WebRTCProvider")
	}
	tools, err := t.openConnection(ctx, rtcProv)
	if err != nil {
		return nil, err
	}
	return tools, nil
}

// DeregisterToolProvider closes the WebRTC connection.
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

// CallTool sends a request over the WebRTC data channel and waits for a response.
func (t *WebRTCClientTransport) CallTool(ctx context.Context, toolName string, args map[string]any, prov Provider, l *string) (any, error) {
	if t.dc == nil {
		return nil, errors.New("data channel not established")
	}
	payload, err := json.Marshal(map[string]any{"tool": toolName, "args": args})
	if err != nil {
		return nil, err
	}
	resultCh := make(chan any, 1)
	errCh := make(chan error, 1)
	t.dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var res any
		if err := json.Unmarshal(msg.Data, &res); err != nil {
			errCh <- err
			return
		}
		resultCh <- res
	})
	if err := t.dc.SendText(string(payload)); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case res := <-resultCh:
		return res, nil
	}
}
