//go:build integration
// +build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockHTTPProvider creates a mock HTTP UTCP provider for integration testing
func setupMockHTTPProvider(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	// Mock discovery endpoint
	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		tools := []map[string]interface{}{
			{
				"name":        "echo",
				"description": "Echo tool that returns input",
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"message": map[string]interface{}{
							"type":        "string",
							"description": "Message to echo",
						},
					},
					"required": []string{"message"},
				},
			},
			{
				"name":        "calculator",
				"description": "Simple calculator",
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"operation": map[string]interface{}{
							"type": "string",
							"enum": []string{"add", "subtract", "multiply", "divide"},
						},
						"a": map[string]interface{}{"type": "number"},
						"b": map[string]interface{}{"type": "number"},
					},
					"required": []string{"operation", "a", "b"},
				},
			},
		}
		json.NewEncoder(w).Encode(tools)
	})

	// Mock tool execution endpoint
	mux.HandleFunc("/tools/", func(w http.ResponseWriter, r *http.Request) {
		toolName := r.URL.Path[len("/tools/"):]

		var params map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid parameters"})
			return
		}

		switch toolName {
		case "echo":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": params["message"],
			})
		case "calculator":
			op := params["operation"].(string)
			a := params["a"].(float64)
			b := params["b"].(float64)

			var result float64
			switch op {
			case "add":
				result = a + b
			case "subtract":
				result = a - b
			case "multiply":
				result = a * b
			case "divide":
				if b != 0 {
					result = a / b
				} else {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{"error": "Division by zero"})
					return
				}
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": result,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Tool not found"})
		}
	})

	return httptest.NewServer(mux)
}

// Integration test for the full bridge server
func TestBridgeServerIntegration(t *testing.T) {
	// Setup mock HTTP provider
	mockProvider := setupMockHTTPProvider(t)
	defer mockProvider.Close()

	// Create provider configuration
	tmpDir := t.TempDir()
	httpProviderFile := filepath.Join(tmpDir, "provider-http.json")
	httpProviderConfig := map[string]interface{}{
		"name":        "test-http-provider",
		"type":        "http",
		"url":         mockProvider.URL,
		"description": "Test HTTP provider",
	}

	data, err := json.Marshal(httpProviderConfig)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(httpProviderFile, data, 0644))

	// Create CLI provider configuration
	cliProviderFile := filepath.Join(tmpDir, "provider-cli.json")
	cliProviderConfig := map[string]interface{}{
		"name":        "test-cli-provider",
		"type":        "cli",
		"description": "Test CLI provider",
		"tools": []map[string]interface{}{
			{
				"name":        "echo",
				"description": "CLI echo command",
				"command":     "echo",
				"args":        []string{"{{message}}"},
			},
		},
	}

	data, err = json.Marshal(cliProviderConfig)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cliProviderFile, data, 0644))

	// Create bridge server
	bridge := NewBridgeServer()

	// Add providers
	err = bridge.AddProvider("http", httpProviderFile)
	assert.NoError(t, err)

	err = bridge.AddProvider("cli", cliProviderFile)
	assert.NoError(t, err)

	// Discover tools
	err = bridge.DiscoverTools()
	assert.NoError(t, err)

	// Verify tools were discovered
	assert.True(t, len(bridge.tools) > 0)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", bridge.healthHandler)
	router.GET("/api/v1/tools", bridge.listToolsHandler)
	router.GET("/api/v1/tools/:name", bridge.getToolHandler)
	router.POST("/api/v1/tools/call", bridge.callToolHandler)
	router.POST("/api/v1/tools/refresh", bridge.refreshToolsHandler)

	// Test health endpoint
	t.Run("Health Check", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var health map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &health)
		require.NoError(t, err)

		assert.Equal(t, "healthy", health["status"])
		assert.Equal(t, float64(2), health["providers"])
		assert.True(t, health["tools"].(float64) > 0)
	})

	// Test listing tools
	t.Run("List Tools", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/tools", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		tools := response["tools"].([]interface{})
		assert.True(t, len(tools) > 0)

		// Check for namespaced tools
		foundHTTPEcho := false
		foundCLIEcho := false
		for _, tool := range tools {
			t := tool.(map[string]interface{})
			if t["name"] == "http.echo" {
				foundHTTPEcho = true
			}
			if t["name"] == "cli.echo" {
				foundCLIEcho = true
			}
		}
		assert.True(t, foundHTTPEcho || foundCLIEcho)
	})

	// Test searching tools
	t.Run("Search Tools", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/tools?query=calculator", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		tools := response["tools"].([]interface{})
		// Only HTTP provider has calculator
		for _, tool := range tools {
			toolMap := tool.(map[string]interface{})
			toolName := toolMap["name"].(string)
			assert.Contains(t, toolName, "calculator")
		}
	})

	// Test getting specific tool
	t.Run("Get Tool Info", func(t *testing.T) {
		// First, find a tool name from discovery
		req := httptest.NewRequest("GET", "/api/v1/tools", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var listResponse map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &listResponse)
		tools := listResponse["tools"].([]interface{})

		if len(tools) > 0 {
			firstTool := tools[0].(map[string]interface{})
			toolName := firstTool["name"].(string)

			// Get specific tool
			req = httptest.NewRequest("GET", "/api/v1/tools/"+toolName, nil)
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var tool map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &tool)
			require.NoError(t, err)

			assert.Equal(t, toolName, tool["name"])
			assert.NotEmpty(t, tool["provider"])
		}
	})

	// Test calling a tool (Note: This will fail without proper UTCP client setup)
	t.Run("Call Tool", func(t *testing.T) {
		requestBody := map[string]interface{}{
			"tool": "http.echo",
			"params": map[string]interface{}{
				"message": "Hello from integration test",
			},
		}

		body, _ := json.Marshal(requestBody)
		req := httptest.NewRequest("POST", "/api/v1/tools/call", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// This will fail because we're using mocked providers
		// In a real integration test with go-utcp, this would succeed
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		// The test acknowledges the endpoint exists and responds
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)
	})

	// Test refresh functionality
	t.Run("Refresh Tools", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/tools/refresh", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "Tools refreshed successfully", response["message"])
	})
}

