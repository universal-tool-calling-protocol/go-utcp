package adk

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIClientGenerate(t *testing.T) {
	t.Parallel()

	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(body), "\"model\":\"gpt-test\"") {
			t.Fatalf("unexpected body: %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Hello from OpenAI"}}]}`))
	}))
	t.Cleanup(server.Close)

	client := NewOpenAIClient("secret", "gpt-test",
		WithOpenAIBaseURL(server.URL),
	)

	got, err := client.Generate(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if want := "Hello from OpenAI"; got != want {
		t.Fatalf("unexpected response: got %q want %q", got, want)
	}
	if capturedAuth != "Bearer secret" {
		t.Fatalf("unexpected auth header: %q", capturedAuth)
	}
}

func TestGeminiClientGenerate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-pro:generateContent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if q := r.URL.Query().Get("key"); q != "gem-key" {
			t.Fatalf("missing key query parameter: %s", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Gemini says hi"}]}}]}`))
	}))
	t.Cleanup(server.Close)

	client := NewGeminiClient("gem-key", "gemini-pro",
		WithGeminiBaseURL(server.URL),
	)

	got, err := client.Generate(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if want := "Gemini says hi"; got != want {
		t.Fatalf("unexpected response: got %q want %q", got, want)
	}
}

func TestOllamaClientGenerate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"Hello from Ollama"}`))
	}))
	t.Cleanup(server.Close)

	client := NewOllamaClient("llama3",
		WithOllamaBaseURL(server.URL),
	)

	got, err := client.Generate(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if want := "Hello from Ollama"; got != want {
		t.Fatalf("unexpected response: got %q want %q", got, want)
	}
}

func TestCollectStreamingOllamaResponse(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		`{"response":"Hello, "}`,
		`{"response":"world","done":false}`,
		`{"response":"!","done":true}`,
	}, "\n")
	got, err := collectStreamingOllamaResponse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if want := "Hello, world!"; got != want {
		t.Fatalf("unexpected aggregation: got %q want %q", got, want)
	}
}
