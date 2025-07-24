package webrtc

import (
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// WebRTCProvider represents a WebRTC data channel.
type WebRTCProvider struct {
	BaseProvider
	SignalingServer string `json:"signaling_server"`
	PeerID          string `json:"peer_id"`
	DataChannelName string `json:"data_channel_name"`
	// auth always nil
}
