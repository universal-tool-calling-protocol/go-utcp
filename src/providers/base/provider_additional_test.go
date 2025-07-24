package base

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
)

func TestUnmarshalAuth_Types(t *testing.T) {
	apiJSON := []byte(`{"auth_type":"api_key","api_key":"secret","var_name":"X","location":"header"}`)
	a, err := UnmarshalAuth(apiJSON)
	if err != nil {
		t.Fatalf("api key err: %v", err)
	}
	if _, ok := a.(*ApiKeyAuth); !ok {
		t.Fatalf("expected ApiKeyAuth got %T", a)
	}

	basicJSON := []byte(`{"auth_type":"basic","username":"u","password":"p"}`)
	b, err := UnmarshalAuth(basicJSON)
	if err != nil {
		t.Fatalf("basic err: %v", err)
	}
	if _, ok := b.(*BasicAuth); !ok {
		t.Fatalf("expected BasicAuth got %T", b)
	}

	oauthJSON := []byte(`{"auth_type":"oauth2","token_url":"http://t","client_id":"cid","client_secret":"sec"}`)
	o, err := UnmarshalAuth(oauthJSON)
	if err != nil {
		t.Fatalf("oauth err: %v", err)
	}
	if _, ok := o.(*OAuth2Auth); !ok {
		t.Fatalf("expected OAuth2Auth got %T", o)
	}
}
