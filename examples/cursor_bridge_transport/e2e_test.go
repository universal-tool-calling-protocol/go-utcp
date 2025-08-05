//go:build e2e
// +build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E test that runs the actual server and client
func TestE2EFullSystem(t *testing.T) {
	// Skip if not running E2E tests
	if os.Getenv("RUN_E2E_TESTS") != "true" {
		t.Skip("Skipping E2E tests. Set RUN_E2E_TESTS=true to run.")
	}

	// Setup test directory
	testDir := t.TempDir()

	// Create provider configurations
	setupE2EProviders(t, testDir)

	// Start the bridge server in a goroutine
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	serverStarted := make(chan bool)
	serverPort := "18080" // Use a different port for testing

	go func() {
		os.Setenv("PORT", serverPort)
		defer os.Unsetenv("PORT")

		// Change to test directory
		originalDir, _ := os.Getwd()
		os.Chdir(testDir)
		defer os.Chdir(originalDir)

		// Signal that server is starting
		close(serverStarted)

		// Run the server (this would be the actual main() in production)
		runTestServer(serverCtx, serverPort)
	}()

	// Wait for server to start
	<-serverStarted
	time.Sleep(2 * time.Second) // Give server time to initialize

	// Run client tests against the server
	serverURL := fmt.Sprintf("http://localhost:%s", serverPort)

	t.Run("E2E Health Check", func(t *testing.T) {
		resp, err := http.Get(serverURL + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var health map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&health)
		require.NoError(t, err)

		assert.Equal(t, "healthy", health["status"])
	})

	t.Run("E2E List Tools", func(t *testing.T) {
		resp, err := http.Get(serverURL + "/api/v1/tools")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		tools := response["tools"].([]interface{})
		assert.True(t, len(tools) > 0)
	})

	t.Run("E2E Call Tool", func(t *testing.T) {
		// First get available tools
		resp, err := http.Get(serverURL + "/api/v1/tools")
		require.NoError(t, err)

		var listResponse map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&listResponse)
		resp.Body.Close()

		tools := listResponse["tools"].([]interface{})
		if len(tools) > 0 {
			// Call the first available tool
			firstTool := tools[0].(map[string]interface{})
			toolName := firstTool["name"].(string)

			callRequest := map[string]interface{}{
				"tool": toolName,
				"params": map[string]interface{}{
					"message": "E2E test message",
				},
			}

			body, _ := json.Marshal(callRequest)
			resp, err = http.Post(
				serverURL+"/api/v1/tools/call",
				"application/json",
				bytes.NewReader(body),
			)
			require.NoError(t, err)
			defer resp.Body.Close()

			var callResponse map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&callResponse)

			// Check that we got a response (success or error)
			assert.NotNil(t, callResponse)
		}
	})

	// Test using the CLI client if available
	t.Run("E2E CLI Client", func(t *testing.T) {
		// Check if client binary exists
		clientPath := filepath.Join("..", "cursor_bridge_client", "cursor-utcp")
		if _, err := os.Stat(clientPath); os.IsNotExist(err) {
			t.Skip("Client binary not found, skipping CLI tests")
		}

		// Test list command
		cmd := exec.Command(clientPath, "-url", serverURL, "-cmd", "list")
		output, err := cmd.Output()
		if err == nil {
			var result map[string]interface{}
			err = json.Unmarshal(output, &result)
			assert.NoError(t, err)
			assert.NotNil(t, result["tools"])
		}

		// Test health command
		cmd = exec.Command(clientPath, "-url", serverURL, "-cmd", "health")
		output, err = cmd.Output()
		if err == nil {
			var health map[string]interface{}
			err = json.Unmarshal(output, &health)
			assert.NoError(t, err)
			assert.Equal(t, "healthy", health["status"])
		}
	})
}

