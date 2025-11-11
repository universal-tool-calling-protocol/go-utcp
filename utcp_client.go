package utcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/universal-tool-calling-protocol/go-utcp/src/openapi"
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

// --- FAST PATH PRIMITIVES ---
// We keep a read-optimized cache for tool resolution and inlined fast-callers to
// remove per-call map lookups/allocs.
type fastCaller func(ctx context.Context, args map[string]any) (any, error)
type fastStreamCaller func(ctx context.Context, args map[string]any) (transports.StreamResult, error)

// UtcpClientInterface defines the public API.
type UtcpClientInterface interface {
	RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error)
	DeregisterToolProvider(ctx context.Context, providerName string) error
	CallTool(ctx context.Context, toolName string, args map[string]any) (any, error)
	SearchTools(query string, limit int) ([]Tool, error)
	GetTransports() map[string]ClientTransport
	CallToolStream(ctx context.Context, toolName string, args map[string]any) (transports.StreamResult, error)
	CallToolChain(ctx context.Context, steps []ChainStep, timeout time.Duration) (map[string]any, error)
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

	// Legacy caches (used for invalidation + scans)
	providerToolsCache    map[string][]Tool
	providerToolsCacheMu  sync.RWMutex
	toolResolutionCache   map[string]*resolvedTool // key is full tool name
	toolResolutionCacheMu sync.RWMutex

	// New caches
	resolved sync.Map // map[string]*resolvedTool
	callers  sync.Map // map[string]fastCaller
	streams  sync.Map // map[string]fastStreamCaller
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

	// If providersFilePath is set, we *used to* adjust a Text transport base path.
	// Removed because there's no TextTransport in this codebase.
	if cfg.ProvidersFilePath != "" {
		_ = filepath.Dir(cfg.ProvidersFilePath) // keep import; may be useful later
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
	// NOTE: If runtime logging overhead shows up in profiles, consider passing no-op loggers here.
	return map[string]ClientTransport{
		"http": NewHttpClientTransport(
			func(format string, args ...interface{}) {
				fmt.Printf("HTTP Transport: "+format+"\n", args...)
			},
		),
		"cli": NewCliTransport(
			func(format string, args ...interface{}) {
				fmt.Printf("CLI Transport: "+format+"\n", args...)
			},
		),
		"sse": NewSSETransport(func(format string, args ...interface{}) {
			fmt.Printf("SSE Transport: "+format+"\n", args...)
		}),
		"http_stream": NewStreamableHTTPTransport(func(format string, args ...interface{}) {
			fmt.Printf("HTTP Stream Transport: "+format+"\n", args...)
		}),
		"mcp": NewMCPTransport(
			func(format string, args ...interface{}) {
				fmt.Printf("MCP Transport: "+format+"\n", args...)
			},
		),
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
		"text": texttransport.NewTextTransport(func(format string, args ...interface{}) {
			fmt.Printf("Text Transport: "+format+"\n", args...)
		}),
	}
}

// Updated loadProviders method using the new parser
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
	successCount := 0

	for i, raw := range rawList {
		if err := c.processProvider(ctx, raw, i); err != nil {
			errors = append(errors, fmt.Sprintf("provider %d: %v", i, err))
			continue
		}
		successCount++
	}
	if len(errors) > 0 {
		for _, errMsg := range errors {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", errMsg)
		}
	}

	_ = successCount // Currently unused; keep for future logging/metrics
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
		p.Name = name
	case *TextProvider:
		p.Name = name
	}
}

