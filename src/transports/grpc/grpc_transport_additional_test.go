package grpc

import (
	"testing"

	gnmi "github.com/openconfig/gnmi/proto/gnmi"
	provgrpc "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
)

func TestParseGNMIPath(t *testing.T) {
	p := parseGNMIPath("/interfaces/interface[name=Ethernet2][subif=0]/state/oper-status")
	if len(p.GetElem()) != 4 {
		t.Fatalf("expected 4 elems, got %d", len(p.GetElem()))
	}
	if p.GetElem()[1].GetName() != "interface" {
		t.Fatalf("unexpected second element name: %s", p.GetElem()[1].GetName())
	}
	if p.GetElem()[1].GetKey()["name"] != "Ethernet2" || p.GetElem()[1].GetKey()["subif"] != "0" {
		t.Fatalf("unexpected keys: %v", p.GetElem()[1].GetKey())
	}
}

func TestBuildSubscribeRequest_SubMode(t *testing.T) {
	tpt := NewGRPCClientTransport(nil)
	gp := &provgrpc.GRPCProvider{}
	args := map[string]any{
		"path":     "/interfaces/interface[name=eth0]/state/oper-status",
		"mode":     "STREAM",
		"sub_mode": "ON_CHANGE",
	}
	req, err := tpt.buildSubscribeRequest(args, gp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sub := req.GetSubscribe().GetSubscription()
	if len(sub) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(sub))
	}
	if sub[0].Mode != gnmi.SubscriptionMode_ON_CHANGE {
		t.Fatalf("expected sub mode ON_CHANGE, got %v", sub[0].Mode)
	}
	if req.GetSubscribe().Mode != gnmi.SubscriptionList_STREAM {
		t.Fatalf("expected list mode STREAM, got %v", req.GetSubscribe().Mode)
	}
}
