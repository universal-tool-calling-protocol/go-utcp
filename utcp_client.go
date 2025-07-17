package UTCP

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	// import your server package
)

// UtcpClientInterface defines the public API.
type UtcpClientInterface interface {
	RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error)
	DeregisterToolProvider(ctx context.Context, providerName string) error
	CallTool(ctx context.Context, toolName string, args map[string]any) (any, error)
	SearchTools(ctx context.Context, query string, limit int) ([]Tool, error)
}

// UtcpClient holds all state and implements UtcpClientInterface.
type UtcpClient struct {
	config         *UtcpClientConfig
	transports     map[string]ClientTransport
	toolRepository ToolRepository
	searchStrategy ToolSearchStrategy
}

// NewUtcpClient constructs a new client, loading providers if configured.
func NewUTCPClient(
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
		"http": NewHttpClientTransport(
			func(format string, args ...interface{}) {
				fmt.Printf("HTTP Transport: "+format+"\n", args...)
			},
		), // You'll need to implement these
		"cli": NewCliTransport(
			func(format string, args ...interface{}) {
				fmt.Printf("CLI Transport: "+format+"\n", args...)
			},
		), // You'll need to implement these
		// You'll need to implement these
		"sse": NewSSETransport(func(format string, args ...interface{}) {
			fmt.Printf("SSE Transport: "+format+"\n", args...)
		}), // You'll need to implement these
		"http_stream": NewStreamableHTTPTransport(func(format string, args ...interface{}) {
			fmt.Printf("HTTP Stream Transport: "+format+"\n", args...)
		}), // You'll need to implement these
		"mcp": NewMCPTransport(
			func(format string, args ...interface{}) {
				fmt.Printf("MCP Transport: "+format+"\n", args...)
			},
		), // You'll need to implement these
		"websocket": NewWebSocketTransport(func(format string, args ...interface{}) {
			fmt.Printf("WebSocket Transport: "+format+"\n", args...)
		}),
		"tcp": NewTCPClientTransport(
			func(format string, args ...interface{}) {
				fmt.Printf("TCP Transport: "+format+"\n", args...)
		"udp": NewUDPTransport(
			func(format string, args ...interface{}) {
				fmt.Printf("UDP Transport: "+format+"\n", args...)
			},
		),
		"text": NewTextTransport(""), // You'll need to implement these
		"graphql": NewGraphQLClientTransport(func(msg string, err error) {
			fmt.Printf("GraphQL Transport: %s: %v\n", msg, err)
		}),
		"webrtc": NewWebRTCClientTransport(func(format string, args ...interface{}) {
			fmt.Printf("WebRTC Transport: "+format+"\n", args...)
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

		blob, _ := json.Marshal(subbed)
		prov, err := UnmarshalProvider(blob)
		if err != nil {
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
func (c *UtcpClient) getProviderName(prov Provider) string {
	switch p := prov.(type) {
	case *HttpProvider:
		return p.Name
	case *CliProvider:
		return p.Name
	case *SSEProvider:
		return p.Name
	case *StreamableHttpProvider:
		return p.Name
	case *WebSocketProvider:
		return p.Name
	case *GRPCProvider:
		return p.Name
	case *GraphQLProvider:
		return p.Name
	case *TCPProvider:
		return p.Name
	case *UDPProvider:
		return p.Name
	case *WebRTCProvider:
		return p.Name
	case *MCPProvider:
		return p.Name()
	case *TextProvider:
		return p.Name
	default:
		return "unknown"
	}
}

// setProviderName sets the name on a provider
func (c *UtcpClient) setProviderName(prov Provider, name string) {
	switch p := prov.(type) {
	case *HttpProvider:
		p.Name = name
	case *CliProvider:
		p.Name = name
	case *SSEProvider:
		p.Name = name
	case *StreamableHttpProvider:
		p.Name = name
	case *WebSocketProvider:
		p.Name = name
	case *GRPCProvider:
		p.Name = name
	case *GraphQLProvider:
		p.Name = name
	case *TCPProvider:
		p.Name = name
	case *UDPProvider:
		p.Name = name
	case *WebRTCProvider:
		p.Name = name
	case *MCPProvider:
		p.name = name
	case *TextProvider:
		p.Name = name
	}
}

// RegisterToolProvider applies variable substitution, picks the right transport, and registers tools.
func (c *UtcpClient) RegisterToolProvider(
	ctx context.Context,
	prov Provider,
) ([]Tool, error) {
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
	var selectedTool *Tool
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

	return tr.CallTool(ctx, toolName, args, *prov, nil)
}

func (c *UtcpClient) SearchTools(query string, limit int) ([]Tool, error) {
	tools, err := c.searchStrategy.SearchTools(context.Background(), query, limit)
	if err != nil {
		return nil, err
	}

	// Convert []*Tool to []Tool
	result := make([]Tool, len(tools))
	for i, tool := range tools {
		result[i] = tool
	}
	return result, nil
}

// ----- variable substitution helpers -----

// substituteProviderVariables dumps to JSON, replaces vars, and re‑unmarshals.
func (c *UtcpClient) substituteProviderVariables(p Provider) Provider {
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
func (c *UtcpClient) providerToMap(p Provider) map[string]any {
	blob, _ := json.Marshal(p)
	var result map[string]any
	json.Unmarshal(blob, &result)
	return result
}

// createProviderOfType creates a new provider instance of the given type
func (c *UtcpClient) createProviderOfType(ptype ProviderType) Provider {
	switch ptype {
	case ProviderHTTP:
		return &HttpProvider{}
	case ProviderCLI:
		return &CliProvider{}
	case ProviderSSE:
		return &SSEProvider{}
	case ProviderHTTPStream:
		return &StreamableHttpProvider{}
	case ProviderWebSocket:
		return &WebSocketProvider{}
	case ProviderGRPC:
		return &GRPCProvider{}
	case ProviderGraphQL:
		return &GraphQLProvider{}
	case ProviderTCP:
		return &TCPProvider{}
	case ProviderUDP:
		return &UDPProvider{}
	case ProviderWebRTC:
		return &WebRTCProvider{}
	case ProviderMCP:
		return &MCPProvider{}
	case ProviderText:
		return &TextProvider{}
	default:
		return &HttpProvider{} // fallback
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
