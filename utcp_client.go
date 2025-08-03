package utcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/helpers"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tag"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/sse"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/cli"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/graphql"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/grpc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/http"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/mcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/streamable"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/tcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/udp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/webrtc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/websocket"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/cli"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/graphql"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/sse"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/streamable"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/tcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/udp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/webrtc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/websocket"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// UtcpClientInterface defines the public API.
type UtcpClientInterface interface {
	RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error)
	DeregisterToolProvider(ctx context.Context, providerName string) error
	CallTool(ctx context.Context, toolName string, args map[string]any) (any, error)
	SearchTools(query string, limit int) ([]Tool, error)
	GetTransports() map[string]ClientTransport
	CallToolStream(ctx context.Context, toolName string, args map[string]any) (transports.StreamResult, error)
}
type resolvedTool struct {
	provider  Provider
	transport ClientTransport
	callName  string
	tool      *Tool
}

// UtcpClient holds all state and implements UtcpClientInterface.
type UtcpClient struct {
	config         *UtcpClientConfig
	transports     map[string]ClientTransport
	toolRepository ToolRepository
	searchStrategy ToolSearchStrategy

	// caches
	providerToolsCache    map[string][]Tool
	providerToolsCacheMu  sync.RWMutex
	toolResolutionCache   map[string]*resolvedTool // key is full tool name
	toolResolutionCacheMu sync.RWMutex
}

// NewUtcpClient constructs a new client, loading providers if configured.
func NewUTCPClient(
	ctx context.Context,
	cfg *UtcpClientConfig,
	repo ToolRepository,
	strat ToolSearchStrategy,
) (UtcpClientInterface, error) {
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
		config:              cfg,
		transports:          defaultTransports(),
		toolRepository:      repo,
		searchStrategy:      strat,
		providerToolsCache:  make(map[string][]Tool),
		toolResolutionCache: make(map[string]*resolvedTool),
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

			},
		),
		"udp": NewUDPTransport(
			func(format string, args ...interface{}) {
				fmt.Printf("UDP Transport: "+format+"\n", args...)
			},
		),
		"grpc": NewGRPCClientTransport(func(format string, args ...interface{}) {
			fmt.Printf("gRPC Transport: "+format+"\n", args...)
		}),
		"graphql": NewGraphQLClientTransport(func(msg string, err error) {
			fmt.Printf("GraphQL Transport: %s: %v\n", msg, err)
		}),
		"webrtc": NewWebRTCClientTransport(func(format string, args ...interface{}) {
			fmt.Printf("WebRTC Transport: "+format+"\n", args...)
		}),
	}
}

// Updated loadProviders method using the new parser
func (c *UtcpClient) loadProviders(ctx context.Context, path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not read providers file %q: %w", path, err)
	}
	rawList, err := parseProvidersJSON(data)
	if err != nil {
		return fmt.Errorf("error parsing providers JSON: %w", err)
	}

	var errors []string
	successCount := 0

	for i, raw := range rawList {
		if err := c.processProvider(ctx, raw, i); err != nil {
			errors = append(errors, fmt.Sprintf("provider %d: %v", i, err))
			continue
		}
		successCount++
	}
	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: %d providers failed to load:\n", len(errors))
		for _, errMsg := range errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", errMsg)
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
		p.Name = name
	}
}

// RegisterToolProvider applies variable substitution, picks the right transport, and registers tools.
func (c *UtcpClient) RegisterToolProvider(
	ctx context.Context,
	prov Provider,
) ([]Tool, error) {
	c.ensureCaches() // defensive
	prov = c.substituteProviderVariables(prov)
	name := strings.ReplaceAll(c.getProviderName(prov), ".", "_")
	c.setProviderName(prov, name)

	// Check cache: if already registered, return cached tools and ensure resolution cache is primed
	c.providerToolsCacheMu.RLock()
	if tools, ok := c.providerToolsCache[name]; ok {
		c.providerToolsCacheMu.RUnlock()

		// Prime resolution cache for any missing entries
		tr, _ := c.transports[string(prov.Type())]
		c.toolResolutionCacheMu.Lock()
		for i := range tools {
			if _, exists := c.toolResolutionCache[tools[i].Name]; !exists {
				callName := tools[i].Name
				if prov.Type() == ProviderMCP {
					if _, suffix, ok := strings.Cut(tools[i].Name, "."); ok {
						callName = suffix
					}
				}
				c.toolResolutionCache[tools[i].Name] = &resolvedTool{
					provider:  prov,
					transport: tr,
					callName:  callName,
					tool:      &tools[i],
				}
			}
		}
		c.toolResolutionCacheMu.Unlock()

		return tools, nil
	}
	c.providerToolsCacheMu.RUnlock()

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

	// Populate cache
	c.providerToolsCacheMu.Lock()
	c.providerToolsCache[name] = tools
	c.providerToolsCacheMu.Unlock()

	// Also prime resolution cache for each tool
	c.toolResolutionCacheMu.Lock()
	for i := range tools {
		callName := tools[i].Name
		if prov.Type() == ProviderMCP {
			if _, suffix, ok := strings.Cut(tools[i].Name, "."); ok {
				callName = suffix
			}
		}
		c.toolResolutionCache[tools[i].Name] = &resolvedTool{
			provider:  prov,
			transport: tr,
			callName:  callName,
			tool:      &tools[i],
		}
	}
	c.toolResolutionCacheMu.Unlock()

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
	if err := c.toolRepository.RemoveProvider(ctx, providerName); err != nil {
		return err
	}

	// Invalidate providerToolsCache
	c.providerToolsCacheMu.Lock()
	delete(c.providerToolsCache, providerName)
	c.providerToolsCacheMu.Unlock()

	// Invalidate resolution cache entries for tools of this provider
	c.toolResolutionCacheMu.Lock()
	for k, res := range c.toolResolutionCache {
		// Assuming tool.Name has prefix providerName.
		if strings.HasPrefix(k, providerName+".") {
			delete(c.toolResolutionCache, k)
		} else if res.provider != nil {
			// Fallback check: if provider name matches
			if c.getProviderName(res.provider) == providerName {
				delete(c.toolResolutionCache, k)
			}
		}
	}
	c.toolResolutionCacheMu.Unlock()

	return nil
}

