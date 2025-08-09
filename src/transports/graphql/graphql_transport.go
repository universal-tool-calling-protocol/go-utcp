package graphql

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/graphql"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

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

// TypedArgument allows passing type information with arguments
type TypedArgument struct {
	Value interface{}
	Type  string // GraphQL type like "String", "Int", "Boolean", "MyInputType"
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
	if !(strings.HasPrefix(urlStr, "https://") || strings.HasPrefix(urlStr, "http://localhost") || strings.HasPrefix(urlStr, "http://127.0.0.1") || strings.HasPrefix(urlStr, "wss://") || strings.HasPrefix(urlStr, "ws://localhost") || strings.HasPrefix(urlStr, "ws://127.0.0.1")) {
		return fmt.Errorf("security error: URL must use HTTPS/WSS or start with 'http://localhost', 'http://127.0.0.1', 'ws://localhost', or 'ws://127.0.0.1'. Got: %s", urlStr)
	}
	return nil
}

// handleOAuth2 fetches and caches client credentials tokens.
func (t *GraphQLClientTransport) handleOAuth2(ctx context.Context, auth *OAuth2Auth) (string, error) {
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
	prov *GraphQLProvider,
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
	case *ApiKeyAuth:
		// only inject into headers if Location == "header"
		if strings.EqualFold(auth.Location, "header") && auth.APIKey != "" {
			headers[auth.VarName] = auth.APIKey
		} else if !strings.EqualFold(auth.Location, "header") {
			return nil, fmt.Errorf(
				"apikey location %q not supported for headers",
				auth.Location,
			)
		}

	case *BasicAuth:
		// always go in Authorization header
		creds := auth.Username + ":" + auth.Password
		encoded := base64.StdEncoding.EncodeToString([]byte(creds))
		headers["Authorization"] = "Basic " + encoded

	case *OAuth2Auth:
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

// inferGraphQLType attempts to infer GraphQL type from Go value
func (t *GraphQLClientTransport) inferGraphQLType(value interface{}) string {
	if value == nil {
		return "String" // fallback
	}

	switch reflect.TypeOf(value).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "Int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "Int"
	case reflect.Float32, reflect.Float64:
		return "Float"
	case reflect.Bool:
		return "Boolean"
	case reflect.String:
		return "String"
	case reflect.Map, reflect.Struct, reflect.Slice, reflect.Array:
		return "JSON" // fallback for complex types
	default:
		return "String" // safe fallback
	}
}

// RegisterToolProvider discovers the schema and registers GraphQL fields as tools.
func (t *GraphQLClientTransport) RegisterToolProvider(
	ctx context.Context,
	manualProv Provider,
) ([]Tool, error) {
	prov, ok := manualProv.(*GraphQLProvider)
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

	// Determine introspection URL: use HTTP(S) for subscriptions
	introspectURL := prov.URL
	if strings.EqualFold(prov.OperationType, "subscription") {
		u, err := url.Parse(prov.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid provider URL: %w", err)
		}
		// Switch WS->HTTP scheme
		switch u.Scheme {
		case "ws":
			u.Scheme = "http"
		case "wss":
			u.Scheme = "https"
		}
		// Use standard GraphQL HTTP path for introspection
		u.Path = "/graphql"
		introspectURL = u.String()
	}

	client := graphql.NewClient(introspectURL)
	client.Log = func(s string) { t.log(s, nil) }

	// Introspection query
	introspectionQuery := `
	query IntrospectionQuery {
	  __schema {
	    queryType { fields { name description } }
	    mutationType { fields { name description } }
	    subscriptionType { fields { name description } }
	  }
	}`

	req := graphql.NewRequest(introspectionQuery)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Response container for introspection
	var resp struct {
		Schema struct {
			QueryType struct {
				Fields []struct {
					Name        string  `json:"name"`
					Description *string `json:"description"`
				}
			} `json:"queryType"`
			MutationType *struct {
				Fields []struct {
					Name        string  `json:"name"`
					Description *string `json:"description"`
				}
			} `json:"mutationType"`
			SubscriptionType *struct {
				Fields []struct {
					Name        string  `json:"name"`
					Description *string `json:"description"`
				}
			} `json:"subscriptionType"`
		} `json:"__schema"`
	}
	if err := client.Run(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("introspection failed: %w", err)
	}

	// Build tool list
	var toolsList []Tool

	// Register query fields
	for _, f := range resp.Schema.QueryType.Fields {
		desc := ""
		if f.Description != nil {
			desc = *f.Description
		}
		toolsList = append(toolsList, Tool{
			Name:        fmt.Sprintf("%s.%s", prov.Name, f.Name),
			Description: desc,
			Inputs:      ToolInputOutputSchema{Required: nil},
			Provider:    prov,
		})
	}

	// Register mutation fields
	if resp.Schema.MutationType != nil {
		for _, f := range resp.Schema.MutationType.Fields {
			desc := ""
			if f.Description != nil {
				desc = *f.Description
			}
			toolsList = append(toolsList, Tool{
				Name:        fmt.Sprintf("%s.%s", prov.Name, f.Name),
				Description: desc,
				Inputs:      ToolInputOutputSchema{Required: nil},
				Provider:    prov,
			})
		}
	}

	// Register subscription fields
	if resp.Schema.SubscriptionType != nil {
		for _, f := range resp.Schema.SubscriptionType.Fields {
			desc := ""
			if f.Description != nil {
				desc = *f.Description
			}
			toolsList = append(toolsList, Tool{
				Name:        fmt.Sprintf("%s.%s", prov.Name, f.Name),
				Description: desc,
				Inputs:      ToolInputOutputSchema{Required: nil},
				Provider:    prov,
			})
		}
	}
	return toolsList, nil

}