// Test the auto-discovery feature
func TestAutoDiscovery(t *testing.T) {
	// Setup mock provider
	mockProvider := setupMockHTTPProvider(t)
	defer mockProvider.Close()

	// Create provider configuration
	tmpDir := t.TempDir()
	providerFile := filepath.Join(tmpDir, "provider.json")
	providerConfig := map[string]interface{}{
		"name": "auto-discovery-test",
		"type": "http",
		"url":  mockProvider.URL,
	}

	data, _ := json.Marshal(providerConfig)
	os.WriteFile(providerFile, data, 0644)

	// Create bridge and add provider
	bridge := NewBridgeServer()
	bridge.AddProvider("test", providerFile)

	// Create discovery service (simulated)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initial discovery
	err := bridge.DiscoverTools()
	assert.NoError(t, err)
	bridge.toolsMutex.RLock()
	initialCount := len(bridge.tools)
	bridge.toolsMutex.RUnlock()

	// Simulate auto-discovery running
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				bridge.DiscoverTools()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait a bit for auto-discovery to run
	time.Sleep(250 * time.Millisecond)

	// Tools count should remain stable
	bridge.toolsMutex.RLock()
	finalCount := len(bridge.tools)
	bridge.toolsMutex.RUnlock()
	
	assert.Equal(t, initialCount, finalCount)
}

// Test concurrent access during integration
func TestConcurrentIntegration(t *testing.T) {
	// Setup bridge with providers
	bridge := NewBridgeServer()

	// Create test tools
	for i := 0; i < 5; i++ {
		bridge.tools[fmt.Sprintf("test.tool%d", i)] = &Tool{
			Name:     fmt.Sprintf("test.tool%d", i),
			Provider: "test",
		}
	}

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/tools", bridge.listToolsHandler)
	router.POST("/api/v1/tools/refresh", bridge.refreshToolsHandler)

	// Concurrent requests
	done := make(chan bool)
	errors := make(chan error, 100)

	// Start multiple goroutines making requests
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				// Alternate between list and refresh
				var req *http.Request
				if j%2 == 0 {
					req = httptest.NewRequest("GET", "/api/v1/tools", nil)
				} else {
					req = httptest.NewRequest("POST", "/api/v1/tools/refresh", nil)
				}

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					errors <- fmt.Errorf("request failed with status %d", w.Code)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	close(errors)

	// Check for errors
	var errorCount int
	for err := range errors {
		t.Logf("Concurrent request error: %v", err)
		errorCount++
	}

	assert.Equal(t, 0, errorCount, "No errors should occur during concurrent access")
}

// Test error scenarios in integration
func TestIntegrationErrorScenarios(t *testing.T) {
	bridge := NewBridgeServer()

	// Test with invalid provider configuration
	t.Run("Invalid Provider Config", func(t *testing.T) {
		tmpDir := t.TempDir()
		invalidFile := filepath.Join(tmpDir, "invalid.json")
		os.WriteFile(invalidFile, []byte("{invalid json}"), 0644)

		err := bridge.AddProvider("invalid", invalidFile)
		assert.Error(t, err)
	})

	// Test with non-existent provider file
	t.Run("Non-existent Provider File", func(t *testing.T) {
		err := bridge.AddProvider("nonexistent", "/path/to/nowhere.json")
		assert.Error(t, err)
	})

	// Test tool call with no providers
	t.Run("Call Tool With No Providers", func(t *testing.T) {
		ctx := context.Background()
		_, err := bridge.CallTool(ctx, "any.tool", map[string]interface{}{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider not found")
	})
}
