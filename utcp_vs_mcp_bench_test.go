package utcp

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cast"

	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
)

// Enhanced server setup with more realistic tools
func startEnhancedBenchServer(addr string) *mcpserver.StreamableHTTPServer {
	srv := mcpserver.NewMCPServer("demo", "1.0.0")

	// Simple hello tool
	hello := mcp.NewTool("hello", mcp.WithString("name"))
	srv.AddTool(hello, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := cast.ToString(req.GetArguments()["name"])
		if name == "" {
			name = "World"
		}
		return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
	})

	// Echo tool for payload testing
	echo := mcp.NewTool("echo", mcp.WithString("data"))
	srv.AddTool(echo, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		data := cast.ToString(req.GetArguments()["data"])
		return mcp.NewToolResultText(data), nil
	})

	// Processing tool that does some work
	process := mcp.NewTool("process",
		mcp.WithString("text"),
		mcp.WithString("iterations"))
	srv.AddTool(process, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text := cast.ToString(req.GetArguments()["text"])
		iterations := cast.ToInt(req.GetArguments()["iterations"])
		if iterations == 0 {
			iterations = 1
		}

		result := text
		for i := 0; i < iterations; i++ {
			result = strings.ToUpper(strings.ToLower(result))
		}
		return mcp.NewToolResultText(result), nil
	})

	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	go func() { _ = httpSrv.Start(addr) }()
	time.Sleep(200 * time.Millisecond) // Ensure server is ready
	return httpSrv
}