// RegisterToolProvider applies variable substitution, picks the right transport, and registers tools.
// RegisterToolProvider picks the transport and registers tools.
// NOTE: do NOT call substituteProviderVariables here; processProvider already did it.
func (c *UtcpClient) RegisterToolProvider(
	ctx context.Context,
	prov Provider,
) ([]Tool, error) {
	c.ensureCaches()

	// Derive/sanitize provider name with a safe fallback (prevents ".tool" cases).
	name := strings.ReplaceAll(c.getProviderName(prov), ".", "_")
	if name == "" {
		name = strings.ToLower(string(prov.Type()))
		if name == "" {
			name = "provider"
		}
	}
	c.setProviderName(prov, name)

	// Cache hit? Return and prime fast caches (S1005 fix: no blank identifier).
	c.providerToolsCacheMu.RLock()
	if tools, ok := c.providerToolsCache[name]; ok {
		c.providerToolsCacheMu.RUnlock()

		tr := c.transports[string(prov.Type())]
		if tr == nil {
			// Defensive: provider cached but transport missing; skip priming.
			return tools, nil
		}

		c.toolResolutionCacheMu.Lock()
		for i := range tools {
			if _, exists := c.toolResolutionCache[tools[i].Name]; !exists {
				callName := tools[i].Name
				if prov.Type() == ProviderMCP || prov.Type() == ProviderText {
					if _, suffix, ok := strings.Cut(tools[i].Name, "."); ok {
						callName = suffix
					}
				}
				res := &resolvedTool{provider: prov, transport: tr, callName: callName, tool: &tools[i]}
				c.toolResolutionCache[tools[i].Name] = res
				c.setResolvedSync(tools[i].Name, res)
				c.setFastCallerSync(tools[i].Name, newFastCaller(prov, tr, callName))
				c.setFastStreamCallerSync(tools[i].Name, newFastStreamCaller(prov, tr, callName))
			}
		}
		c.toolResolutionCacheMu.Unlock()

		return tools, nil
	}
	c.providerToolsCacheMu.RUnlock()

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

	// Ask transport (HTTP may use OpenAPI converter)
	var (
		tools []Tool
		err   error
	)
	if prov.Type() == ProviderHTTP {
		if httpProv, ok := prov.(*HttpProvider); ok {
			converter, convErr := openapi.NewConverterFromURL(httpProv.URL, "")
			if convErr != nil {
				tools, err = tr.RegisterToolProvider(ctx, prov) // fallback
				if err != nil {
					return nil, err
				}
			} else {
				manual := converter.Convert()
				if len(manual.Tools) == 0 {
					tools, err = tr.RegisterToolProvider(ctx, prov) // fallback
					if err != nil {
						return nil, err
					}
				} else {
					tools = manual.Tools
				}
			}
		} else {
			tools, err = tr.RegisterToolProvider(ctx, prov)
			if err != nil {
				return nil, err
			}
		}
	} else {
		tools, err = tr.RegisterToolProvider(ctx, prov)
		if err != nil {
			return nil, err
		}
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

	// Cache provider tools
	c.providerToolsCacheMu.Lock()
	c.providerToolsCache[name] = tools
	c.providerToolsCacheMu.Unlock()

	// Prime resolution + fast-call caches
	c.toolResolutionCacheMu.Lock()
	for i := range tools {
		callName := tools[i].Name
		if prov.Type() == ProviderMCP || prov.Type() == ProviderText {
			if _, suffix, ok := strings.Cut(tools[i].Name, "."); ok {
				callName = suffix
			}
		}
		res := &resolvedTool{provider: prov, transport: tr, callName: callName, tool: &tools[i]}
		c.toolResolutionCache[tools[i].Name] = res
		c.setResolvedSync(tools[i].Name, res)
		c.setFastCallerSync(tools[i].Name, newFastCaller(prov, tr, callName))
		c.setFastStreamCallerSync(tools[i].Name, newFastStreamCaller(prov, tr, callName))
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
			c.deleteFastCaches(k)
		} else if res.provider != nil {
			// Fallback check: if provider name matches
			if c.getProviderName(res.provider) == providerName {
				delete(c.toolResolutionCache, k)
				c.deleteFastCaches(k)
			}
		}
	}
	c.toolResolutionCacheMu.Unlock()

	return nil
}

// --- HOT PATH: CallTool with lock-free fast lookup ---
func (c *UtcpClient) CallTool(
	ctx context.Context,
	toolName string,
	args map[string]any,
) (any, error) {
	// 1) Lock-free fast path: Try to fetch the fast-caller immediately
	if fn, ok := c.getFastCaller(toolName); ok {
		return fn(ctx, args)
	}

	// 2) Slow path: Resolve tool details and cache them for future use
	prov, tr, callName, _, err := c.resolveTool(ctx, toolName)
	if err != nil {
		return nil, err
	}

	// Use conditional locking or improved cache handling
	var fn fastCaller
	if fn, err = c.getOrCreateFastCaller(toolName, prov, tr, callName); err != nil {
		return nil, err
	}

	return fn(ctx, args)
}

