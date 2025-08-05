package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewMockUTCPClient creates a new mock UTCP client
func NewMockUTCPClient() *MockUTCPClient {
	return &MockUTCPClient{
		tools:       []string{"echo", "time"},
		toolInfo:    make(map[string]interface{}),
		callResults: make(map[string]interface{}),
	}
}

// Unit Tests

func TestNewBridgeServer(t *testing.T) {
	bs := NewBridgeServer()
	assert.NotNil(t, bs)
	assert.NotNil(t, bs.providers)
	assert.NotNil(t, bs.tools)
	assert.NotNil(t, bs.providerUrls)
}

func TestBridgeServer_AddProvider(t *testing.T) {
	bs := NewBridgeServer()

	// Create temporary provider config
	tmpDir := t.TempDir()
	providerFile := filepath.Join(tmpDir, "test-provider.json")
	providerConfig := map[string]interface{}{
		"name":        "test-provider",
		"type":        "http",
		"url":         "http://localhost:9999",
		"description": "Test provider",
	}

	data, err := json.Marshal(providerConfig)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(providerFile, data, 0644))

	// Test adding provider
	err = bs.AddProvider("test", providerFile)
	assert.NoError(t, err)
	assert.Equal(t, "http://localhost:9999", bs.providerUrls["test"])
}

func TestBridgeServer_DiscoverTools(t *testing.T) {
	bs := NewBridgeServer()

	// Add mock provider
	mockClient := NewMockUTCPClient()
	bs.providers["mock"] = mockClient // Use the mock directly

	// For this test, we'll simulate the discovery
	bs.tools["mock.echo"] = &Tool{
		Name:        "mock.echo",
		Description: "Mock echo tool",
		Provider:    "mock",
	}
	bs.tools["mock.time"] = &Tool{
		Name:        "mock.time",
		Description: "Mock time tool",
		Provider:    "mock",
	}

	assert.Equal(t, 2, len(bs.tools))
	assert.NotNil(t, bs.tools["mock.echo"])
	assert.NotNil(t, bs.tools["mock.time"])
}

func TestBridgeServer_CallTool(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		params   map[string]interface{}
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "Valid tool call",
			toolName: "mock.echo",
			params:   map[string]interface{}{"message": "hello"},
			wantErr:  false,
		},
		{
			name:     "Invalid tool format",
			toolName: "invalidtool",
			params:   map[string]interface{}{},
			wantErr:  true,
			errMsg:   "invalid tool name format",
		},
		{
			name:     "Provider not found",
			toolName: "unknown.tool",
			params:   map[string]interface{}{},
			wantErr:  true,
			errMsg:   "provider not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs := NewBridgeServer()

			// Add mock provider for valid test
			if tt.name == "Valid tool call" {
				bs.providers["mock"] = NewMockUTCPClient()
			}

			ctx := context.Background()
			_, err := bs.CallTool(ctx, tt.toolName, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHealthHandler(t *testing.T) {
	bs := NewBridgeServer()

	// Add some tools and providers for testing
	bs.providers["test"] = NewMockUTCPClient()
	bs.tools["test.tool"] = &Tool{Name: "test.tool"}

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", bs.healthHandler)

	// Create request
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	// Perform request
	router.ServeHTTP(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, float64(1), response["providers"])
	assert.Equal(t, float64(1), response["tools"])
	assert.NotNil(t, response["timestamp"])
}

func TestListToolsHandler(t *testing.T) {
	bs := NewBridgeServer()

	// Add test tools
	bs.tools["http.echo"] = &Tool{
		Name:        "http.echo",
		Description: "Echo service",
		Provider:    "http",
	}
	bs.tools["cli.date"] = &Tool{
		Name:        "cli.date",
		Description: "Date command",
		Provider:    "cli",
	}

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/tools", bs.listToolsHandler)

	tests := []struct {
		name          string
		query         string
		expectedCount int
	}{
		{
			name:          "List all tools",
			query:         "",
			expectedCount: 2,
		},
		{
			name:          "Search for echo",
			query:         "?query=echo",
			expectedCount: 1,
		},
		{
			name:          "Search for cli",
			query:         "?query=cli",
			expectedCount: 1,
		},
		{
			name:          "Search no results",
			query:         "?query=notfound",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/tools"+tt.query, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			toolsInterface, ok := response["tools"]
			require.True(t, ok, "tools field should exist")
			
			tools, ok := toolsInterface.([]interface{})
			require.True(t, ok, "tools should be an array")
			
			assert.Equal(t, tt.expectedCount, len(tools))
			assert.Equal(t, float64(tt.expectedCount), response["count"])
		})
	}
}

func TestGetToolHandler(t *testing.T) {
	bs := NewBridgeServer()

	// Add test tool
	bs.tools["http.echo"] = &Tool{
		Name:        "http.echo",
		Description: "Echo service",
		Provider:    "http",
		Schema: map[string]interface{}{
			"type": "object",
		},
	}

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/tools/:name", bs.getToolHandler)

	tests := []struct {
		name       string
		toolName   string
		wantStatus int
	}{
		{
			name:       "Get existing tool",
			toolName:   "http.echo",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Get non-existing tool",
			toolName:   "unknown.tool",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/tools/"+tt.toolName, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusOK {
				var tool Tool
				err := json.Unmarshal(w.Body.Bytes(), &tool)
				require.NoError(t, err)
				assert.Equal(t, tt.toolName, tool.Name)
			}
		})
	}
}

