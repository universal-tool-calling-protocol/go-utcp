package adk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// HTTPDoer is implemented by *http.Client. It allows provider clients to be
// configured with custom transports while remaining testable.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// --- OpenAI -----------------------------------------------------------------

type openAIConfig struct {
	httpClient HTTPDoer
	baseURL    string
}

// OpenAIOption configures a new OpenAIClient instance.
type OpenAIOption func(*openAIConfig)

// WithOpenAIHTTPClient overrides the HTTP client used to communicate with the
// OpenAI API.
func WithOpenAIHTTPClient(client HTTPDoer) OpenAIOption {
	return func(cfg *openAIConfig) {
		cfg.httpClient = client
	}
}

// WithOpenAIBaseURL sets a custom base URL for the OpenAI API. This is
// primarily useful for testing.
func WithOpenAIBaseURL(baseURL string) OpenAIOption {
	return func(cfg *openAIConfig) {
		if strings.TrimSpace(baseURL) != "" {
			cfg.baseURL = baseURL
		}
	}
}

// OpenAIClient implements the LLM interface using OpenAI's Chat Completions
// endpoint.
type OpenAIClient struct {
	httpClient HTTPDoer
	apiKey     string
	model      string
	baseURL    string
}

// NewOpenAIClient constructs a client capable of invoking OpenAI chat models.
func NewOpenAIClient(apiKey, model string, opts ...OpenAIOption) *OpenAIClient {
	if strings.TrimSpace(apiKey) == "" {
		panic("openai api key must not be empty")
	}
	if strings.TrimSpace(model) == "" {
		panic("openai model must not be empty")
	}

	cfg := &openAIConfig{
		httpClient: http.DefaultClient,
		baseURL:    "https://api.openai.com/v1/chat/completions",
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.httpClient == nil {
		cfg.httpClient = http.DefaultClient
	}

	return &OpenAIClient{
		httpClient: cfg.httpClient,
		apiKey:     apiKey,
		model:      model,
		baseURL:    cfg.baseURL,
	}
}

// Generate fulfils the LLM interface by issuing a chat completion request.
func (c *OpenAIClient) Generate(ctx context.Context, prompt string) (string, error) {
	payload := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("openai request failed: %s", strings.TrimSpace(string(body)))
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("openai response missing choices")
	}
	return strings.TrimSpace(decoded.Choices[0].Message.Content), nil
}

// --- Gemini -----------------------------------------------------------------

type geminiConfig struct {
	httpClient HTTPDoer
	baseURL    string
}

// GeminiOption configures a GeminiClient.
type GeminiOption func(*geminiConfig)

// WithGeminiHTTPClient sets a custom HTTP client.
func WithGeminiHTTPClient(client HTTPDoer) GeminiOption {
	return func(cfg *geminiConfig) {
		cfg.httpClient = client
	}
}

// WithGeminiBaseURL overrides the base API URL, useful for tests.
func WithGeminiBaseURL(baseURL string) GeminiOption {
	return func(cfg *geminiConfig) {
		if strings.TrimSpace(baseURL) != "" {
			cfg.baseURL = baseURL
		}
	}
}

// GeminiClient talks to the Google Gemini API.
type GeminiClient struct {
	httpClient HTTPDoer
	apiKey     string
	model      string
	baseURL    string
}

// NewGeminiClient constructs a Gemini backed LLM implementation.
func NewGeminiClient(apiKey, model string, opts ...GeminiOption) *GeminiClient {
	if strings.TrimSpace(apiKey) == "" {
		panic("gemini api key must not be empty")
	}
	if strings.TrimSpace(model) == "" {
		panic("gemini model must not be empty")
	}

	cfg := &geminiConfig{
		httpClient: http.DefaultClient,
		baseURL:    "https://generativelanguage.googleapis.com",
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.httpClient == nil {
		cfg.httpClient = http.DefaultClient
	}

	return &GeminiClient{
		httpClient: cfg.httpClient,
		apiKey:     apiKey,
		model:      model,
		baseURL:    cfg.baseURL,
	}
}

// Generate requests a completion from the Gemini API.
func (c *GeminiClient) Generate(ctx context.Context, prompt string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	base.Path = path.Join(base.Path, "v1beta", "models", c.model+":generateContent")
	q := base.Query()
	q.Set("key", c.apiKey)
	base.RawQuery = q.Encode()

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("gemini request failed: %s", strings.TrimSpace(string(body)))
	}

	var decoded struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if len(decoded.Candidates) == 0 || len(decoded.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("gemini response missing candidates")
	}
	return strings.TrimSpace(decoded.Candidates[0].Content.Parts[0].Text), nil
}

// --- Ollama -----------------------------------------------------------------

type ollamaConfig struct {
	httpClient HTTPDoer
	baseURL    string
}

// OllamaOption configures an OllamaClient.
type OllamaOption func(*ollamaConfig)

// WithOllamaHTTPClient overrides the HTTP client used to communicate with the
// Ollama daemon.
func WithOllamaHTTPClient(client HTTPDoer) OllamaOption {
	return func(cfg *ollamaConfig) {
		cfg.httpClient = client
	}
}

// WithOllamaBaseURL changes the API endpoint.
func WithOllamaBaseURL(baseURL string) OllamaOption {
	return func(cfg *ollamaConfig) {
		if strings.TrimSpace(baseURL) != "" {
			cfg.baseURL = baseURL
		}
	}
}

// OllamaClient issues requests to a running Ollama instance.
type OllamaClient struct {
	httpClient HTTPDoer
	model      string
	baseURL    string
}

// NewOllamaClient returns an LLM implementation powered by Ollama.
func NewOllamaClient(model string, opts ...OllamaOption) *OllamaClient {
	if strings.TrimSpace(model) == "" {
		panic("ollama model must not be empty")
	}

	cfg := &ollamaConfig{
		httpClient: http.DefaultClient,
		baseURL:    "http://localhost:11434/api/generate",
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.httpClient == nil {
		cfg.httpClient = http.DefaultClient
	}

	return &OllamaClient{
		httpClient: cfg.httpClient,
		model:      model,
		baseURL:    cfg.baseURL,
	}
}

// Generate requests a prediction from the configured Ollama model.
func (c *OllamaClient) Generate(ctx context.Context, prompt string) (string, error) {
	payload := map[string]any{
		"model":  c.model,
		"prompt": prompt,
		"stream": false,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("ollama request failed: %s", strings.TrimSpace(string(body)))
	}

	var decoded struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	return strings.TrimSpace(decoded.Response), nil
}

// Utility used in tests to ensure streaming responses can be aggregated when
// Ollama is configured with stream=true.
func collectStreamingOllamaResponse(body io.Reader) (string, error) {
	scanner := bufio.NewScanner(body)
	var b strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
		}
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return "", err
		}
		b.WriteString(chunk.Response)
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return b.String(), nil
}