func (c *UtcpClient) SearchTools(query string, limit int) ([]Tool, error) {
	tools, err := c.searchStrategy.SearchTools(context.Background(), query, limit)
	if err != nil {
		return nil, err
	}

	// Convert []*Tool to []Tool if needed
	result := make([]Tool, len(tools))
	for i, tool := range tools {
		switch t := any(tool).(type) {
		case Tool:
			result[i] = t
		case *Tool:
			result[i] = *t
		default:
			// fallback (shouldn't happen)
			result[i] = Tool{}
		}
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
		return varRe.ReplaceAllStringFunc(v, func(match string) string {
			g := varRe.FindStringSubmatch(match)
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
	// fast path
	if fn, ok := c.getFastStreamCaller(toolName); ok {
		return fn(ctx, args)
	}

	prov, tr, callName, _, err := c.resolveTool(ctx, toolName)
	if err != nil {
		return nil, err
	}
	fn := newFastStreamCaller(prov, tr, callName)
	c.setFastStreamCallerSync(toolName, fn)
	return fn(ctx, args)
}

// helper to resolve provider, transport, callName, and tool
func (c *UtcpClient) resolveTool(ctx context.Context, toolName string) (Provider, ClientTransport, string, *Tool, error) {
	// Lock-free cache lookup first
	if res, ok := c.getResolved(toolName); ok {
		return res.provider, res.transport, res.callName, res.tool, nil
	}

	// Fallback to legacy path (one-time work per tool)
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
	if cloned.Type() == ProviderMCP || cloned.Type() == ProviderText {
		// Strip provider prefix for transports that expect unprefixed names
		callName = suffix
	}

	// Publish into both caches for future lock-free lookups
	res := &resolvedTool{provider: cloned, transport: tr, callName: callName, tool: selectedTool}
	c.toolResolutionCacheMu.Lock()
	c.toolResolutionCache[toolName] = res
	c.toolResolutionCacheMu.Unlock()

	c.setResolvedSync(toolName, res)
	c.setFastCallerSync(toolName, newFastCaller(cloned, tr, callName))
	c.setFastStreamCallerSync(toolName, newFastStreamCaller(cloned, tr, callName))

	return cloned, tr, callName, selectedTool, nil
}

// getFastCaller retrieves a fastCaller for a tool from the cache (sync.Map)
func (c *UtcpClient) getFastCaller(name string) (fastCaller, bool) {
	if v, ok := c.callers.Load(name); ok {
		return v.(fastCaller), true
	}
	return nil, false
}

// setFastCallerSync safely stores the fast-caller in the cache.
func (c *UtcpClient) setFastCallerSync(name string, fn fastCaller) {
	c.callers.Store(name, fn)
}

func (c *UtcpClient) getFastStreamCaller(name string) (fastStreamCaller, bool) {
	if v, ok := c.streams.Load(name); ok {
		return v.(fastStreamCaller), true
	}
	return nil, false
}
func (c *UtcpClient) setFastStreamCallerSync(name string, fn fastStreamCaller) {
	c.streams.Store(name, fn)
}

func (c *UtcpClient) getResolved(name string) (*resolvedTool, bool) {
	if v, ok := c.resolved.Load(name); ok {
		return v.(*resolvedTool), true
	}
	return nil, false
}
func (c *UtcpClient) setResolvedSync(name string, res *resolvedTool) {
	c.resolved.Store(name, res)
}

func (c *UtcpClient) deleteFastCaches(name string) {
	c.resolved.Delete(name)
	c.callers.Delete(name)
	c.streams.Delete(name)
}

// newFastCaller creates a fastCaller function based on the provider, transport, and call name
func newFastCaller(prov Provider, tr ClientTransport, call string) fastCaller {
	p := prov
	t := tr
	cn := call
	return func(ctx context.Context, args map[string]any) (any, error) {
		return t.CallTool(ctx, cn, args, p, nil)
	}
}
func newFastStreamCaller(prov Provider, tr ClientTransport, call string) fastStreamCaller {
	p := prov
	t := tr
	cn := call
	return func(ctx context.Context, args map[string]any) (transports.StreamResult, error) {
		return t.CallToolStream(ctx, cn, args, p)
	}
}

func (c *UtcpClient) ensureCaches() {
	if c.providerToolsCache == nil {
		c.providerToolsCache = make(map[string][]Tool)
	}
	if c.toolResolutionCache == nil {
		c.toolResolutionCache = make(map[string]*resolvedTool)
	}
}

// getOrCreateFastCaller tries to retrieve the fastCaller from cache, or creates and caches a new one.
func (c *UtcpClient) getOrCreateFastCaller(toolName string, prov Provider, tr ClientTransport, callName string) (fastCaller, error) {
	// Check cache first
	if fn, ok := c.getFastCaller(toolName); ok {
		return fn, nil
	}

	// Resolve and cache the fast-caller
	fn := newFastCaller(prov, tr, callName)
	c.setFastCallerSync(toolName, fn)

	return fn, nil
}
