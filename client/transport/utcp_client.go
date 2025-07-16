// file: utcp_client.go
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"server" // import your server package
)

// UtcpClientInterface defines the public API.
type UtcpClientInterface interface {
	RegisterToolProvider(ctx context.Context, prov server.Provider) ([]server.Tool, error)
	DeregisterToolProvider(ctx context.Context, providerName string) error
	CallTool(ctx context.Context, toolName string, args map[string]any) (any, error)
	SearchTools(ctx context.Context, query string, limit int) ([]server.Tool, error)
}

// UtcpClient holds all state and implements UtcpClientInterface.
type UtcpClient struct {
	config         *UtcpClientConfig
	transports     map[string]ClientTransport
	toolRepository ToolRepository
	searchStrategy ToolSearchStrategy
}

// NewUtcpClient constructs a new client, loading providers if configured.
func NewUtcpClient(
	ctx context.Context,
	cfg *UtcpClientConfig,
	repo ToolRepository,
	strat ToolSearchStrategy,
) (*UtcpClient, error) {
	if cfg == nil {
		cfg = NewClientConfig()
	}
	if repo == nil {
		repo = NewInMemoryToolRepository() // You'll need to implement this
	}
	if strat == nil {
		strat = NewTagSearchStrategy(repo, 1.0) // You'll need to implement this
	}

	client := &UtcpClient{
		config:         cfg,
		transports:     defaultTransports(),
		toolRepository: repo,
		searchStrategy: strat,
	}

	// if providersFilePath is set, adjust TextTransport base path
	if cfg.ProvidersFilePath != "" {
		dir := filepath.Dir(cfg.ProvidersFilePath)
		if textTransport, ok := client.transports["text"]; ok {
			if tt, ok := textTransport.(TextTransport); ok {
				tt.SetBasePath(dir) // Assume this method exists
			}
		}
	}

	// eager variable substitution if inline vars present
	if len(cfg.Variables) > 0 {
		// Create a clone without variables to avoid circular references
		clone := &UtcpClientConfig{
			ProvidersFilePath: cfg.ProvidersFilePath,
			LoadVariablesFrom: cfg.LoadVariablesFrom,
			Variables:         make(map[string]string),
		}
		if substituted, ok := client.replaceVarsInAny(cfg.Variables, clone).(map[string]string); ok {
			client.config.Variables = substituted
		}
	}

	// load & register providers from JSON file
	if cfg.ProvidersFilePath != "" {
		if err := client.loadProviders(ctx, cfg.ProvidersFilePath); err != nil {
			return nil, err
		}
	}

	return client, nil
}

// defaultTransports wires up your various transport implementations.
func defaultTransports() map[string]ClientTransport {
	return map[string]ClientTransport{
		"http":        NewHTTPTransport(),           // You'll need to implement these
		"cli":         NewCliTransport(nil),         // You'll need to implement these
		"sse":         NewSSETransport(),            // You'll need to implement these
		"http_stream": NewStreamableHTTPTransport(), // You'll need to implement these
		"mcp":         NewMCPTransport(),            // You'll need to implement these
		"text":        NewTextTransport(""),         // You'll need to implement these
		"graphql": NewGraphQLTransport(func(msg string, err error) {
			fmt.Printf("GraphQL Transport: %s: %v\n", msg, err)
		}),
	}
}

// loadProviders reads a JSON array of providers, substitutes variables, and registers each.
func (c *UtcpClient) loadProviders(ctx context.Context, path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not read providers file %q: %w", path, err)
	}

	var rawList []map[string]any
	if err := json.Unmarshal(data, &rawList); err != nil {
		return fmt.Errorf("invalid JSON in providers file %q: %w", path, err)
	}

	for _, raw := range rawList {
		ptype, ok := raw["provider_type"].(string)
		if !ok || ptype == "" {
			fmt.Fprintf(os.Stderr, "warning: skipping provider without type: %v\n", raw)
			continue
		}

		// substitute inline variables first
		subbed := c.replaceVarsInAny(raw, c.config).(map[string]any)

		// decode into the right Provider struct
		var prov server.Provider
		switch ptype {
		case "http":
			prov = &server.HttpProvider{}
		case "cli":
			prov = &server.CliProvider{}
		case "sse":
			prov = &server.SSEProvider{}
		case "http_stream":
			prov = &server.StreamableHttpProvider{}
		case "websocket":
			prov = &server.WebSocketProvider{}
		case "grpc":
			prov = &server.GRPCProvider{}
		case "graphql":
			prov = &server.GraphQLProvider{}
		case "tcp":
			prov = &server.TCPProvider{}
		case "udp":
			prov = &server.UDPProvider{}
		case "webrtc":
			prov = &server.WebRTCProvider{}
		case "mcp":
			prov = &server.MCPProvider{}
		case "text":
			prov = &server.TextProvider{}
		default:
			fmt.Fprintf(os.Stderr, "warning: unsupported provider type %q, skipping\n", ptype)
			continue
		}

		blob, _ := json.Marshal(subbed)
		if err := json.Unmarshal(blob, prov); err != nil {
			fmt.Fprintf(os.Stderr, "error decoding provider %q: %v\n", ptype, err)
			continue
		}

		if _, err := c.RegisterToolProvider(ctx, prov); err != nil {
			fmt.Fprintf(os.Stderr, "error registering provider %q: %v\n", c.getProviderName(prov), err)
		}
	}

	return nil
}

