package utcp

import (
	"context"
	"testing"

	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tag"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

// dummyTransport implements the minimal interface needed.
type dummyTransport struct{}

func (d *dummyTransport) RegisterToolProvider(ctx context.Context, prov base.Provider) ([]tools.Tool, error) {
	// Return one tool with a fixed name
	return []tools.Tool{{Name: "dummy.tool"}}, nil
}
func (d *dummyTransport) CallTool(ctx context.Context, callName string, args map[string]any, prov base.Provider, extra *string) (any, error) {
	// Simulate light work
	return "ok", nil
}
func (d *dummyTransport) DeregisterToolProvider(ctx context.Context, prov base.Provider) error {
	return nil
}
func (d *dummyTransport) CallToolStream(ctx context.Context, toolName string, args map[string]any, p base.Provider) (transports.StreamResult, error) {
	return nil, nil // Not used in this benchmark
}

// The real transports in your code satisfy ClientTransport; adapt this to your actual interfaces.
func setupClientWithTransport(tType string) UtcpClientInterface {
	// Build a client with in-memory repo, simple search strategy, and stubbed transport map.
	cfg := NewClientConfig()
	repo := repository.NewInMemoryToolRepository()
	strat := tag.NewTagSearchStrategy(repo, 1.0)
	client, _ := NewUTCPClient(context.Background(), cfg, repo, strat)

	// Replace the transport of interest with a dummy that’s predictable.
	clientTransports := client.GetTransports()
	clientTransports[tType] = repository.ClientTransport(&dummyTransport{}) // cast to your actual type

	return client
}

func BenchmarkCallTool_ColdWarm_MCP_vs_HTTP(b *testing.B) {
	ctx := context.Background()

	// Setup two clients: one simulating MCP, one HTTP (or generic)
	mcpClient := setupClientWithTransport("mcp")
	httpClient := setupClientWithTransport("http")

	// You need to register a dummy MCP provider and a dummy HTTP provider.
	// Assume you have constructors like NewMCPProvider and NewHTTPProvider; create minimal providers with one tool.
	// For brevity, pseudocode is below—replace with actual provider construction.

	// Register MCP provider
	// provMCP := &utcp.MCPProvider{Name: "prov_mcp", ...}
	// toolsMCP, _ := mcpClient.RegisterToolProvider(ctx, provMCP)
	// toolNameMCP := toolsMCP[0].Name

	// Register HTTP provider
	// provHTTP := &utcp.HttpProvider{Name: "prov_http", ...}
	// toolsHTTP, _ := httpClient.RegisterToolProvider(ctx, provHTTP)
	// toolNameHTTP := toolsHTTP[0].Name

	// For this skeleton, assume you have toolNameMCP and toolNameHTTP populated:
	toolNameMCP := "prov_mcp.someTool"   // replace with actual returned name
	toolNameHTTP := "prov_http.someTool" // replace with actual returned name

	b.Run("MCP_Cold", func(sb *testing.B) {
		sb.ResetTimer()
		for i := 0; i < sb.N; i++ {
			// Use a fresh client to force cold path or explicitly clear caches if your API allows.
			_, _ = mcpClient.CallTool(ctx, toolNameMCP, map[string]any{})
		}
	})

	b.Run("MCP_Warm", func(sb *testing.B) {
		// Prime cache
		_, _ = mcpClient.CallTool(ctx, toolNameMCP, map[string]any{})
		sb.ResetTimer()
		for i := 0; i < sb.N; i++ {
			_, _ = mcpClient.CallTool(ctx, toolNameMCP, map[string]any{})
		}
	})

	b.Run("HTTP_Cold", func(sb *testing.B) {
		sb.ResetTimer()
		for i := 0; i < sb.N; i++ {
			_, _ = httpClient.CallTool(ctx, toolNameHTTP, map[string]any{})
		}
	})

	b.Run("HTTP_Warm", func(sb *testing.B) {
		_, _ = httpClient.CallTool(ctx, toolNameHTTP, map[string]any{})
		sb.ResetTimer()
		for i := 0; i < sb.N; i++ {
			_, _ = httpClient.CallTool(ctx, toolNameHTTP, map[string]any{})
		}
	})
}

func BenchmarkCallTool_Concurrent_Warm(b *testing.B) {
	ctx := context.Background()
	client := setupClientWithTransport("http")
	toolName := "prov_http.someTool" // fill from registration

	// Prime
	_, _ = client.CallTool(ctx, toolName, map[string]any{})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = client.CallTool(ctx, toolName, map[string]any{})
		}
	})
}
