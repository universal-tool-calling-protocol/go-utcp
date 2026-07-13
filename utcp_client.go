package utcp

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	jsoniter "github.com/json-iterator/go"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/helpers"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tag"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/cli"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/graphql"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/grpc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/http"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/mcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/sse"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/streamable"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/tcp"
	texttransport "github.com/universal-tool-calling-protocol/go-utcp/src/transports/text"
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
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/text"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/udp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/webrtc"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/websocket"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// NOTE: jsoniter is already used project-wide
var json = jsoniter.ConfigFastest

// Precompiled var substitution regex (avoids recompiling per call)
var varRe = regexp.MustCompile(`\${(\w+)}|\$(\w+)`)

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
}

type clientCache struct {
	tools         map[string]*resolvedTool
	providerTools map[string][]Tool
}

// UtcpClient holds all state and implements UtcpClientInterface.
type UtcpClient struct {
	config         *UtcpClientConfig
	transports     map[string]ClientTransport
	toolRepository ToolRepository
	searchStrategy ToolSearchStrategy

	cacheMu  sync.Mutex
	cache    atomic.Pointer[clientCache]
	fallback sync.Map
}

// NewUTCPClient constructs a new client, loading providers if configured.
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
		repo = NewInMemoryToolRepository()
	}
	if strat == nil {
		strat = NewTagSearchStrategy(repo, 1.0)
	}

	client := &UtcpClient{
		config:         cfg,
		transports:     defaultTransports(),
		toolRepository: repo,
		searchStrategy: strat,
	}
	client.cache.Store(newClientCache())

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
		"http":        NewHttpClientTransport(nil),
		"cli":         NewCliTransport(nil),
		"sse":         NewSSETransport(nil),
		"http_stream": NewStreamableHTTPTransport(nil),
		"mcp":         NewMCPTransport(nil),
		"websocket":   NewWebSocketTransport(nil),
		"tcp":         NewTCPClientTransport(nil),
		"udp":         NewUDPTransport(nil),
		"grpc":        NewGRPCClientTransport(nil),
		"graphql":     NewGraphQLClientTransport(nil),
		"webrtc":      NewWebRTCClientTransport(nil),
		"text":        texttransport.NewTextTransport(nil),
	}
}

func (c *UtcpClient) loadProviders(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not read providers file %q: %w", path, err)
	}
	rawList, err := parseProvidersJSON(data)
	if err != nil {
		return fmt.Errorf("error parsing providers JSON: %w", err)
	}

	var errors []string

	for i, raw := range rawList {
		if err := c.processProvider(ctx, raw, i); err != nil {
			errors = append(errors, fmt.Sprintf("provider %d: %v", i, err))
			continue
		}
	}
	if len(errors) > 0 {
		for _, errMsg := range errors {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", errMsg)
		}
	}

	return nil
}

// getProviderName extracts the name from a provider
func (c *UtcpClient) getProviderName(prov Provider) string {
	if name, ok := ProviderName(prov); ok {
		return name
	}
	return "unknown"
}

// setProviderName sets the name on a provider
func (c *UtcpClient) setProviderName(prov Provider, name string) {
	SetProviderName(prov, name)
}