// Test server under load
func TestE2ELoadTest(t *testing.T) {
	if os.Getenv("RUN_LOAD_TESTS") != "true" {
		t.Skip("Skipping load tests. Set RUN_LOAD_TESTS=true to run.")
	}

	// Setup and start server
	bridge := NewBridgeServer()

	// Add test tools
	for i := 0; i < 100; i++ {
		bridge.tools[fmt.Sprintf("load.tool%d", i)] = &Tool{
			Name:        fmt.Sprintf("load.tool%d", i),
			Description: fmt.Sprintf("Load test tool %d", i),
			Provider:    "load",
		}
	}

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", bridge.healthHandler)
	router.GET("/api/v1/tools", bridge.listToolsHandler)

	// Create test server
	server := httptest.NewServer(router)
	defer server.Close()

	// Metrics
	var (
		totalRequests   int
		successRequests int
		totalDuration   time.Duration
	)

	// Run concurrent requests
	concurrency := 50
	requestsPerClient := 100
	done := make(chan bool, concurrency)

	startTime := time.Now()

	for i := 0; i < concurrency; i++ {
		go func(clientID int) {
			client := &http.Client{
				Timeout: 5 * time.Second,
			}

			for j := 0; j < requestsPerClient; j++ {
				// Alternate between different endpoints
				var url string
				if j%3 == 0 {
					url = server.URL + "/health"
				} else if j%3 == 1 {
					url = server.URL + "/api/v1/tools"
				} else {
					url = fmt.Sprintf("%s/api/v1/tools?query=tool%d", server.URL, j%10)
				}

				reqStart := time.Now()
				resp, err := client.Get(url)
				reqDuration := time.Since(reqStart)

				totalRequests++
				totalDuration += reqDuration

				if err == nil && resp.StatusCode == http.StatusOK {
					successRequests++
					resp.Body.Close()
				}
			}

			done <- true
		}(i)
	}

	// Wait for all clients
	for i := 0; i < concurrency; i++ {
		<-done
	}

	duration := time.Since(startTime)

	// Calculate metrics
	successRate := float64(successRequests) / float64(totalRequests) * 100
	avgLatency := totalDuration / time.Duration(totalRequests)
	throughput := float64(totalRequests) / duration.Seconds()

	// Log results
	t.Logf("Load Test Results:")
	t.Logf("  Total Requests: %d", totalRequests)
	t.Logf("  Successful: %d (%.2f%%)", successRequests, successRate)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f req/s", throughput)
	t.Logf("  Avg Latency: %v", avgLatency)

	// Assertions
	assert.Greater(t, successRate, 95.0, "Success rate should be above 95%")
	assert.Less(t, avgLatency, 100*time.Millisecond, "Average latency should be under 100ms")
}

// Test failover and recovery
func TestE2EFailoverRecovery(t *testing.T) {
	// Create bridge with mock providers
	bridge := NewBridgeServer()

	// Setup providers that can be toggled
	mockProvider1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tools" {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"name": "tool1", "description": "Tool from provider 1"},
			})
		}
	}))
	defer mockProvider1.Close()

	mockProvider2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tools" {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"name": "tool2", "description": "Tool from provider 2"},
			})
		}
	}))
	defer mockProvider2.Close()

	// Add providers
	tmpDir := t.TempDir()

	provider1File := filepath.Join(tmpDir, "provider1.json")
	provider1Config := map[string]interface{}{
		"name": "provider1",
		"type": "http",
		"url":  mockProvider1.URL,
	}
	data, _ := json.Marshal(provider1Config)
	os.WriteFile(provider1File, data, 0644)
	bridge.AddProvider("provider1", provider1File)

	provider2File := filepath.Join(tmpDir, "provider2.json")
	provider2Config := map[string]interface{}{
		"name": "provider2",
		"type": "http",
		"url":  mockProvider2.URL,
	}
	data, _ = json.Marshal(provider2Config)
	os.WriteFile(provider2File, data, 0644)
	bridge.AddProvider("provider2", provider2File)

	// Initial discovery
	err := bridge.DiscoverTools()
	assert.NoError(t, err)
	// Each HTTP provider adds 2 tools (echo and status)
	assert.Equal(t, 4, len(bridge.tools))

	// Simulate provider 1 failure by closing it
	mockProvider1.Close()

	// Rediscover - should still have tools from provider 2
	err = bridge.DiscoverTools()
	assert.NoError(t, err)

	// Should still have at least one tool (from provider 2)
	toolCount := 0
	for name := range bridge.tools {
		if strings.HasPrefix(name, "provider2.") {
			toolCount++
		}
	}
	assert.Greater(t, toolCount, 0, "Should still have tools from working provider")
}

// Helper functions

