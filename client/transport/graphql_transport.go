package transport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"server"
	"strings"
	"sync"

	"github.com/machinebox/graphql"
)

// GraphQLClientTransport is a simple, robust, production-ready GraphQL transport using gql.
// Stateless, per-operation. Supports all GraphQL features.
type GraphQLClientTransport struct {
	log         func(msg string, err error)
	oauthTokens map[string]OAuth2TokenResponse
	mu          sync.Mutex
}

// OAuth2TokenResponse holds the response fields from an OAuth2 token endpoint.
type OAuth2TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

// NewGraphQLClientTransport creates a new transport instance.
func NewGraphQLClientTransport(logger func(msg string, err error)) *GraphQLClientTransport {
	if logger == nil {
		logger = func(msg string, err error) {}
	}
	return &GraphQLClientTransport{
		log:         logger,
		oauthTokens: make(map[string]OAuth2TokenResponse),
	}
}

// enforceHTTPSOrLocalhost ensures secure or local URLs.
func (t *GraphQLClientTransport) enforceHTTPSOrLocalhost(urlStr string) error {
	if !(strings.HasPrefix(urlStr, "https://") || strings.HasPrefix(urlStr, "http://localhost") || strings.HasPrefix(urlStr, "http://127.0.0.1")) {
		return fmt.Errorf("security error: URL must use HTTPS or start with 'http://localhost' or 'http://127.0.0.1'. Got: %s", urlStr)
	}
	return nil
}

// handleOAuth2 fetches and caches client credentials tokens.
func (t *GraphQLClientTransport) handleOAuth2(ctx context.Context, auth *server.OAuth2Auth) (string, error) {
	t.mu.Lock()
	if token, ok := t.oauthTokens[auth.ClientID]; ok {
		t.mu.Unlock()
		return token.AccessToken, nil
	}
	t.mu.Unlock()

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", auth.ClientID)
	data.Set("client_secret", auth.ClientSecret)
	data.Set("scope", *auth.Scope)

	req, err := http.NewRequestWithContext(ctx, "POST", auth.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("token request failed: %s", string(body))
	}
	var tokenResp OAuth2TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	t.mu.Lock()
	t.oauthTokens[auth.ClientID] = tokenResp
	t.mu.Unlock()
	return tokenResp.AccessToken, nil
}

// prepareHeaders constructs HTTP headers including auth.
func (t *GraphQLClientTransport) prepareHeaders(
	ctx context.Context,
	prov *server.GraphQLProvider,
) (map[string]string, error) {
	headers := make(map[string]string)

	// 1) Copy any user‑supplied headers (defensive nil-check)
	if prov.Headers != nil {
		for k, v := range prov.Headers {
			headers[k] = v
		}
	}

	// 2) If there's no Auth pointer, we're done
	if prov.Auth == nil {
		return headers, nil
	}

	// 3) Dereference to get the actual Auth interface
	authIface := *prov.Auth
	if authIface == nil {
		return headers, nil
	}

	// 4) Type‑switch on the real auth type
	switch auth := authIface.(type) {
	case *server.ApiKeyAuth:
		// only inject into headers if Location == "header"
		if strings.EqualFold(auth.Location, "header") && auth.APIKey != "" {
			headers[auth.VarName] = auth.APIKey
		} else if !strings.EqualFold(auth.Location, "header") {
			return nil, fmt.Errorf(
				"apikey location %q not supported for headers",
				auth.Location,
			)
		}

	case *server.BasicAuth:
		// always go in Authorization header
		creds := auth.Username + ":" + auth.Password
		encoded := base64.StdEncoding.EncodeToString([]byte(creds))
		headers["Authorization"] = "Basic " + encoded

	case *server.OAuth2Auth:
		token, err := t.handleOAuth2(ctx, auth)
		if err != nil {
			return nil, fmt.Errorf("oauth2 token error: %w", err)
		}
		headers["Authorization"] = "Bearer " + token

	default:
		return nil, fmt.Errorf("unrecognized auth type %T", authIface)
	}

	return headers, nil
}