// getProviderName extracts the name from a provider
func (c *UtcpClient) getProviderName(prov server.Provider) string {
	switch p := prov.(type) {
	case *server.HttpProvider:
		return p.Name
	case *server.CliProvider:
		return p.Name
	case *server.SSEProvider:
		return p.Name
	case *server.StreamableHttpProvider:
		return p.Name
	case *server.WebSocketProvider:
		return p.Name
	case *server.GRPCProvider:
		return p.Name
	case *server.GraphQLProvider:
		return p.Name
	case *server.TCPProvider:
		return p.Name
	case *server.UDPProvider:
		return p.Name
	case *server.WebRTCProvider:
		return p.Name
	case *server.MCPProvider:
		return p.Name
	case *server.TextProvider:
		return p.Name
	default:
		return "unknown"
	}
}

// setProviderName sets the name on a provider
func (c *UtcpClient) setProviderName(prov server.Provider, name string) {
	switch p := prov.(type) {
	case *server.HttpProvider:
		p.Name = name
	case *server.CliProvider:
		p.Name = name
	case *server.SSEProvider:
		p.Name = name
	case *server.StreamableHttpProvider:
		p.Name = name
	case *server.WebSocketProvider:
		p.Name = name
	case *server.GRPCProvider:
		p.Name = name
	case *server.GraphQLProvider:
		p.Name = name
	case *server.TCPProvider:
		p.Name = name
	case *server.UDPProvider:
		p.Name = name
	case *server.WebRTCProvider:
		p.Name = name
	case *server.MCPProvider:
		p.Name = name
	case *server.TextProvider:
		p.Name = name
	}
}

// RegisterToolProvider applies variable substitution, picks the right transport, and registers tools.
func (c *UtcpClient) RegisterToolProvider(
	ctx context.Context,
	prov server.Provider,
) ([]server.Tool, error) {
	prov = c.substituteProviderVariables(prov)
	name := strings.ReplaceAll(c.getProviderName(prov), ".", "_")
	c.setProviderName(prov, name)

	tr, ok := c.transports[string(prov.Type())]
	if !ok {
		return nil, fmt.Errorf("unsupported provider type: %s", prov.Type())
	}

	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		return nil, err
	}

	// Prefix tool names with provider name if not already prefixed
	for i := range tools {
		if !strings.HasPrefix(tools[i].Name, name+".") {
			tools[i].Name = name + "." + tools[i].Name
		}
	}

	if err := c.toolRepository.SaveProviderWithTools(ctx, prov, tools); err != nil {
		return nil, err
	}
	return tools, nil
}

func (c *UtcpClient) DeregisterToolProvider(ctx context.Context, providerName string) error {
	prov, err := c.toolRepository.GetProvider(ctx, providerName)
	if err != nil {
		return err
	}
	if prov == nil {
		return fmt.Errorf("provider not found: %s", providerName)
	}

	tr, ok := c.transports[string((*prov).Type())]
	if !ok {
		return fmt.Errorf("no transport for provider type %s", (*prov).Type())
	}
	if err := tr.DeregisterToolProvider(ctx, *prov); err != nil {
		return err
	}
	return c.toolRepository.RemoveProvider(ctx, providerName)
}

