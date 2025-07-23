package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/concepts"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers"
	"gopkg.in/yaml.v2"
)

// HttpClientTransport implements ClientTransportInterface for HTTP-based tool providers.
type HttpClientTransport struct {
	httpClient  *http.Client
	oauthTokens map[string]map[string]interface{}
	logger      func(format string, args ...interface{})
}

// NewHttpClientTransport constructs a new HttpClientTransport.
func NewHttpClientTransport(logger func(format string, args ...interface{})) *HttpClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &HttpClientTransport{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		oauthTokens: make(map[string]map[string]interface{}),
		logger:      logger,
	}
}

// DeregisterToolProvider is a no-op for CLI transport.
func (t *HttpClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	// stateless
	return nil
}

// applyAuth applies authentication to the request based on provider config.
func (t *HttpClientTransport) applyAuth(req *http.Request, provider *HttpProvider) error {
	if provider.Auth == nil {
		return nil
	}
	authIfc := *provider.Auth
	switch a := authIfc.(type) {
	case *ApiKeyAuth:
		if a.APIKey == "" {
			t.logger("API key not found for ApiKeyAuth.")
			return errors.New("API key for ApiKeyAuth not found")
		}
		switch a.Location {
		case "header":
			req.Header.Set(a.VarName, a.APIKey)
		case "query":
			q := req.URL.Query()
			q.Set(a.VarName, a.APIKey)
			req.URL.RawQuery = q.Encode()
		case "cookie":
			req.AddCookie(&http.Cookie{Name: a.VarName, Value: a.APIKey})
		}
	case *BasicAuth:
		req.SetBasicAuth(a.Username, a.Password)
	}
	return nil
}

// handleOAuth2 performs client credentials flow for OAuth2.
func (t *HttpClientTransport) handleOAuth2(ctx context.Context, oauth *OAuth2Auth) (string, error) {
	if tokenData, ok := t.oauthTokens[oauth.ClientID]; ok {
		if access, exists := tokenData["access_token"].(string); exists {
			return access, nil
		}
	}
	// Try credentials in body
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", oauth.ClientID)
	form.Set("client_secret", oauth.ClientSecret)
	form.Set("scope", *oauth.Scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauth.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.httpClient.Do(req)
	if err == nil && resp.StatusCode < 300 {
		defer resp.Body.Close()
		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			t.oauthTokens[oauth.ClientID] = data
			if tok, ok := data["access_token"].(string); ok {
				return tok, nil
			}
		}
	}
	// Fallback: Basic Auth header
	req2, err := http.NewRequestWithContext(ctx, http.MethodPost, oauth.TokenURL, strings.NewReader("grant_type=client_credentials&scope="+url.QueryEscape(*oauth.Scope)))
	if err != nil {
		return "", err
	}
	req2.SetBasicAuth(oauth.ClientID, oauth.ClientSecret)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp2, err := t.httpClient.Do(req2)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()
	var data2 map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&data2); err != nil {
		return "", err
	}
	t.oauthTokens[oauth.ClientID] = data2
	if tok, ok := data2["access_token"].(string); ok {
		return tok, nil
	}
	return "", errors.New("access_token not found in OAuth2 response")
}

// RegisterToolProvider discovers tools from a REST HttpProvider.
func (t *HttpClientTransport) RegisterToolProvider(ctx context.Context, p Provider) ([]Tool, error) {
	hp, ok := p.(*HttpProvider)
	if !ok {
		return nil, errors.New("HttpTransport can only be used with HttpProvider")
	}
	urlStr := hp.URL
	if !(strings.HasPrefix(urlStr, "https://") || strings.HasPrefix(urlStr, "http://localhost") || strings.HasPrefix(urlStr, "http://127.0.0.1")) {
		return nil, fmt.Errorf("security error: URL must use HTTPS or localhost; got: %s", urlStr)
	}
	t.logger("Discovering tools from '%s' at %s", hp.Name, urlStr)

	req, err := http.NewRequestWithContext(ctx, hp.HTTPMethod, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header = make(http.Header)
	for k, v := range hp.Headers {
		req.Header.Set(k, v)
	}
	if err := t.applyAuth(req, hp); err != nil {
		return nil, err
	}
	// OAuth2
	if hp.Auth != nil {
		authIfc := *hp.Auth
		if oauth, ok := authIfc.(*OAuth2Auth); ok {
			token, err := t.handleOAuth2(ctx, oauth)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		t.logger("Error connecting to %s: %v", hp.Name, err)
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		t.logger("Error response from %s: %s", hp.Name, resp.Status)
		return nil, nil
	}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw interface{}
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "yaml") || strings.HasSuffix(urlStr, ".yaml") || strings.HasSuffix(urlStr, ".yml") {
		yaml.Unmarshal(bodyBytes, &raw)
	} else {
		json.Unmarshal(bodyBytes, &raw)
	}
	// Parse UTCP manual
	var manual UtcpManual
	mmap, ok := raw.(map[string]interface{})
	if ok {
		if _, hasVer := mmap["version"]; hasVer {
			manual = NewUtcpManualFromMap(mmap)
		} else {
			converter := NewOpenAPIConverter(raw, hp.URL, hp.Name)
			manual = converter.Convert()
		}
	}
	return manual.Tools, nil
}

// CallTool calls a specific tool on the HTTP provider.
// CallTool calls a specific tool on the HTTP provider.
func (t *HttpClientTransport) CallTool(ctx context.Context, toolName string, args map[string]any, p Provider, l *string) (any, error) {
	hp, ok := p.(*HttpProvider)
	if !ok {
		return nil, errors.New("HttpTransport can only be used with HttpProvider")
	}

	// Use the URL as-is from the provider - this allows flexibility in URL patterns
	urlTemplate := hp.URL

	// Handle URL template substitution for path parameters
	for key, val := range args {
		placeholder := fmt.Sprintf("{%s}", key)
		if strings.Contains(urlTemplate, placeholder) {
			urlTemplate = strings.ReplaceAll(urlTemplate, placeholder, fmt.Sprintf("%v", val))
			delete(args, key)
		}
	}

	u, err := url.Parse(urlTemplate)
	if err != nil {
		return nil, err
	}

	var req *http.Request

	// Determine request method and body based on remaining args and HTTP method
	if len(args) > 0 && hp.HTTPMethod == "POST" {
		// POST with JSON body
		jsonData, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		req, err = http.NewRequestWithContext(ctx, hp.HTTPMethod, u.String(), strings.NewReader(string(jsonData)))
		if err != nil {
			return nil, err
		}
		req.Header = make(http.Header)
		req.Header.Set("Content-Type", "application/json")
	} else {
		// GET or POST with query parameters
		q := u.Query()
		for k, v := range args {
			q.Set(k, fmt.Sprintf("%v", v))
		}
		u.RawQuery = q.Encode()

		req, err = http.NewRequestWithContext(ctx, hp.HTTPMethod, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header = make(http.Header)
	}

	// Copy headers from provider config
	for k, v := range hp.Headers {
		req.Header.Set(k, v)
	}

	if err := t.applyAuth(req, hp); err != nil {
		return nil, err
	}

	// OAuth2
	if hp.Auth != nil {
		authIfc := *hp.Auth
		if oauth, ok := authIfc.(*OAuth2Auth); ok {
			token, err := t.handleOAuth2(ctx, oauth)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		t.logger("Error calling tool %s: %v", toolName, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tool %s returned error status: %s", toolName, resp.Status)
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
