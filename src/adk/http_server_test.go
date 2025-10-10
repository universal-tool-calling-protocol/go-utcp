package adk

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPHandler(t *testing.T) {
	agent := NewAgent("http-agent")
	agent.MustRegisterTool(ToolDefinition{
		Name:        "hello",
		Description: "Says hello",
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			name, _ := input["name"].(string)
			if name == "" {
				name = "world"
			}
			return map[string]any{"greeting": "hello " + name}, nil
		},
	})

	handler := agent.HTTPHandler()

	req := httptest.NewRequest(http.MethodGet, "/manual", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("manual request failed: %d", rr.Code)
	}
	var manualResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &manualResp); err != nil {
		t.Fatalf("failed to decode manual response: %v", err)
	}
	tools, ok := manualResp["Tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected manual to contain a single tool: %#v", manualResp)
	}

	payload := map[string]any{
		"tool":  "hello",
		"input": map[string]any{"name": "utcp"},
	}
	body, _ := json.Marshal(payload)
	req = httptest.NewRequest(http.MethodPost, "/call", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("call endpoint returned %d", rr.Code)
	}
	var resp callResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode call response: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("expected no error, got %s", resp.Error)
	}
	if resp.Output["greeting"] != "hello utcp" {
		t.Fatalf("unexpected output: %#v", resp.Output)
	}
}
