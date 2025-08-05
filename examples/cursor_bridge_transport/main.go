package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	// TODO: Uncomment when UTCP client API is updated
	// utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

// Tool represents a discovered tool
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Provider    string      `json:"provider"`
	Schema      interface{} `json:"schema,omitempty"`
}

// BridgeServer aggregates tools from multiple UTCP providers
type BridgeServer struct {
	providers    map[string]interface{} // Will be replaced with actual UTCP client
	tools        map[string]*Tool
	toolsMutex   sync.RWMutex
	providerUrls map[string]string
}

// NewBridgeServer creates a new bridge server
func NewBridgeServer() *BridgeServer {
	return &BridgeServer{
		providers:    make(map[string]interface{}),
		tools:        make(map[string]*Tool),
		providerUrls: make(map[string]string),
	}
}

// AddProvider adds a UTCP provider to the bridge
func (bs *BridgeServer) AddProvider(name, providerFile string) error {
	// Read provider configuration
	providerData, err := os.ReadFile(providerFile)
	if err != nil {
		return fmt.Errorf("failed to read provider file: %w", err)
	}

	var providerConfig map[string]interface{}
	if err := json.Unmarshal(providerData, &providerConfig); err != nil {
		return fmt.Errorf("failed to parse provider config: %w", err)
	}

	// Extract URL from provider config
	if url, ok := providerConfig["url"].(string); ok {
		bs.providerUrls[name] = url
	}

	// TODO: Replace with actual UTCP client initialization
	// For now, just store the provider config
	bs.providers[name] = providerConfig

	log.Printf("Added provider %s from %s", name, providerFile)
	return nil
}

// DiscoverTools discovers all tools from registered providers
func (bs *BridgeServer) DiscoverTools() error {
	bs.toolsMutex.Lock()
	defer bs.toolsMutex.Unlock()

	// Clear existing tools
	bs.tools = make(map[string]*Tool)

	// TODO: Replace with actual UTCP discovery
	// For now, create mock tools based on provider type
	for providerName, providerInterface := range bs.providers {
		config, ok := providerInterface.(map[string]interface{})
		if !ok {
			continue
		}

		providerType, _ := config["type"].(string)

		// Create mock tools based on provider type
		switch providerType {
		case "http":
			bs.tools[providerName+".echo"] = &Tool{
				Name:        providerName + ".echo",
				Description: "Echo service from " + providerName,
				Provider:    providerName,
			}
			bs.tools[providerName+".status"] = &Tool{
				Name:        providerName + ".status",
				Description: "Status check from " + providerName,
				Provider:    providerName,
			}
		case "cli":
			if tools, ok := config["tools"].([]interface{}); ok {
				for _, tool := range tools {
					if t, ok := tool.(map[string]interface{}); ok {
						name, _ := t["name"].(string)
						desc, _ := t["description"].(string)
						if name != "" {
							fullName := providerName + "." + name
							bs.tools[fullName] = &Tool{
								Name:        fullName,
								Description: desc,
								Provider:    providerName,
								Schema:      t,
							}
						}
					}
				}
			}
		}
	}

	log.Printf("Discovered %d tools across %d providers", len(bs.tools), len(bs.providers))
	return nil
}

// CallTool executes a tool through the appropriate provider
func (bs *BridgeServer) CallTool(ctx context.Context, toolName string, params map[string]interface{}) (interface{}, error) {
	// Parse provider from tool name
	parts := strings.SplitN(toolName, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid tool name format, expected: provider.toolname")
	}

	providerName := parts[0]
	actualToolName := parts[1]

	// Get the provider
	_, exists := bs.providers[providerName]
	if !exists {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	// TODO: Replace with actual UTCP tool call
	// For now, return mock response
	mockResponse := map[string]interface{}{
		"result":    fmt.Sprintf("Mock execution of %s via %s provider", actualToolName, providerName),
		"params":    params,
		"timestamp": time.Now().Unix(),
	}

	return mockResponse, nil
}

// HTTP Handlers

func (bs *BridgeServer) healthHandler(c *gin.Context) {
	status := gin.H{
		"status":    "healthy",
		"providers": len(bs.providers),
		"tools":     len(bs.tools),
		"timestamp": time.Now().Unix(),
	}
	c.JSON(http.StatusOK, status)
}

func (bs *BridgeServer) listToolsHandler(c *gin.Context) {
	query := c.Query("query")
	limit := 100

	bs.toolsMutex.RLock()
	defer bs.toolsMutex.RUnlock()

	results := make([]*Tool, 0)
	for _, tool := range bs.tools {
		// Simple search filter
		if query != "" && !strings.Contains(strings.ToLower(tool.Name), strings.ToLower(query)) &&
			!strings.Contains(strings.ToLower(tool.Description), strings.ToLower(query)) {
			continue
		}
		results = append(results, tool)
		if len(results) >= limit {
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tools": results,
		"count": len(results),
	})
}

func (bs *BridgeServer) getToolHandler(c *gin.Context) {
	toolName := c.Param("name")

	bs.toolsMutex.RLock()
	tool, exists := bs.tools[toolName]
	bs.toolsMutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}

	c.JSON(http.StatusOK, tool)
}

func (bs *BridgeServer) callToolHandler(c *gin.Context) {
	var request struct {
		Tool   string                 `json:"tool" binding:"required"`
		Params map[string]interface{} `json:"params"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Execute tool call
	ctx := c.Request.Context()
	result, err := bs.CallTool(ctx, request.Tool, request.Params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"result":   result,
		"duration": time.Now().Unix(),
	})
}

func (bs *BridgeServer) refreshToolsHandler(c *gin.Context) {
	if err := bs.DiscoverTools(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Tools refreshed successfully"})
}

func main() {
	// Create bridge server
	bridge := NewBridgeServer()

	// Add providers from configuration files
	// Example: Add HTTP provider
	if _, err := os.Stat("provider-http.json"); err == nil {
		if err := bridge.AddProvider("http", "provider-http.json"); err != nil {
			log.Printf("Failed to add HTTP provider: %v", err)
		}
	}

	// Example: Add CLI provider
	if _, err := os.Stat("provider-cli.json"); err == nil {
		if err := bridge.AddProvider("cli", "provider-cli.json"); err != nil {
			log.Printf("Failed to add CLI provider: %v", err)
		}
	}

	// Example: Add MCP provider
	if _, err := os.Stat("provider-mcp.json"); err == nil {
		if err := bridge.AddProvider("mcp", "provider-mcp.json"); err != nil {
			log.Printf("Failed to add MCP provider: %v", err)
		}
	}

	// Initial tool discovery
	if err := bridge.DiscoverTools(); err != nil {
		log.Printf("Initial tool discovery failed: %v", err)
	}

	// Start periodic discovery
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := bridge.DiscoverTools(); err != nil {
				log.Printf("Periodic tool discovery failed: %v", err)
			}
		}
	}()

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// Setup routes
	router.GET("/health", bridge.healthHandler)
	router.GET("/api/v1/tools", bridge.listToolsHandler)
	router.GET("/api/v1/tools/:name", bridge.getToolHandler)
	router.POST("/api/v1/tools/call", bridge.callToolHandler)
	router.POST("/api/v1/tools/refresh", bridge.refreshToolsHandler)

	// Root route
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service":     "cursor-utcp-bridge-example",
			"version":     "1.0.0",
			"description": "Example UTCP Bridge that aggregates multiple providers",
			"endpoints": gin.H{
				"health": "/health",
				"tools":  "/api/v1/tools",
				"call":   "/api/v1/tools/call",
			},
		})
	})

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Cursor UTCP Bridge on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