// RegisterToolProvider picks the transport and registers tools.
// NOTE: do NOT call substituteProviderVariables here; processProvider already did it.
func (c *UtcpClient) RegisterToolProvider(
	ctx context.Context,
	prov Provider,
) ([]Tool, error) {
	// Derive/sanitize provider name with a safe fallback (prevents ".tool" cases).
	name := strings.ReplaceAll(c.getProviderName(prov), ".", "_")
	if name == "" || name == "unknown" {
		name = strings.ToLower(string(prov.Type()))
		if name == "" {
			name = "provider"
		}
	}
	c.setProviderName(prov, name)

	if tools, ok := c.cachedProviderTools(name); ok {
		return tools, nil
	}

	// Look up transport
	tr, ok := c.transports[string(prov.Type())]
	if !ok || tr == nil {
		return nil, fmt.Errorf("unsupported provider type: %s", prov.Type())
	}

	// For gRPC providers, ensure sane endpoint defaults (avoid dialing :0).
	if gp, ok := prov.(*GRPCProvider); ok {
		if gp.Host == "" {
			gp.Host = "127.0.0.1"
		}
		if gp.Port == 0 {
			gp.Port = 9339
		}
	}

	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		return nil, err
	}

	// Normalize tool names so they are always qualified with this provider and never start with "."
	for i := range tools {
		n := strings.TrimLeft(tools[i].Name, ".")
		if dot := strings.Index(n, "."); dot >= 0 {
			prefix, suffix := n[:dot], n[dot+1:]
			if prefix != name {
				n = name + "." + suffix
			}
			tools[i].Name = n
		} else {
			tools[i].Name = name + "." + n
		}
	}

	// Persist provider + tools
	if err := c.toolRepository.SaveProviderWithTools(ctx, prov, tools); err != nil {
		return nil, err
	}

	c.publishProvider(name, prov, tr, tools)

	return append([]Tool(nil), tools...), nil
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

	c.removeCachedProvider(providerName)

	return nil
}

func (c *UtcpClient) CallTool(
	ctx context.Context,
	toolName string,
	args map[string]any,
) (any, error) {
	resolved, err := c.resolveTool(ctx, toolName)
	if err != nil {
		return nil, err
	}
	return resolved.transport.CallTool(ctx, resolved.callName, args, resolved.provider, nil)
}

func (c *UtcpClient) SearchTools(query string, limit int) ([]Tool, error) {
	ctx := context.Background()
	if query == "" {
		all, err := c.toolRepository.GetTools(ctx)
		return limitTools(all, limit), err
	}

	if provider, err := c.toolRepository.GetProvider(ctx, query); err != nil {
		return nil, err
	} else if provider != nil {
		providerTools, err := c.toolRepository.GetToolsByProvider(ctx, query)
		return limitTools(providerTools, limit), err
	}

	if c.searchStrategy != nil {
		return c.searchStrategy.SearchTools(ctx, query, limit)
	}
	return nil, nil
}

func limitTools(all []Tool, limit int) []Tool {
	if limit <= 0 || len(all) <= limit {
		return all
	}
	return all[:limit]
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
	switch v := p.(type) {
	case *HttpProvider:
		cp := *v
		return &cp
	case *CliProvider:
		cp := *v
		return &cp
	case *SSEProvider:
		cp := *v
		return &cp
	case *StreamableHttpProvider:
		cp := *v
		return &cp
	case *WebSocketProvider:
		cp := *v
		return &cp
	case *GRPCProvider:
		cp := *v
		return &cp
	case *GraphQLProvider:
		cp := *v
		return &cp
	case *TCPProvider:
		cp := *v
		return &cp
	case *UDPProvider:
		cp := *v
		return &cp
	case *WebRTCProvider:
		cp := *v
		return &cp
	case *MCPProvider:
		cp := *v
		return &cp
	case *TextProvider:
		cp := *v
		return &cp
	default:
		// Worst case, skip cloning; treat providers as read-only.
		return p
	}
}

// providerToMap converts a provider to a map for JSON manipulation
func (c *UtcpClient) providerToMap(p Provider) map[string]any {
	blob, _ := json.Marshal(p)
	var result map[string]any
	_ = json.Unmarshal(blob, &result)
	return result
}

// createProviderOfType creates a new provider instance of the given type
func (c *UtcpClient) createProviderOfType(ptype ProviderType) Provider {
	provider, err := NewProvider(ptype)
	if err != nil {
		return &HttpProvider{}
	}
	return provider
}