func (c *UtcpClient) CallTool(
	ctx context.Context,
	toolName string,
	args map[string]any,
) (any, error) {
	// Fast‑path: check the resolution cache first to avoid extra work
	c.toolResolutionCacheMu.RLock()
	if res, ok := c.toolResolutionCache[toolName]; ok {
		prov := res.provider
		tr := res.transport
		callName := res.callName
		c.toolResolutionCacheMu.RUnlock()
		return tr.CallTool(ctx, callName, args, prov, nil)
	}
	c.toolResolutionCacheMu.RUnlock()

	// Cache miss – resolve via repository and update the cache
	prov, tr, callName, _, err := c.resolveTool(ctx, toolName)
	if err != nil {
		return nil, err
	}
	return tr.CallTool(ctx, callName, args, prov, nil)
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

// ----- variable substitution src -----

// substituteProviderVariables dumps to JSON, replaces vars, and re‑unmarshals.
func (c *UtcpClient) substituteProviderVariables(p Provider) Provider {
	// Convert provider to map for substitution
	raw := c.providerToMap(p)
	out := c.replaceVarsInAny(raw, c.config).(map[string]any)

	// Create new provider of the same type
	newProv := c.createProviderOfType(p.Type())

	// Marshal and unmarshal to populate the new provider
	if blob, err := json.Marshal(out); err == nil {
		_ = json.Unmarshal(blob, newProv)
	}
	return newProv
}

// cloneProvider deep-copies a provider without variable substitution.
func (c *UtcpClient) cloneProvider(p Provider) Provider {
	raw := c.providerToMap(p)
	newProv := c.createProviderOfType(p.Type())
	if blob, err := json.Marshal(raw); err == nil {
		_ = json.Unmarshal(blob, newProv)
	}
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

func parseProvidersJSON(data []byte) ([]map[string]any, error) {
	var rawData interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	switch v := rawData.(type) {
	case []interface{}:
		// Direct array of providers
		return convertInterfaceArrayToMapArray(v)

	case map[string]interface{}:
		// Object that might contain providers
		if providersRaw, exists := v["providers"]; exists {
			switch providers := providersRaw.(type) {
			case []interface{}:
				// providers is an array
				return convertInterfaceArrayToMapArray(providers)
			case map[string]interface{}:
				// providers is a single object
				converted := convertInterfaceMapToStringMap(providers)
				return []map[string]any{converted}, nil
			default:
				return nil, fmt.Errorf("'providers' field must be an array or object, got %T", providersRaw)
			}
		}
		// Single provider object (no "providers" wrapper)
		converted := convertInterfaceMapToStringMap(v)
		return []map[string]any{converted}, nil

	default:
		return nil, fmt.Errorf("JSON root must be array or object, got %T", rawData)
	}
}

func convertInterfaceArrayToMapArray(items []interface{}) ([]map[string]any, error) {
	result := make([]map[string]any, len(items))

	for i, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("item at index %d is not an object, got %T", i, item)
		}
		result[i] = convertInterfaceMapToStringMap(itemMap)
	}

	return result, nil
}

// convertInterfaceMapToStringMap converts map[string]interface{} to map[string]any
func convertInterfaceMapToStringMap(input map[string]interface{}) map[string]any {
	result := make(map[string]any, len(input))
	for k, v := range input {
		result[k] = v
	}
	return result
}