// RegisterToolProvider discovers schema and registers tools.
func (t *GraphQLClientTransport) RegisterToolProvider(ctx context.Context, manualProv server.Provider) ([]server.Tool, error) {
	prov, ok := manualProv.(*server.GraphQLProvider)
	if !ok {
		return nil, errors.New("GraphQLClientTransport can only be used with GraphQLProvider")
	}
	if err := t.enforceHTTPSOrLocalhost(prov.URL); err != nil {
		return nil, err
	}
	headers, err := t.prepareHeaders(ctx, prov)
	if err != nil {
		return nil, err
	}
	client := graphql.NewClient(prov.URL)
	client.Log = func(s string) { t.log(s, nil) }

	// Introspection
	var schema struct {
		__Schema struct {
			QueryType struct {
				Fields []struct {
					Name        string
					Description *string
				}
			}
			MutationType struct {
				Fields []struct {
					Name        string
					Description *string
				}
			}
		} `json:"__schema"`
	}
	req := graphql.NewRequest(`query { __schema { queryType { fields { name description } } mutationType { fields { name description } } } }`)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if err := client.Run(ctx, req, &schema); err != nil {
		return nil, err
	}
	var toolsList []server.Tool
	for _, f := range schema.__Schema.QueryType.Fields {
		desc := ""
		if f.Description != nil {
			desc = *f.Description
		}
		toolsList = append(toolsList, server.Tool{Name: f.Name, Description: desc, Inputs: server.ToolInputOutputSchema{Required: nil}, Provider: prov})
	}
	for _, f := range schema.__Schema.MutationType.Fields {
		desc := ""
		if f.Description != nil {
			desc = *f.Description
		}
		toolsList = append(toolsList, server.Tool{Name: f.Name, Description: desc, Inputs: server.ToolInputOutputSchema{Required: nil}, Provider: prov})
	}
	return toolsList, nil
}

// DeregisterToolProvider is a no-op for stateless transport.
func (t *GraphQLClientTransport) DeregisterToolProvider(ctx context.Context, manualProv server.Provider) error {
	return nil
}

// CallTool executes a GraphQL operation by name.
func (t *GraphQLClientTransport) CallTool(ctx context.Context, toolName string, arguments map[string]any, toolProvider server.Provider) (any, error) {
	prov, ok := toolProvider.(*server.GraphQLProvider)
	if !ok {
		return nil, errors.New("GraphQLClientTransport can only be used with GraphQLProvider")
	}
	if err := t.enforceHTTPSOrLocalhost(prov.URL); err != nil {
		return nil, err
	}
	headers, err := t.prepareHeaders(ctx, prov)
	if err != nil {
		return nil, err
	}
	client := graphql.NewClient(prov.URL)
	client.Log = func(s string) { t.log(s, nil) }

	// build simple query
	var b strings.Builder
	b.WriteString("query ")
	var defs, passes []string
	for k := range arguments {
		defs = append(defs, fmt.Sprintf("$%s: String", k))
		passes = append(passes, fmt.Sprintf("%s: $%s", k, k))
	}
	if len(defs) > 0 {
		b.WriteString("(" + strings.Join(defs, ", ") + ") ")
	}
	b.WriteString("{ " + toolName)
	if len(passes) > 0 {
		b.WriteString("(" + strings.Join(passes, ", ") + ")")
	}
	b.WriteString(" }")
	req := graphql.NewRequest(b.String())

	for k, v := range arguments {
		req.Var(k, v)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	var resp map[string]interface{}
	if err := client.Run(ctx, req, &resp); err != nil {
		return nil, err
	}
	if data, ok := resp[toolName]; ok {
		return data, nil
	}
	return resp, nil
}

// Close clears cached tokens.
func (t *GraphQLClientTransport) Close() error {
	t.mu.Lock()
	t.oauthTokens = make(map[string]OAuth2TokenResponse)
	t.mu.Unlock()
	return nil
}
