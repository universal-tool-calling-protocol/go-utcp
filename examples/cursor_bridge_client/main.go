package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// CursorBridgeClient is a client for the Cursor UTCP Bridge
type CursorBridgeClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewCursorBridgeClient creates a new client
func NewCursorBridgeClient(baseURL string) *CursorBridgeClient {
	return &CursorBridgeClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Tool represents a tool in the bridge
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Provider    string      `json:"provider"`
	Schema      interface{} `json:"schema,omitempty"`
}

// ToolsResponse represents the response from listing tools
type ToolsResponse struct {
	Tools []*Tool `json:"tools"`
	Count int     `json:"count"`
}

// CallResponse represents the response from calling a tool
type CallResponse struct {
	Success  bool        `json:"success"`
	Result   interface{} `json:"result"`
	Error    string      `json:"error,omitempty"`
	Duration int64       `json:"duration"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Providers int    `json:"providers"`
	Tools     int    `json:"tools"`
	Timestamp int64  `json:"timestamp"`
}

// doRequest performs an HTTP request with authentication
func (c *CursorBridgeClient) doRequest(method, path string, body interface{}) (*http.Response, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
}

// SearchTools searches for tools by query
func (c *CursorBridgeClient) SearchTools(query string, limit int) (*ToolsResponse, error) {
	path := fmt.Sprintf("/api/v1/tools?query=%s&limit=%d", url.QueryEscape(query), limit)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, body)
	}

	var result ToolsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetTool gets information about a specific tool
func (c *CursorBridgeClient) GetTool(toolName string) (*Tool, error) {
	path := fmt.Sprintf("/api/v1/tools/%s", toolName)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, body)
	}

	var tool Tool
	if err := json.NewDecoder(resp.Body).Decode(&tool); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &tool, nil
}

// CallTool calls a tool with the given parameters
func (c *CursorBridgeClient) CallTool(ctx context.Context, toolName string, params map[string]interface{}) (*CallResponse, error) {
	request := map[string]interface{}{
		"tool":   toolName,
		"params": params,
	}

	resp, err := c.doRequest("POST", "/api/v1/tools/call", request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result CallResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// HealthCheck checks the health of the bridge server
func (c *CursorBridgeClient) HealthCheck() (*HealthResponse, error) {
	resp, err := c.doRequest("GET", "/health", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &health, nil
}

// RefreshTools refreshes the tool list on the server
func (c *CursorBridgeClient) RefreshTools() error {
	resp, err := c.doRequest("POST", "/api/v1/tools/refresh", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to refresh tools: %s", body)
	}

	return nil
}

func main() {
	var (
		baseURL  = flag.String("url", "http://localhost:8080", "UTCP bridge server URL")
		command  = flag.String("cmd", "", "Command to execute: list, search, info, call, health, refresh")
		query    = flag.String("query", "", "Search query for tools")
		toolName = flag.String("tool", "", "Tool name")
		input    = flag.String("input", "{}", "JSON input for tool call")
		limit    = flag.Int("limit", 10, "Maximum number of results")
		pretty   = flag.Bool("pretty", false, "Pretty print JSON output")
	)
	flag.Parse()

	// Override with environment variables if set
	if envURL := os.Getenv("CURSOR_UTCP_URL"); envURL != "" {
		*baseURL = envURL
	}

	client := NewCursorBridgeClient(*baseURL)

	// Helper function to print JSON
	printJSON := func(data interface{}) {
		var output []byte
		var err error
		if *pretty {
			output, err = json.MarshalIndent(data, "", "  ")
		} else {
			output, err = json.Marshal(data)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding output: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(output))
	}

	switch *command {
	case "list", "search":
		result, err := client.SearchTools(*query, *limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error searching tools: %v\n", err)
			os.Exit(1)
		}
		printJSON(result)

	case "info":
		if *toolName == "" {
			fmt.Fprintf(os.Stderr, "Error: tool name required for info command\n")
			os.Exit(1)
		}
		tool, err := client.GetTool(*toolName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting tool info: %v\n", err)
			os.Exit(1)
		}
		printJSON(tool)

	case "call":
		if *toolName == "" {
			fmt.Fprintf(os.Stderr, "Error: tool name required for call command\n")
			os.Exit(1)
		}

		var params map[string]interface{}
		if err := json.Unmarshal([]byte(*input), &params); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing input JSON: %v\n", err)
			os.Exit(1)
		}

		ctx := context.Background()
		result, err := client.CallTool(ctx, *toolName, params)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error calling tool: %v\n", err)
			os.Exit(1)
		}

		if !result.Success {
			fmt.Fprintf(os.Stderr, "Tool call failed: %s\n", result.Error)
			os.Exit(1)
		}

		printJSON(result)

	case "health":
		health, err := client.HealthCheck()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking health: %v\n", err)
			os.Exit(1)
		}
		printJSON(health)

	case "refresh":
		if err := client.RefreshTools(); err != nil {
			fmt.Fprintf(os.Stderr, "Error refreshing tools: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(`{"success": true, "message": "Tools refreshed successfully"}`)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", *command)
		fmt.Fprintf(os.Stderr, "Available commands: list, search, info, call, health, refresh\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -cmd list                          # List all tools\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd search -query http            # Search for HTTP tools\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd info -tool http.echo          # Get info about a tool\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd call -tool http.echo -input '{\"message\":\"hello\"}'\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd health                        # Check server health\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd refresh                       # Refresh tool list\n", os.Args[0])
		os.Exit(1)
	}
}