// BenchmarkOptimizedUTCPCall - Optimized UTCP with persistent client
func BenchmarkOptimizedUTCPCall(b *testing.B) {
	httpSrv := startEnhancedBenchServer(":8201")
	defer httpSrv.Shutdown(context.Background())

	ctx := context.Background()
	client, err := NewUTCPClient(ctx, NewClientConfig(), nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer client.DeregisterToolProvider(ctx, "demo")

	prov := &providers.MCPProvider{Name: "demo", URL: "http://localhost:8201/mcp"}
	if _, err := client.RegisterToolProvider(ctx, prov); err != nil {
		b.Fatal(err)
	}

	// Warmup
	for i := 0; i < 10; i++ {
		client.CallTool(ctx, "demo.hello", map[string]any{"name": "warmup"})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := client.CallTool(ctx, "demo.hello", map[string]any{"name": "Bench"}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOptimizedMCPCall - Optimized MCP with persistent client
func BenchmarkOptimizedMCPCall(b *testing.B) {
	httpSrv := startEnhancedBenchServer(":8202")
	defer httpSrv.Shutdown(context.Background())

	ctx := context.Background()
	client, err := mcpclient.NewStreamableHttpClient("http://localhost:8202/mcp")
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	if err := client.Start(ctx); err != nil {
		b.Fatal(err)
	}
	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "bench", Version: "1.0"},
		},
	}
	if _, err := client.Initialize(ctx, initReq); err != nil {
		b.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = "hello"
	req.Params.Arguments = map[string]any{"name": "Bench"}

	// Warmup
	for i := 0; i < 10; i++ {
		client.CallTool(ctx, req)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := client.CallTool(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPayloadComparison - Direct payload size comparison
func BenchmarkPayloadComparison(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"100B", 100},
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}

	for _, s := range sizes {
		payload := strings.Repeat("x", s.size)

		b.Run(fmt.Sprintf("UTCP_%s", s.name), func(b *testing.B) {
			httpSrv := startEnhancedBenchServer(":8210")
			defer httpSrv.Shutdown(context.Background())

			ctx := context.Background()
			client, _ := NewUTCPClient(ctx, NewClientConfig(), nil, nil)
			prov := &providers.MCPProvider{Name: "demo", URL: "http://localhost:8210/mcp"}
			client.RegisterToolProvider(ctx, prov)
			defer client.DeregisterToolProvider(ctx, "demo")

			b.SetBytes(int64(s.size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := client.CallTool(ctx, "demo.echo", map[string]any{"data": payload}); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("MCP_%s", s.name), func(b *testing.B) {
			httpSrv := startEnhancedBenchServer(":8211")
			defer httpSrv.Shutdown(context.Background())

			ctx := context.Background()
			client, _ := mcpclient.NewStreamableHttpClient("http://localhost:8211/mcp")
			defer client.Close()

			client.Start(ctx)
			initReq := mcp.InitializeRequest{
				Params: mcp.InitializeParams{
					ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
					ClientInfo:      mcp.Implementation{Name: "bench", Version: "1.0"},
				},
			}
			client.Initialize(ctx, initReq)

			req := mcp.CallToolRequest{}
			req.Params.Name = "echo"
			req.Params.Arguments = map[string]any{"data": payload}

			b.SetBytes(int64(s.size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := client.CallTool(ctx, req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkConcurrentComparison - Concurrent load comparison
func BenchmarkConcurrentComparison(b *testing.B) {
	b.Run("UTCP_Concurrent", func(b *testing.B) {
		httpSrv := startEnhancedBenchServer(":8220")
		defer httpSrv.Shutdown(context.Background())

		ctx := context.Background()
		client, _ := NewUTCPClient(ctx, NewClientConfig(), nil, nil)
		prov := &providers.MCPProvider{Name: "demo", URL: "http://localhost:8220/mcp"}
		client.RegisterToolProvider(ctx, prov)
		defer client.DeregisterToolProvider(ctx, "demo")

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if _, err := client.CallTool(ctx, "demo.hello", map[string]any{"name": "parallel"}); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("MCP_Concurrent", func(b *testing.B) {
		httpSrv := startEnhancedBenchServer(":8221")
		defer httpSrv.Shutdown(context.Background())

		ctx := context.Background()
		client, _ := mcpclient.NewStreamableHttpClient("http://localhost:8221/mcp")
		defer client.Close()

		client.Start(ctx)
		initReq := mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: "bench", Version: "1.0"},
			},
		}
		client.Initialize(ctx, initReq)

		req := mcp.CallToolRequest{}
		req.Params.Name = "hello"
		req.Params.Arguments = map[string]any{"name": "parallel"}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if _, err := client.CallTool(ctx, req); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

// BenchmarkMemoryComparison - Memory allocation comparison
func BenchmarkMemoryComparison(b *testing.B) {
	b.Run("UTCP_Memory", func(b *testing.B) {
		httpSrv := startEnhancedBenchServer(":8230")
		defer httpSrv.Shutdown(context.Background())

		ctx := context.Background()
		client, _ := NewUTCPClient(ctx, NewClientConfig(), nil, nil)
		prov := &providers.MCPProvider{Name: "demo", URL: "http://localhost:8230/mcp"}
		client.RegisterToolProvider(ctx, prov)
		defer client.DeregisterToolProvider(ctx, "demo")

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := client.CallTool(ctx, "demo.hello", map[string]any{"name": "memory"}); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("MCP_Memory", func(b *testing.B) {
		httpSrv := startEnhancedBenchServer(":8231")
		defer httpSrv.Shutdown(context.Background())

		ctx := context.Background()
		client, _ := mcpclient.NewStreamableHttpClient("http://localhost:8231/mcp")
		defer client.Close()

		client.Start(ctx)
		initReq := mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: "bench", Version: "1.0"},
			},
		}
		client.Initialize(ctx, initReq)

		req := mcp.CallToolRequest{}
		req.Params.Name = "hello"
		req.Params.Arguments = map[string]any{"name": "memory"}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := client.CallTool(ctx, req); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkProcessingLoad - Test with workload simulation
func BenchmarkProcessingLoad(b *testing.B) {
	workloads := []struct {
		name       string
		text       string
		iterations int
	}{
		{"Light", "hello world", 10},
		{"Medium", strings.Repeat("processing load test ", 50), 100},
		{"Heavy", strings.Repeat("heavy processing load test ", 100), 500},
	}

	for _, w := range workloads {
		b.Run(fmt.Sprintf("UTCP_%s", w.name), func(b *testing.B) {
			httpSrv := startEnhancedBenchServer(":8240")
			defer httpSrv.Shutdown(context.Background())

			ctx := context.Background()
			client, _ := NewUTCPClient(ctx, NewClientConfig(), nil, nil)
			prov := &providers.MCPProvider{Name: "demo", URL: "http://localhost:8240/mcp"}
			client.RegisterToolProvider(ctx, prov)
			defer client.DeregisterToolProvider(ctx, "demo")

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				args := map[string]any{
					"text":       w.text,
					"iterations": fmt.Sprintf("%d", w.iterations),
				}
				if _, err := client.CallTool(ctx, "demo.process", args); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("MCP_%s", w.name), func(b *testing.B) {
			httpSrv := startEnhancedBenchServer(":8241")
			defer httpSrv.Shutdown(context.Background())

			ctx := context.Background()
			client, _ := mcpclient.NewStreamableHttpClient("http://localhost:8241/mcp")
			defer client.Close()

			client.Start(ctx)
			initReq := mcp.InitializeRequest{
				Params: mcp.InitializeParams{
					ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
					ClientInfo:      mcp.Implementation{Name: "bench", Version: "1.0"},
				},
			}
			client.Initialize(ctx, initReq)

			req := mcp.CallToolRequest{}
			req.Params.Name = "process"
			req.Params.Arguments = map[string]any{
				"text":       w.text,
				"iterations": fmt.Sprintf("%d", w.iterations),
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := client.CallTool(ctx, req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLatencyMeasurement - Measure call latencies
func BenchmarkLatencyMeasurement(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping latency measurement in short mode")
	}

	const numSamples = 100

	b.Run("UTCP_Latency", func(b *testing.B) {
		httpSrv := startEnhancedBenchServer(":8250")
		defer httpSrv.Shutdown(context.Background())

		ctx := context.Background()
		client, _ := NewUTCPClient(ctx, NewClientConfig(), nil, nil)
		prov := &providers.MCPProvider{Name: "demo", URL: "http://localhost:8250/mcp"}
		client.RegisterToolProvider(ctx, prov)
		defer client.DeregisterToolProvider(ctx, "demo")

		var latencies []time.Duration
		b.ResetTimer()

		for i := 0; i < numSamples; i++ {
			start := time.Now()
			if _, err := client.CallTool(ctx, "demo.hello", map[string]any{"name": "latency"}); err != nil {
				b.Fatal(err)
			}
			latencies = append(latencies, time.Since(start))
		}

		// Calculate and report percentiles
		reportLatencyStats(b, latencies)
	})

	b.Run("MCP_Latency", func(b *testing.B) {
		httpSrv := startEnhancedBenchServer(":8251")
		defer httpSrv.Shutdown(context.Background())

		ctx := context.Background()
		client, _ := mcpclient.NewStreamableHttpClient("http://localhost:8251/mcp")
		defer client.Close()

		client.Start(ctx)
		initReq := mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: "bench", Version: "1.0"},
			},
		}
		client.Initialize(ctx, initReq)

		req := mcp.CallToolRequest{}
		req.Params.Name = "hello"
		req.Params.Arguments = map[string]any{"name": "latency"}

		var latencies []time.Duration
		b.ResetTimer()

		for i := 0; i < numSamples; i++ {
			start := time.Now()
			if _, err := client.CallTool(ctx, req); err != nil {
				b.Fatal(err)
			}
			latencies = append(latencies, time.Since(start))
		}

		// Calculate and report percentiles
		reportLatencyStats(b, latencies)
	})
}

// Helper function to calculate and report latency statistics
func reportLatencyStats(b *testing.B, latencies []time.Duration) {
	if len(latencies) == 0 {
		return
	}

	// Simple bubble sort for small arrays
	for i := 0; i < len(latencies)-1; i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}

	// Calculate percentiles
	percentiles := map[int]time.Duration{
		50: latencies[len(latencies)*50/100],
		90: latencies[len(latencies)*90/100],
		95: latencies[len(latencies)*95/100],
		99: latencies[len(latencies)*99/100],
	}

	// Calculate average
	var total time.Duration
	for _, lat := range latencies {
		total += lat
	}
	avg := total / time.Duration(len(latencies))

	b.Logf("Latency stats - Avg: %v, P50: %v, P90: %v, P95: %v, P99: %v",
		avg, percentiles[50], percentiles[90], percentiles[95], percentiles[99])
}

// BenchmarkErrorHandling - Performance under error conditions
func BenchmarkErrorHandling(b *testing.B) {
	b.Run("UTCP_Errors", func(b *testing.B) {
		httpSrv := startEnhancedBenchServer(":8260")
		defer httpSrv.Shutdown(context.Background())

		ctx := context.Background()
		client, _ := NewUTCPClient(ctx, NewClientConfig(), nil, nil)
		prov := &providers.MCPProvider{Name: "demo", URL: "http://localhost:8260/mcp"}
		client.RegisterToolProvider(ctx, prov)
		defer client.DeregisterToolProvider(ctx, "demo")

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Call non-existent tool - should error quickly
			client.CallTool(ctx, "demo.nonexistent", map[string]any{"test": "data"})
		}
	})

	b.Run("MCP_Errors", func(b *testing.B) {
		httpSrv := startEnhancedBenchServer(":8261")
		defer httpSrv.Shutdown(context.Background())

		ctx := context.Background()
		client, _ := mcpclient.NewStreamableHttpClient("http://localhost:8261/mcp")
		defer client.Close()

		client.Start(ctx)
		initReq := mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: "bench", Version: "1.0"},
			},
		}
		client.Initialize(ctx, initReq)

		req := mcp.CallToolRequest{}
		req.Params.Name = "nonexistent"
		req.Params.Arguments = map[string]any{"test": "data"}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Call non-existent tool - should error quickly
			client.CallTool(ctx, req)
		}
	})
}
