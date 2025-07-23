package graphql

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers"
)

// helper pointer to string
func sptr(s string) *string { return &s }

// TestGraphQLTransport_prepareHeaders various auth cases.
func TestGraphQLTransport_prepareHeaders(t *testing.T) {
	tr := NewGraphQLClientTransport(nil)
	ctx := context.Background()
	prov := &GraphQLProvider{URL: "https://example.com"}

	// no auth
	hdr, err := tr.prepareHeaders(ctx, prov)
	if err != nil || len(hdr) != 0 {
		t.Fatalf("unexpected headers %v err %v", hdr, err)
	}

	// api key header
	a := &ApiKeyAuth{AuthType: APIKeyType, APIKey: "k", VarName: "X", Location: "header"}
	var auth Auth = a
	prov.Auth = &auth
	hdr, err = tr.prepareHeaders(ctx, prov)
	if err != nil || hdr["X"] != "k" {
		t.Fatalf("apikey header failed: %v %v", hdr, err)
	}

	// invalid api key location
	a.Location = "query"
	hdr, err = tr.prepareHeaders(ctx, prov)
	if err == nil {
		t.Fatalf("expected error for bad location")
	}

	// basic auth
	b := &BasicAuth{AuthType: BasicType, Username: "u", Password: "p"}
	auth = b
	prov.Auth = &auth
	hdr, err = tr.prepareHeaders(ctx, prov)
	if err != nil || hdr["Authorization"] == "" {
		t.Fatalf("basic auth failed: %v %v", hdr, err)
	}

	// oauth2 auth using test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	}))
	defer ts.Close()
	o := &OAuth2Auth{AuthType: OAuth2Type, TokenURL: ts.URL, ClientID: "id", ClientSecret: "sec", Scope: sptr("scope")}
	auth = o
	prov.Auth = &auth
	hdr, err = tr.prepareHeaders(ctx, prov)
	if err != nil || hdr["Authorization"] != "Bearer tok" {
		t.Fatalf("oauth2 auth failed: %v %v", hdr, err)
	}

	// verify Close clears cache
	tr.Close()
	if len(tr.oauthTokens) != 0 {
		t.Fatalf("Close should clear tokens")
	}
}

// TestGraphQL_enforceHTTPS ensures invalid URLs are rejected.
func TestGraphQL_enforceHTTPS(t *testing.T) {
	tr := NewGraphQLClientTransport(nil)
	if err := tr.enforceHTTPSOrLocalhost("http://example.com"); err == nil {
		t.Fatalf("expected error for insecure URL")
	}
	if err := tr.enforceHTTPSOrLocalhost("https://good.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestGraphQL_handleOAuth2 covers token caching.
func TestGraphQL_handleOAuth2(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	}))
	defer ts.Close()

	old := http.DefaultClient
	http.DefaultClient = ts.Client()
	defer func() { http.DefaultClient = old }()

	tr := NewGraphQLClientTransport(nil)
	oauth := &OAuth2Auth{AuthType: OAuth2Type, TokenURL: ts.URL, ClientID: "id", ClientSecret: "sec", Scope: sptr("s")}
	tok, err := tr.handleOAuth2(context.Background(), oauth)
	if err != nil || tok != "tok" {
		t.Fatalf("first call failed: %s %v", tok, err)
	}
	ts.Close() // further network calls would fail
	tok2, err := tr.handleOAuth2(context.Background(), oauth)
	if err != nil || tok2 != "tok" {
		t.Fatalf("cached call failed: %s %v", tok2, err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 network call, got %d", calls)
	}
}

// TestGraphQL_RegisterAndCall_Errors exercises error branches.
func TestGraphQL_RegisterAndCall_Errors(t *testing.T) {
	tr := NewGraphQLClientTransport(nil)
	ctx := context.Background()
	// wrong provider type
	if _, err := tr.RegisterToolProvider(ctx, &CliProvider{}); err == nil {
		t.Fatalf("expected error for wrong provider")
	}
	if _, err := tr.CallTool(ctx, "x", nil, &CliProvider{}, nil); err == nil {
		t.Fatalf("expected error for wrong provider")
	}
	prov := &GraphQLProvider{URL: "http://example.com"}
	if _, err := tr.RegisterToolProvider(ctx, prov); err == nil {
		t.Fatalf("expected https enforcement error")
	}
	if _, err := tr.CallTool(ctx, "foo", nil, prov, nil); err == nil {
		t.Fatalf("expected https enforcement error")
	}
}

// TestGraphQL_CallTool_NoData ensures result map when tool key missing.
func TestGraphQL_CallTool_NoData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"other": 1}})
	}))
	defer server.Close()
	prov := &GraphQLProvider{URL: server.URL}
	tr := NewGraphQLClientTransport(nil)
	res, err := tr.CallTool(context.Background(), "foo", nil, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	m, ok := res.(map[string]interface{})
	if !ok || m["other"] != float64(1) {
		t.Fatalf("unexpected result: %#v", res)
	}
}
