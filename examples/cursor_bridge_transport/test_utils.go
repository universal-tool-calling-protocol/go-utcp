package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestLogger implements a logger for testing
type TestLogger struct {
	mu   sync.Mutex
	logs []string
}

// MockUTCPClient implements a mock UTCP client for testing
type MockUTCPClient struct {
	tools       []string
	toolInfo    map[string]interface{}
	callResults map[string]interface{}
	callError   error
	mu          sync.RWMutex
}

func NewTestLogger() *TestLogger {
	return &TestLogger{
		logs: make([]string, 0),
	}
}

func (l *TestLogger) Log(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, message)
}

func (l *TestLogger) GetLogs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string{}, l.logs...)
}

// MockToolProvider creates a mock HTTP server that implements UTCP protocol
func CreateMockToolProvider(tools []MockTool) *httptest.Server {
	mux := http.NewServeMux()

	// Tool discovery endpoint
	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var toolList []map[string]interface{}
		for _, tool := range tools {
			toolList = append(toolList, map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"schema":      tool.Schema,
			})
		}

		json.NewEncoder(w).Encode(toolList)
	})

	// Tool execution endpoint
	mux.HandleFunc("/tools/", func(w http.ResponseWriter, r *http.Request) {
		toolName := r.URL.Path[len("/tools/"):]

		// Find the tool
		var targetTool *MockTool
		for _, tool := range tools {
			if tool.Name == toolName {
				targetTool = &tool
				break
			}
		}

		if targetTool == nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Tool not found",
			})
			return
		}

		// Parse parameters
		var params map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Invalid parameters",
			})
			return
		}

		// Execute mock handler
		if targetTool.Handler != nil {
			result, err := targetTool.Handler(params)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"error": err.Error(),
				})
				return
			}

			json.NewEncoder(w).Encode(result)
		} else {
			// Default response
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": "Mock execution successful",
				"params": params,
			})
		}
	})

	// Tool info endpoint
	mux.HandleFunc("/tool/", func(w http.ResponseWriter, r *http.Request) {
		toolName := r.URL.Path[len("/tool/"):]

		for _, tool := range tools {
			if tool.Name == toolName {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"name":        tool.Name,
					"description": tool.Description,
					"schema":      tool.Schema,
				})
				return
			}
		}

		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Tool not found"})
	})

	return httptest.NewServer(mux)
}

// MockTool represents a mock tool for testing
type MockTool struct {
	Name        string
	Description string
	Schema      map[string]interface{}
	Handler     func(params map[string]interface{}) (interface{}, error)
}

// CreateTestProviderConfig creates a provider configuration file for testing
func CreateTestProviderConfig(t *testing.T, dir, name, providerType, url string) string {
	config := map[string]interface{}{
		"name":        name,
		"type":        providerType,
		"url":         url,
		"description": "Test provider " + name,
	}

	if providerType == "cli" {
		// Add some CLI tools
		config["tools"] = []map[string]interface{}{
			{
				"name":        "echo",
				"description": "Echo command",
				"command":     "echo",
				"args":        []string{"{{message}}"},
			},
			{
				"name":        "pwd",
				"description": "Print working directory",
				"command":     "pwd",
			},
		}
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal provider config: %v", err)
	}

	filename := filepath.Join(dir, "provider-"+name+".json")
	if err := os.WriteFile(filename, data, 0644); err != nil {
		t.Fatalf("Failed to write provider config: %v", err)
	}

	return filename
}