func setupE2EProviders(t *testing.T, dir string) {
	// Create HTTP provider config
	httpConfig := map[string]interface{}{
		"name":        "e2e-http",
		"type":        "http",
		"url":         "http://localhost:9999", // Mock URL
		"description": "E2E HTTP provider",
	}
	data, _ := json.Marshal(httpConfig)
	os.WriteFile(filepath.Join(dir, "provider-http.json"), data, 0644)

	// Create CLI provider config
	cliConfig := map[string]interface{}{
		"name":        "e2e-cli",
		"type":        "cli",
		"description": "E2E CLI provider",
		"tools": []map[string]interface{}{
			{
				"name":        "echo",
				"description": "Echo command",
				"command":     "echo",
				"args":        []string{"{{message}}"},
			},
			{
				"name":        "date",
				"description": "Date command",
				"command":     "date",
			},
		},
	}
	data, _ = json.Marshal(cliConfig)
	os.WriteFile(filepath.Join(dir, "provider-cli.json"), data, 0644)
}

func runTestServer(ctx context.Context, port string) {
	// This simulates the main() function for testing
	bridge := NewBridgeServer()

	// Load providers from current directory
	files, _ := filepath.Glob("provider-*.json")
	for _, file := range files {
		name := filepath.Base(file)
		name = name[9 : len(name)-5] // Remove "provider-" and ".json"
		bridge.AddProvider(name, file)
	}

	// Initial discovery
	bridge.DiscoverTools()

	// Setup router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Routes
	router.GET("/health", bridge.healthHandler)
	router.GET("/api/v1/tools", bridge.listToolsHandler)
	router.GET("/api/v1/tools/:name", bridge.getToolHandler)
	router.POST("/api/v1/tools/call", bridge.callToolHandler)
	router.POST("/api/v1/tools/refresh", bridge.refreshToolsHandler)
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"service": "cursor-utcp-bridge-e2e-test",
			"version": "1.0.0",
		})
	})

	// Start server
	server := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	server.ListenAndServe()
}

// Test data consistency across operations
func TestE2EDataConsistency(t *testing.T) {
	bridge := NewBridgeServer()

	// Add test tools
	initialTools := map[string]*Tool{
		"test.tool1": {Name: "test.tool1", Provider: "test", Description: "Tool 1"},
		"test.tool2": {Name: "test.tool2", Provider: "test", Description: "Tool 2"},
		"test.tool3": {Name: "test.tool3", Provider: "test", Description: "Tool 3"},
	}

	for name, tool := range initialTools {
		bridge.tools[name] = tool
	}

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/tools", bridge.listToolsHandler)
	router.GET("/api/v1/tools/:name", bridge.getToolHandler)

	// Verify initial state
	req := httptest.NewRequest("GET", "/api/v1/tools", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	tools := response["tools"].([]interface{})
	assert.Equal(t, 3, len(tools))

	// Verify each tool individually
	for name := range initialTools {
		req = httptest.NewRequest("GET", "/api/v1/tools/"+name, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var tool map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &tool)
		assert.Equal(t, name, tool["name"])
	}

	// Simulate concurrent modifications
	done := make(chan bool, 3)

	// Reader goroutine
	go func() {
		for i := 0; i < 50; i++ {
			req := httptest.NewRequest("GET", "/api/v1/tools", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		}
		done <- true
	}()

	// Writer goroutine 1
	go func() {
		for i := 0; i < 25; i++ {
			bridge.toolsMutex.Lock()
			bridge.tools[fmt.Sprintf("new.tool%d", i)] = &Tool{
				Name:     fmt.Sprintf("new.tool%d", i),
				Provider: "new",
			}
			bridge.toolsMutex.Unlock()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Writer goroutine 2
	go func() {
		for i := 0; i < 25; i++ {
			bridge.toolsMutex.Lock()
			// Simulate tool updates
			if tool, exists := bridge.tools["test.tool1"]; exists {
				tool.Description = fmt.Sprintf("Updated %d times", i)
			}
			bridge.toolsMutex.Unlock()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for all operations
	for i := 0; i < 3; i++ {
		<-done
	}

	// Final verification
	req = httptest.NewRequest("GET", "/api/v1/tools", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	json.Unmarshal(w.Body.Bytes(), &response)
	finalTools := response["tools"].([]interface{})

	// Should have original 3 + 25 new tools
	assert.Equal(t, 28, len(finalTools))

	// Verify data integrity
	toolNames := make(map[string]bool)
	for _, tool := range finalTools {
		toolMap := tool.(map[string]interface{})
		name := toolMap["name"].(string)
		assert.False(t, toolNames[name], "No duplicate tools")
		toolNames[name] = true
	}
}
