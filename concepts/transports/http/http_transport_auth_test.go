package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/concepts/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/concepts/providers"
)

func TestHttpClientTransport_applyAuth(t *testing.T) {
	tr := NewHttpClientTransport(nil)
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	keyAuth := &ApiKeyAuth{AuthType: APIKeyType, APIKey: "k", VarName: "X", Location: "header"}
	var a Auth = keyAuth
	if err := tr.applyAuth(req, &HttpProvider{Auth: &a}); err != nil {
		t.Fatalf("applyAuth err: %v", err)
	}
	if req.Header.Get("X") != "k" {
		t.Fatalf("header not set")
	}

	req2, _ := http.NewRequest("GET", "http://e.com?q=1", nil)
	keyAuth.Location = "query"
	a = keyAuth
	if err := tr.applyAuth(req2, &HttpProvider{Auth: &a}); err != nil {
		t.Fatalf("applyAuth query err: %v", err)
	}
	if req2.URL.Query().Get("X") != "k" {
		t.Fatalf("query not set")
	}

	req3, _ := http.NewRequest("GET", "http://e.com", nil)
	keyAuth.Location = "cookie"
	a = keyAuth
	if err := tr.applyAuth(req3, &HttpProvider{Auth: &a}); err != nil {
		t.Fatalf("applyAuth cookie err: %v", err)
	}
	if c, err := req3.Cookie("X"); err != nil || c.Value != "k" {
		t.Fatalf("cookie not set")
	}

	basic := &BasicAuth{AuthType: BasicType, Username: "u", Password: "p"}
	req4, _ := http.NewRequest("GET", "http://e.com", nil)
	a = basic
	if err := tr.applyAuth(req4, &HttpProvider{Auth: &a}); err != nil {
		t.Fatalf("basic err: %v", err)
	}
	if req4.Header.Get("Authorization") == "" {
		t.Fatalf("basic header missing")
	}
}

func TestHttpClientTransport_handleOAuth2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"tok"}`))
	}))
	defer server.Close()

	tr := NewHttpClientTransport(nil)
	tr.httpClient = server.Client()
	oauth := &OAuth2Auth{AuthType: OAuth2Type, TokenURL: server.URL, ClientID: "id", ClientSecret: "sec", Scope: ptr("scope")}
	tok, err := tr.handleOAuth2(context.Background(), oauth)
	if err != nil || tok != "tok" {
		t.Fatalf("got %s err %v", tok, err)
	}
	// second call should use cached token
	tok2, err := tr.handleOAuth2(context.Background(), oauth)
	if err != nil || tok2 != "tok" {
		t.Fatalf("cached token %s err %v", tok2, err)
	}
}

func ptr(s string) *string { return &s }

func TestHttpClientTransport_handleOAuth2_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer server.Close()
	tr := NewHttpClientTransport(nil)
	tr.httpClient = server.Client()
	oauth := &OAuth2Auth{AuthType: OAuth2Type, TokenURL: server.URL, ClientID: "id", ClientSecret: "sec", Scope: ptr("s")}
	if _, err := tr.handleOAuth2(context.Background(), oauth); err == nil {
		t.Fatalf("expected error")
	}
}