func (c *UtcpClient) CallTool(
	ctx context.Context,
	toolName string,
	args map[string]any,
) (any, error) {
	parts := strings.SplitN(toolName, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid tool name: %s", toolName)
	}
	providerName := parts[0]

	prov, err := c.toolRepository.GetProvider(ctx, providerName)
	if err != nil {
		return nil, err
	}
	if prov == nil {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	tools, err := c.toolRepository.GetToolsByProvider(ctx, providerName)
	if err != nil {
		return nil, err
	}
	var selectedTool *server.Tool
	for _, t := range tools {
		if t.Name == toolName {
			selectedTool = &t
			break
		}
	}
	if selectedTool == nil {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	// re‑substitute any provider vars before call
	*prov = c.substituteProviderVariables(*prov)

	tr, ok := c.transports[string((*prov).Type())]
	if !ok {
		return nil, fmt.Errorf("no transport for provider type %s", (*prov).Type())
	}

	return tr.CallTool(ctx, toolName, args, *prov)
}

func (c *UtcpClient) SearchTools(query string, limit int) ([]server.Tool, error) {
	tools, err := c.searchStrategy.SearchTools(context.Background(), query, limit)
	if err != nil {
		return nil, err
	}

	// Convert []*server.Tool to []server.Tool
	result := make([]server.Tool, len(tools))
	for i, tool := range tools {
		result[i] = tool
	}
	return result, nil
}

// ----- variable substitution helpers -----

// substituteProviderVariables dumps to JSON, replaces vars, and re‑unmarshals.
func (c *UtcpClient) substituteProviderVariables(p server.Provider) server.Provider {
	// Convert provider to map for substitution
	raw := c.providerToMap(p)
	out := c.replaceVarsInAny(raw, c.config).(map[string]any)

	// Create new provider of the same type
	newProv := c.createProviderOfType(p.Type())

	// Marshal and unmarshal to populate the new provider
	blob, _ := json.Marshal(out)
	_ = json.Unmarshal(blob, newProv)
	return newProv
}

// providerToMap converts a provider to a map for JSON manipulation
func (c *UtcpClient) providerToMap(p server.Provider) map[string]any {
	blob, _ := json.Marshal(p)
	var result map[string]any
	json.Unmarshal(blob, &result)
	return result
}

// createProviderOfType creates a new provider instance of the given type
func (c *UtcpClient) createProviderOfType(ptype server.ProviderType) server.Provider {
	switch ptype {
	case server.ProviderHTTP:
		return &server.HttpProvider{}
	case server.ProviderCLI:
		return &server.CliProvider{}
	case server.ProviderSSE:
		return &server.SSEProvider{}
	case server.ProviderHTTPStream:
		return &server.StreamableHttpProvider{}
	case server.ProviderWebSocket:
		return &server.WebSocketProvider{}
	case server.ProviderGRPC:
		return &server.GRPCProvider{}
	case server.ProviderGraphQL:
		return &server.GraphQLProvider{}
	case server.ProviderTCP:
		return &server.TCPProvider{}
	case server.ProviderUDP:
		return &server.UDPProvider{}
	case server.ProviderWebRTC:
		return &server.WebRTCProvider{}
	case server.ProviderMCP:
		return &server.MCPProvider{}
	case server.ProviderText:
		return &server.TextProvider{}
	default:
		return &server.HttpProvider{} // fallback
	}
}

// replaceVarsInAny walks strings, maps, lists and does ${VAR}/$VAR substitution.
func (c *UtcpClient) replaceVarsInAny(x any, cfg *UtcpClientConfig) any {
	switch v := x.(type) {
	case string:
		re := regexp.MustCompile(`\${(\w+)}|\$(\w+)`)
		return re.ReplaceAllStringFunc(v, func(match string) string {
			g := re.FindStringSubmatch(match)
			name := g[1]
			if name == "" {
				name = g[2]
			}
			val, err := c.getVariable(name, cfg)
			if err != nil {
				// Return the original match if variable not found
				return match
			}
			return val
		})
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = c.replaceVarsInAny(e, cfg)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = c.replaceVarsInAny(e, cfg)
		}
		return out
	default:
		return x
	}
}

// getVariable checks inline, loaders, then os.Getenv.
func (c *UtcpClient) getVariable(key string, cfg *UtcpClientConfig) (string, error) {
	if v, ok := cfg.Variables[key]; ok {
		return v, nil
	}
	for _, loader := range cfg.LoadVariablesFrom {
		if val, err := loader.Get(key); err == nil && val != "" {
			return val, nil
		}
	}
	if env := os.Getenv(key); env != "" {
		return env, nil
	}
	return "", &UtcpVariableNotFound{VariableName: key}
}

// Helper functions that you'll need to implement:

// NewInMemoryToolRepository creates an in-memory tool repository
func NewInMemoryToolRepository() ToolRepository {
	// You'll need to implement this based on your ToolRepository interface
	panic("NewInMemoryToolRepository not implemented")
}

// Transport constructors - you'll need to implement these
func NewHTTPTransport() ClientTransport {
	panic("NewHTTPTransport not implemented")
}

func NewCLITransport() ClientTransport {
	panic("NewCLITransport not implemented")
}

func NewSSETransport() ClientTransport {
	panic("NewSSETransport not implemented")
}

func NewStreamableHTTPTransport() ClientTransport {
	panic("NewStreamableHTTPTransport not implemented")
}

func NewMCPTransport() ClientTransport {
	panic("NewMCPTransport not implemented")
}

func NewTextTransport(basePath string) ClientTransport {
	panic("NewTextTransport not implemented")
}

// TextTransport interface for setting base path
type TextTransport interface {
	ClientTransport
	SetBasePath(path string)
}