// DeregisterToolProvider is a no-op for stateless transport.
func (t *GraphQLClientTransport) DeregisterToolProvider(ctx context.Context, manualProv Provider) error {
	return nil
}

// CallTool executes a GraphQL operation by name with proper type support.
func (t *GraphQLClientTransport) CallTool(ctx context.Context, toolName string, arguments map[string]any, toolProvider Provider, l *string) (any, error) {
	prov, ok := toolProvider.(*GraphQLProvider)
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

	// Build operation with proper types
	var b strings.Builder
	opType := "query"
	if prov.OperationType != "" {
		opType = strings.ToLower(prov.OperationType)
	}

	// Validate operation type
	if opType != "query" && opType != "mutation" && opType != "subscription" {
		return nil, fmt.Errorf("invalid operation type: %s. Must be query, mutation, or subscription", opType)
	}

	b.WriteString(opType + " ")

	if prov.OperationName != nil {
		b.WriteString(*prov.OperationName + " ")
	}

	// Build variable definitions and argument passes
	var defs, passes []string
	for k, v := range arguments {
		var gqlType string
		// Check if argument is a TypedArgument with explicit type
		if typedArg, ok := v.(TypedArgument); ok {
			gqlType = typedArg.Type
		} else {
			// Infer type from Go value
			gqlType = t.inferGraphQLType(v)
		}
		defs = append(defs, fmt.Sprintf("$%s: %s", k, gqlType))
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

	// Set variables with actual values
	for k, v := range arguments {
		if typedArg, ok := v.(TypedArgument); ok {
			req.Var(k, typedArg.Value)
		} else {
			req.Var(k, v)
		}
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Handle different operation types
	switch opType {
	case "subscription":
		return t.handleSubscription(ctx, client, req, toolName, prov, b.String(), arguments)
	case "mutation":
		return t.handleMutation(ctx, client, req, toolName)
	default: // query
		return t.handleQuery(ctx, client, req, toolName)
	}
}

// handleQuery processes GraphQL queries
func (t *GraphQLClientTransport) handleQuery(ctx context.Context, client *graphql.Client, req *graphql.Request, toolName string) (any, error) {
	var resp map[string]interface{}
	if err := client.Run(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	if data, ok := resp[toolName]; ok {
		return data, nil
	}
	return resp, nil
}

// handleMutation processes GraphQL mutations
func (t *GraphQLClientTransport) handleMutation(ctx context.Context, client *graphql.Client, req *graphql.Request, toolName string) (any, error) {
	var resp map[string]interface{}
	if err := client.Run(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("mutation execution failed: %w", err)
	}

	// Check for GraphQL errors in response
	if errors, ok := resp["errors"]; ok {
		return nil, fmt.Errorf("mutation returned errors: %v", errors)
	}

	if data, ok := resp[toolName]; ok {
		return data, nil
	}
	return resp, nil
}

// handleSubscription processes GraphQL subscriptions
func (t *GraphQLClientTransport) handleSubscription(ctx context.Context, client *graphql.Client, req *graphql.Request, toolName string, prov *GraphQLProvider, query string, variables map[string]any) (any, error) {
	// For subscriptions, we need WebSocket support which the standard graphql.Client doesn't provide
	// Check if the URL is a WebSocket URL
	if strings.HasPrefix(prov.URL, "ws://") || strings.HasPrefix(prov.URL, "wss://") {
		return t.handleWebSocketSubscription(ctx, req, toolName, prov, query, variables)
	}

	// Fallback: Some GraphQL servers support subscriptions over POST (like GraphQL over SSE)
	var resp map[string]interface{}
	if err := client.Run(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("subscription execution failed: %w", err)
	}

	// For subscriptions over HTTP, return the response (might be a single result or setup info)
	if data, ok := resp[toolName]; ok {
		return data, nil
	}
	return resp, nil
}

// handleWebSocketSubscription handles WebSocket-based subscriptions
func (t *GraphQLClientTransport) handleWebSocketSubscription(ctx context.Context, req *graphql.Request, toolName string, prov *GraphQLProvider, query string, variables map[string]any) (any, error) {
	// Create WebSocket connection with the "graphql-ws" subprotocol and propagate request headers
	dialer := websocket.Dialer{Subprotocols: []string{"graphql-ws"}}
	hdr := http.Header{}
	for k, vals := range req.Header {
		for _, v := range vals {
			hdr.Add(k, v)
		}
	}
	conn, _, err := dialer.Dial(prov.URL, hdr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	// Send connection init message (GraphQL WebSocket Protocol)
	initMsg := map[string]interface{}{
		"type": "connection_init",
	}
	if err := conn.WriteJSON(initMsg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send connection init: %w", err)
	}

	// Wait for connection_ack
	var ackMsg map[string]interface{}
	if err := conn.ReadJSON(&ackMsg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read connection ack: %w", err)
	}

	if ackMsg["type"] != "connection_ack" {
		conn.Close()
		return nil, fmt.Errorf("expected connection_ack, got: %v", ackMsg["type"])
	}

	// Prepare variables with proper values
	processedVars := make(map[string]interface{})
	for k, v := range variables {
		if typedArg, ok := v.(TypedArgument); ok {
			processedVars[k] = typedArg.Value
		} else {
			processedVars[k] = v
		}
	}

	// Send subscription start message
	startMsg := map[string]interface{}{
		"id":   "subscription-1",
		"type": "start",
		"payload": map[string]interface{}{
			"query":     query,         // Use the query string we built
			"variables": processedVars, // Use processed variables
		},
	}

	if err := conn.WriteJSON(startMsg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send subscription start: %w", err)
	}

	// Return a subscription result that manages the WebSocket connection
	return &SubscriptionResult{
		conn:     conn,
		toolName: toolName,
		ctx:      ctx,
	}, nil
}

// SubscriptionResult represents the result of a GraphQL subscription
type SubscriptionResult struct {
	conn     *websocket.Conn
	toolName string
	ctx      context.Context
}

// Next returns the next piece of data from the subscription
func (sr *SubscriptionResult) Next() (interface{}, error) {
	for {
		var msg map[string]interface{}
		if err := sr.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				return nil, fmt.Errorf("websocket error: %w", err)
			}
			return nil, io.EOF
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		switch msgType {
		case "data":
			if payload, ok := msg["payload"].(map[string]interface{}); ok {
				if data, ok := payload["data"].(map[string]interface{}); ok {
					if toolData, ok := data[sr.toolName]; ok {
						return toolData, nil
					}
					return data, nil
				}
			}
		case "error":
			if payload, ok := msg["payload"]; ok {
				return nil, fmt.Errorf("subscription error: %v", payload)
			}
			return nil, fmt.Errorf("subscription error")
		case "complete":
			return nil, io.EOF
		}
	}
}

// Close closes the subscription connection
func (sr *SubscriptionResult) Close() error {
	if sr.conn != nil {
		// Send stop message
		stopMsg := map[string]interface{}{
			"id":   "subscription-1",
			"type": "stop",
		}
		sr.conn.WriteJSON(stopMsg)
		return sr.conn.Close()
	}
	return nil
}

// Close clears cached tokens.
func (t *GraphQLClientTransport) Close() error {
	t.mu.Lock()
	t.oauthTokens = make(map[string]OAuth2TokenResponse)
	t.mu.Unlock()
	return nil
}

func (t *GraphQLClientTransport) CallToolStream(
	ctx context.Context,
	toolName string,
	args map[string]any,
	p Provider,
) (transports.StreamResult, error) {
	return nil, errors.New("streaming is supported, use CallTool")
}