// processProvider handles individual provider processing
func (c *UtcpClient) processProvider(ctx context.Context, raw map[string]any, index int) error {
	ptype, ok := raw["provider_type"].(string)
	if !ok || ptype == "" {
		return fmt.Errorf("missing or invalid provider_type")
	}
	// Substitute inline variables
	subbed := c.replaceVarsInAny(raw, c.config).(map[string]any)

	blob, err := json.Marshal(subbed)
	if err != nil {
		return fmt.Errorf("error marshaling provider: %w", err)
	}

	prov, err := UnmarshalProvider(blob)
	if err != nil {
		return fmt.Errorf("error decoding provider %q: %w", ptype, err)
	}

	// Look up the name; if it's empty, default to "<providerType>_<index>"
	providerName := c.getProviderName(prov)
	if providerName == "" {
		providerName = fmt.Sprintf("%s_%d", ptype, index)
		c.setProviderName(prov, providerName)
	} else {
		// sanitize dots
		providerName = strings.ReplaceAll(providerName, ".", "_")
		c.setProviderName(prov, providerName)
	}

	// Now register
	tools, err := c.RegisterToolProvider(ctx, prov)
	if err != nil {
		return fmt.Errorf("error registering provider %q: %w", providerName, err)
	}
	fmt.Printf("Successfully registered provider %s (%d tools)\n", providerName, len(tools))
	return nil
}

func (u *UtcpClient) GetTransports() map[string]ClientTransport {
	return u.transports
}

func (c *UtcpClient) CallToolStream(
	ctx context.Context,
	toolName string,
	args map[string]any,
) (transports.StreamResult, error) {
	// Fast‑path: attempt to use the cached resolution
	c.toolResolutionCacheMu.RLock()
	if res, ok := c.toolResolutionCache[toolName]; ok {
		prov := res.provider
		tr := res.transport
		callName := res.callName
		c.toolResolutionCacheMu.RUnlock()
		return tr.CallToolStream(ctx, callName, args, prov)
	}
	c.toolResolutionCacheMu.RUnlock()

	// Cache miss – resolve and then perform the call
	prov, tr, callName, _, err := c.resolveTool(ctx, toolName)
	if err != nil {
		return nil, err
	}
	return tr.CallToolStream(ctx, callName, args, prov)
}

// helper to resolve provider, transport, callName, and tool
func (c *UtcpClient) resolveTool(ctx context.Context, toolName string) (Provider, ClientTransport, string, *Tool, error) {
	// Cache lookup
	c.toolResolutionCacheMu.RLock()
	if res, ok := c.toolResolutionCache[toolName]; ok {
		prov := res.provider
		tr := res.transport
		c.toolResolutionCacheMu.RUnlock()
		return prov, tr, res.callName, res.tool, nil
	}
	c.toolResolutionCacheMu.RUnlock()

	providerName, suffix, ok := strings.Cut(toolName, ".")
	if !ok {
		return nil, nil, "", nil, fmt.Errorf("invalid tool name: %s", toolName)
	}

	prov, err := c.toolRepository.GetProvider(ctx, providerName)
	if err != nil {
		return nil, nil, "", nil, err
	}
	if prov == nil {
		return nil, nil, "", nil, fmt.Errorf("provider not found: %s", providerName)
	}

	tools, err := c.toolRepository.GetToolsByProvider(ctx, providerName)
	if err != nil {
		return nil, nil, "", nil, err
	}

	var selectedTool *Tool
	for i := range tools {
		if tools[i].Name == toolName {
			selectedTool = &tools[i]
			break
		}
	}
	if selectedTool == nil {
		return nil, nil, "", nil, fmt.Errorf("tool not found: %s", toolName)
	}

	// clone provider to avoid mutating repository entry
	cloned := c.cloneProvider(*prov)

	tr, ok := c.transports[string(cloned.Type())]
	if !ok {
		return nil, nil, "", nil, fmt.Errorf("no transport for provider type %s", cloned.Type())
	}

	callName := toolName
	if cloned.Type() == ProviderMCP {
		// Strip provider prefix for MCP transport
		callName = suffix
	}

	// Cache the resolution (provider already has variables substituted)
	c.toolResolutionCacheMu.Lock()
	c.toolResolutionCache[toolName] = &resolvedTool{
		provider:  cloned,
		transport: tr,
		callName:  callName,
		tool:      selectedTool,
	}
	c.toolResolutionCacheMu.Unlock()

	return cloned, tr, callName, selectedTool, nil
}

func (c *UtcpClient) ensureCaches() {
	if c.providerToolsCache == nil {
		c.providerToolsCache = make(map[string][]Tool)
	}
	if c.toolResolutionCache == nil {
		c.toolResolutionCache = make(map[string]*resolvedTool)
	}
}
