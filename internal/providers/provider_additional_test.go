package providers

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/internal/auth"
)

func TestUnmarshalAuth_Types(t *testing.T) {
	apiJSON := []byte(`{"auth_type":"api_key","api_key":"secret","var_name":"X","location":"header"}`)
	a, err := unmarshalAuth(apiJSON)
	if err != nil {
		t.Fatalf("api key err: %v", err)
	}
	if _, ok := a.(*ApiKeyAuth); !ok {
		t.Fatalf("expected ApiKeyAuth got %T", a)
	}

	basicJSON := []byte(`{"auth_type":"basic","username":"u","password":"p"}`)
	b, err := unmarshalAuth(basicJSON)
	if err != nil {
		t.Fatalf("basic err: %v", err)
	}
	if _, ok := b.(*BasicAuth); !ok {
		t.Fatalf("expected BasicAuth got %T", b)
	}

	oauthJSON := []byte(`{"auth_type":"oauth2","token_url":"http://t","client_id":"cid","client_secret":"sec"}`)
	o, err := unmarshalAuth(oauthJSON)
	if err != nil {
		t.Fatalf("oauth err: %v", err)
	}
	if _, ok := o.(*OAuth2Auth); !ok {
		t.Fatalf("expected OAuth2Auth got %T", o)
	}
}

func TestUnmarshalProvider_MoreTypes(t *testing.T) {
	cases := []struct {
		json string
		typ  ProviderType
	}{
		{`{"provider_type":"http_stream","name":"hs","url":"http://x","http_method":"GET","auth":{"auth_type":"api_key","api_key":"k","var_name":"X","location":"header"}}`, ProviderHTTPStream},
		{`{"provider_type":"websocket","name":"ws","url":"ws://x","auth":{"auth_type":"api_key","api_key":"k","var_name":"X","location":"header"}}`, ProviderWebSocket},
		{`{"provider_type":"grpc","name":"g","host":"h","port":1,"service_name":"s","method_name":"m","auth":{"auth_type":"api_key","api_key":"k","var_name":"X","location":"header"}}`, ProviderGRPC},
		{`{"provider_type":"graphql","name":"gql","url":"http://g","operation_type":"query","auth":{"auth_type":"api_key","api_key":"k","var_name":"X","location":"header"}}`, ProviderGraphQL},
		{`{"provider_type":"webrtc","name":"w","signaling_server":"s","peer_id":"p","data_channel_name":"d"}`, ProviderWebRTC},
	}
	for _, c := range cases {
		p, err := UnmarshalProvider([]byte(c.json))
		if err != nil {
			t.Errorf("unmarshal error for %s: %v", c.typ, err)
			continue
		}
		if p.Type() != c.typ {
			t.Errorf("type mismatch: got %s want %s", p.Type(), c.typ)
		}
	}
	if _, err := UnmarshalProvider([]byte(`{"provider_type":"unknown"}`)); err == nil {
		t.Errorf("expected error for unknown provider")
	}
}

func TestMCPProvider_Basic(t *testing.T) {
	p := NewMCPProvider("n", []string{"/home/raezil/go-utcp/examples/mcp_client/mcp_server"})
	if p.Type() != ProviderType("mcp") {
		t.Fatalf("Type mismatch")
	}
	if p.Name != "n" {
		t.Fatalf("Name mismatch")
	}
}

func TestUnmarshalAuth_Errors(t *testing.T) {
	if _, err := unmarshalAuth([]byte(`{"auth_type":"unknown"}`)); err == nil {
		t.Fatalf("expected error for unknown type")
	}
	if _, err := unmarshalAuth([]byte(`{`)); err == nil {
		t.Fatalf("expected json error")
	}
}