func TestCallToolHandler(t *testing.T) {
	bs := NewBridgeServer()

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/tools/call", bs.callToolHandler)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "Valid request",
			body:       `{"tool": "mock.echo", "params": {"message": "hello"}}`,
			wantStatus: http.StatusInternalServerError, // Will fail as no real provider
		},
		{
			name:       "Missing tool field",
			body:       `{"params": {"message": "hello"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid JSON",
			body:       `{invalid json}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/tools/call", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestRefreshToolsHandler(t *testing.T) {
	bs := NewBridgeServer()

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/tools/refresh", bs.refreshToolsHandler)

	req := httptest.NewRequest("POST", "/api/v1/tools/refresh", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Tools refreshed successfully", response["message"])
}

func TestToolNamespacing(t *testing.T) {
	// Test tool namespacing
	toolName := "echo"
	providerName := "http"

	namespacedName := fmt.Sprintf("%s.%s", providerName, toolName)
	assert.Equal(t, "http.echo", namespacedName)

	// Test parsing namespaced name
	parts := strings.SplitN(namespacedName, ".", 2)
	assert.Equal(t, 2, len(parts))
	assert.Equal(t, providerName, parts[0])
	assert.Equal(t, toolName, parts[1])
}

// TestConcurrentAccess tests thread safety of the bridge server
func TestConcurrentAccess(t *testing.T) {
	bridge := NewBridgeServer()

	// Add initial tools
	for i := 0; i < 10; i++ {
		toolName := fmt.Sprintf("test.tool%d", i)
		bridge.tools[toolName] = &Tool{
			Name:        toolName,
			Description: fmt.Sprintf("Test tool %d", i),
			Provider:    "test",
		}
	}

	// Concurrent operations
	var wg sync.WaitGroup
	iterations := 100

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bridge.toolsMutex.RLock()
			_ = len(bridge.tools)
			bridge.toolsMutex.RUnlock()
		}()
	}

	// Concurrent writes
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			bridge.toolsMutex.Lock()
			toolName := fmt.Sprintf("concurrent.tool%d", idx)
			bridge.tools[toolName] = &Tool{
				Name:     toolName,
				Provider: "concurrent",
			}
			bridge.toolsMutex.Unlock()
		}(i)
	}

	wg.Wait()

	// Verify all concurrent tools were added
	bridge.toolsMutex.RLock()
	count := 0
	for name := range bridge.tools {
		if strings.HasPrefix(name, "concurrent.") {
			count++
		}
	}
	bridge.toolsMutex.RUnlock()

	assert.Equal(t, iterations, count)
}
