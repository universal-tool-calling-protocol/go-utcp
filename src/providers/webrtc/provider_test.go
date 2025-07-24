package webrtc

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestWebRTCProvider_Basic(t *testing.T) {
	p := &WebRTCProvider{
		BaseProvider:    BaseProvider{Name: "w", ProviderType: ProviderWebRTC},
		SignalingServer: "sig",
		PeerID:          "peer",
		DataChannelName: "chan",
	}
	if p.Type() != ProviderWebRTC {
		t.Fatalf("type mismatch")
	}
	if p.PeerID != "peer" {
		t.Fatalf("peer id mismatch")
	}
}