// CreateTestBridgeServer creates a bridge server with test providers
func CreateTestBridgeServer(t *testing.T) (*BridgeServer, *httptest.Server, func()) {
	// Create mock provider
	mockTools := []MockTool{
		{
			Name:        "test-echo",
			Description: "Test echo tool",
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{
						"type": "string",
					},
				},
			},
			Handler: func(params map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{
					"echo": params["message"],
				}, nil
			},
		},
		{
			Name:        "test-math",
			Description: "Test math operations",
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"a":  map[string]interface{}{"type": "number"},
					"b":  map[string]interface{}{"type": "number"},
					"op": map[string]interface{}{"type": "string"},
				},
			},
			Handler: func(params map[string]interface{}) (interface{}, error) {
				a := params["a"].(float64)
				b := params["b"].(float64)
				op := params["op"].(string)

				var result float64
				switch op {
				case "add":
					result = a + b
				case "subtract":
					result = a - b
				case "multiply":
					result = a * b
				case "divide":
					result = a / b
				}

				return map[string]interface{}{
					"result": result,
				}, nil
			},
		},
	}

	mockProvider := CreateMockToolProvider(mockTools)

	// Create bridge server
	bridge := NewBridgeServer()

	// Create test directory for configs
	tmpDir := t.TempDir()

	// Create provider config
	providerFile := CreateTestProviderConfig(t, tmpDir, "test", "http", mockProvider.URL)

	// Add provider
	if err := bridge.AddProvider("test", providerFile); err != nil {
		t.Fatalf("Failed to add provider: %v", err)
	}

	// Discover tools
	if err := bridge.DiscoverTools(); err != nil {
		t.Fatalf("Failed to discover tools: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		mockProvider.Close()
	}

	return bridge, mockProvider, cleanup
}

// AssertToolExists checks if a tool exists in the bridge
func AssertToolExists(t *testing.T, bridge *BridgeServer, toolName string) {
	bridge.toolsMutex.RLock()
	defer bridge.toolsMutex.RUnlock()

	if _, exists := bridge.tools[toolName]; !exists {
		t.Errorf("Tool %s does not exist in bridge", toolName)
	}
}

// AssertToolCount checks the number of tools in the bridge
func AssertToolCount(t *testing.T, bridge *BridgeServer, expected int) {
	bridge.toolsMutex.RLock()
	defer bridge.toolsMutex.RUnlock()

	actual := len(bridge.tools)
	if actual != expected {
		t.Errorf("Expected %d tools, got %d", expected, actual)
	}
}

// CreateMockUTCPClient creates a mock UTCP client for testing
func CreateMockUTCPClient(tools []string, responses map[string]interface{}) interface{} {
	// This would require implementing a mock UTCP client
	// For now, return a simple mock
	return &MockUTCPClient{
		tools:       tools,
		callResults: responses,
		toolInfo:    make(map[string]interface{}),
	}
}

// TestProviderConfig represents a test provider configuration
type TestProviderConfig struct {
	Name     string
	Type     string
	URL      string
	Tools    []MockTool
	FailRate float64 // Probability of failure for chaos testing
}

// CreateChaosProvider creates a provider that randomly fails
func CreateChaosProvider(config TestProviderConfig) *httptest.Server {
	failCount := 0
	totalCount := 0
	mu := sync.Mutex{}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		totalCount++
		shouldFail := float64(failCount)/float64(totalCount) < config.FailRate
		if shouldFail {
			failCount++
		}
		mu.Unlock()

		if shouldFail {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Chaos monkey strikes!"))
			return
		}

		// Normal operation
		switch r.URL.Path {
		case "/tools":
			var toolList []map[string]interface{}
			for _, tool := range config.Tools {
				toolList = append(toolList, map[string]interface{}{
					"name":        tool.Name,
					"description": tool.Description,
				})
			}
			json.NewEncoder(w).Encode(toolList)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return httptest.NewServer(mux)
}

// WaitForCondition waits for a condition to be true or times out
func WaitForCondition(t *testing.T, condition func() bool, timeout, interval time.Duration, message string) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(interval)
	}

	t.Fatalf("Timeout waiting for condition: %s", message)
}

// CompareTools compares two tool slices for equality
func CompareTools(t *testing.T, expected, actual []*Tool) {
	if len(expected) != len(actual) {
		t.Errorf("Tool count mismatch: expected %d, got %d", len(expected), len(actual))
		return
	}

	expectedMap := make(map[string]*Tool)
	for _, tool := range expected {
		expectedMap[tool.Name] = tool
	}

	for _, tool := range actual {
		if expectedTool, exists := expectedMap[tool.Name]; exists {
			if tool.Description != expectedTool.Description ||
				tool.Provider != expectedTool.Provider {
				t.Errorf("Tool %s mismatch: expected %+v, got %+v", tool.Name, expectedTool, tool)
			}
		} else {
			t.Errorf("Unexpected tool: %s", tool.Name)
		}
	}
}