// replaceVarsInAny walks strings, maps, lists and does ${VAR}/$VAR substitution.
func (c *UtcpClient) replaceVarsInAny(x any, cfg *UtcpClientConfig) any {
	switch v := x.(type) {
	case string:
		return varRe.ReplaceAllStringFunc(v, func(match string) string {
			name := match[1:]
			if len(name) >= 2 && name[0] == '{' {
				name = name[1 : len(name)-1]
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
				return []map[string]any{providers}, nil
			default:
				return nil, fmt.Errorf("'providers' field must be an array or object, got %T", providersRaw)
			}
		}
		// Single provider object (no "providers" wrapper)
		return []map[string]any{v}, nil

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
		result[i] = itemMap
	}

	return result, nil
}

// processProvider handles individual provider processing
// processProvider handles individual provider processing (normalizes keys and auth,
// tolerates both "type" and "provider_type", and avoids :0 by defaulting host/port).
func (c *UtcpClient) processProvider(ctx context.Context, raw map[string]any, index int) error {
	// Copy for normalization
	sub := make(map[string]any, len(raw))
	for k, v := range raw {
		sub[k] = v
	}

	// Accept both "type" and "provider_type" (ensure "type" is set for UnmarshalProvider)
	var ptype string
	if t, ok := sub["type"].(string); ok && t != "" {
		ptype = t
	} else if t, ok := sub["provider_type"].(string); ok && t != "" {
		ptype = t
		sub["type"] = t
	} else {
		return fmt.Errorf("missing or invalid provider_type/type")
	}

	// Normalize auth: accept "auth.type" as alias for "auth.auth_type"
	if a, ok := sub["auth"]; ok && a != nil {
		if amap, ok := a.(map[string]any); ok {
			if _, have := amap["auth_type"]; !have {
				if v, ok := amap["type"]; ok {
					amap["auth_type"] = v
				}
			}
			sub["auth"] = amap
		}
	}

	// Sensible defaults for endpoints (avoid dialing :0 for gRPC providers)
	if ptype == "grpc" {
		if _, ok := sub["host"]; !ok {
			sub["host"] = "127.0.0.1"
		}
		if _, ok := sub["port"]; !ok {
			sub["port"] = 9339
		}
	}

	// Variable substitution after normalization
	subbed := c.replaceVarsInAny(sub, c.config).(map[string]any)
	subbed["type"] = ptype // keep consistent

	blob, err := json.Marshal(subbed)
	if err != nil {
		return fmt.Errorf("error marshaling provider: %w", err)
	}

	prov, err := UnmarshalProvider(blob)
	if err != nil {
		return fmt.Errorf("error decoding provider %q: %w", ptype, err)
	}

	// Name fallback/sanitization
	providerName := c.getProviderName(prov)
	if providerName == "" {
		providerName = fmt.Sprintf("%s_%d", ptype, index)
	}
	providerName = strings.ReplaceAll(providerName, ".", "_")
	c.setProviderName(prov, providerName)

	// Register
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
	resolved, err := c.resolveTool(ctx, toolName)
	if err != nil {
		return nil, err
	}
	return resolved.transport.CallToolStream(ctx, resolved.callName, args, resolved.provider)
}

func (c *UtcpClient) resolveTool(ctx context.Context, toolName string) (*resolvedTool, error) {
	if resolved, ok := c.cachedTool(toolName); ok {
		return resolved, nil
	}

	providerName, suffix, ok := strings.Cut(toolName, ".")
	if !ok {
		return nil, fmt.Errorf("invalid tool name: %s", toolName)
	}

	prov, err := c.toolRepository.GetProvider(ctx, providerName)
	if err != nil {
		return nil, err
	}
	if prov == nil {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	tool, err := c.toolRepository.GetTool(ctx, toolName)
	if err != nil {
		return nil, err
	}
	if tool == nil {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	cloned := c.cloneProvider(*prov)
	tr, ok := c.transports[string(cloned.Type())]
	if !ok || tr == nil {
		return nil, fmt.Errorf("no transport for provider type %s", cloned.Type())
	}

	callName := toolName
	if cloned.Type() == ProviderMCP || cloned.Type() == ProviderText {
		callName = suffix
	}

	resolved := &resolvedTool{provider: cloned, transport: tr, callName: callName}
	c.publishResolved(toolName, resolved)
	return resolved, nil
}

func newClientCache() *clientCache {
	return &clientCache{
		tools:         make(map[string]*resolvedTool),
		providerTools: make(map[string][]Tool),
	}
}

func (c *UtcpClient) cacheSnapshot() *clientCache {
	if snapshot := c.cache.Load(); snapshot != nil {
		return snapshot
	}
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if snapshot := c.cache.Load(); snapshot != nil {
		return snapshot
	}
	snapshot := newClientCache()
	c.cache.Store(snapshot)
	return snapshot
}

func (c *UtcpClient) cachedTool(name string) (*resolvedTool, bool) {
	if resolved, ok := c.cacheSnapshot().tools[name]; ok {
		return resolved, true
	}
	if resolved, ok := c.fallback.Load(name); ok {
		return resolved.(*resolvedTool), true
	}
	return nil, false
}

func (c *UtcpClient) cachedProviderTools(name string) ([]Tool, bool) {
	providerTools, ok := c.cacheSnapshot().providerTools[name]
	if !ok {
		return nil, false
	}
	return append([]Tool(nil), providerTools...), true
}

func (c *UtcpClient) publishProvider(name string, provider Provider, transport ClientTransport, tools []Tool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	current := c.cache.Load()
	if current == nil {
		current = newClientCache()
	}

	resolved := make(map[string]*resolvedTool, len(current.tools)+len(tools))
	for toolName, cached := range current.tools {
		resolved[toolName] = cached
	}
	if previous := current.providerTools[name]; previous != nil {
		for _, tool := range previous {
			delete(resolved, tool.Name)
		}
	}

	toolsCopy := append([]Tool(nil), tools...)
	for _, tool := range toolsCopy {
		callName := tool.Name
		if provider.Type() == ProviderMCP || provider.Type() == ProviderText {
			if _, suffix, ok := strings.Cut(tool.Name, "."); ok {
				callName = suffix
			}
		}
		resolved[tool.Name] = &resolvedTool{
			provider:  provider,
			transport: transport,
			callName:  callName,
		}
	}

	providers := make(map[string][]Tool, len(current.providerTools)+1)
	for providerName, cached := range current.providerTools {
		providers[providerName] = cached
	}
	providers[name] = toolsCopy
	c.cache.Store(&clientCache{tools: resolved, providerTools: providers})
	c.clearFallbackProvider(name)
}

func (c *UtcpClient) publishResolved(name string, resolved *resolvedTool) {
	c.fallback.Store(name, resolved)
}

func (c *UtcpClient) removeCachedProvider(name string) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	current := c.cache.Load()
	if current == nil {
		return
	}

	tools := make(map[string]*resolvedTool, len(current.tools))
	for toolName, cached := range current.tools {
		if strings.HasPrefix(toolName, name+".") || c.getProviderName(cached.provider) == name {
			continue
		}
		tools[toolName] = cached
	}
	providers := make(map[string][]Tool, len(current.providerTools))
	for providerName, cached := range current.providerTools {
		if providerName != name {
			providers[providerName] = cached
		}
	}
	c.cache.Store(&clientCache{tools: tools, providerTools: providers})
	c.clearFallbackProvider(name)
}

func (c *UtcpClient) clearFallbackProvider(name string) {
	c.fallback.Range(func(key, value any) bool {
		toolName := key.(string)
		resolved := value.(*resolvedTool)
		if strings.HasPrefix(toolName, name+".") || c.getProviderName(resolved.provider) == name {
			c.fallback.Delete(toolName)
		}
		return true
	})
}
