package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	utcp "github.com/Raezil/UTCP"
)

// LaunchService interface for dependency injection
type LaunchService interface {
	GetLaunches(ctx context.Context, limit string) (interface{}, error)
}

// GraphQLLaunchService implements LaunchService using GraphQL providers
type GraphQLLaunchService struct {
	transport *utcp.GraphQLClientTransport
	primary   *utcp.GraphQLProvider
	fallback  *utcp.GraphQLProvider
}

func NewGraphQLLaunchService(transport *utcp.GraphQLClientTransport, primary, fallback *utcp.GraphQLProvider) *GraphQLLaunchService {
	return &GraphQLLaunchService{
		transport: transport,
		primary:   primary,
		fallback:  fallback,
	}
}

func (s *GraphQLLaunchService) GetLaunches(ctx context.Context, limit string) (interface{}, error) {
	if limit == "" {
		limit = "3"
	}
	args := map[string]interface{}{"limit": limit}

	// Try primary provider first
	res, err := s.transport.CallTool(ctx, "launchesPast", args, s.primary, nil)
	if err == nil {
		return res, nil
	}
	log.Printf("warning: primary provider failed: %v", err)

	// Fall back to public SpaceX API
	res, err = s.transport.CallTool(ctx, "launchesPast", args, s.fallback, nil)
	if err != nil {
		return nil, fmt.Errorf("both primary and fallback providers failed: %w", err)
	}
	return res, nil
}

// MockLaunchService for testing
type MockLaunchService struct {
	shouldFail bool
	response   interface{}
}

func (m *MockLaunchService) GetLaunches(ctx context.Context, limit string) (interface{}, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock service failure")
	}

	// Return mock data based on limit
	if limit == "1" {
		return map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{
					"mission_name": "Test Mission 1",
					"launch_date":  "2024-01-01",
				},
			},
		}, nil
	}

	return map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{
				"mission_name": "Test Mission 1",
				"launch_date":  "2024-01-01",
			},
			map[string]interface{}{
				"mission_name": "Test Mission 2",
				"launch_date":  "2024-01-02",
			},
			map[string]interface{}{
				"mission_name": "Test Mission 3",
				"launch_date":  "2024-01-03",
			},
		},
	}, nil
}

// Global service instance (for dependency injection)
var launchService LaunchService

func launchesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := r.URL.Query().Get("limit")

	data, err := launchService.GetLaunches(ctx, limit)
	if err != nil {
		log.Printf("error fetching launches: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Service temporarily unavailable"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func main() {
	logger := func(msg string, err error) {
		if err != nil {
			log.Printf("[GraphQLTransport] %s: %v", msg, err)
		} else {
			log.Printf("[GraphQLTransport] %s", msg)
		}
	}

	transport := utcp.NewGraphQLClientTransport(logger)
	defer transport.Close()

	primary := &utcp.GraphQLProvider{
		URL:     "http://localhost:8080/graphql",
		Headers: map[string]string{"Content-Type": "application/json"},
	}

	// Fixed fallback URL - should be different from primary
	fallback := &utcp.GraphQLProvider{
		URL:     "https://api.spacex.land/graphql/", // Real SpaceX API endpoint
		Headers: map[string]string{"Content-Type": "application/json"},
	}

	// Initialize the service
	launchService = NewGraphQLLaunchService(transport, primary, fallback)

	// Example direct call
	ctx := context.Background()
	res, err := launchService.GetLaunches(ctx, "2")
	if err != nil {
		log.Printf("error fetching launches: %v", err)
	} else {
		fmt.Printf("Direct call result: %+v\n", res)
	}

	http.HandleFunc("/launches", launchesHandler)

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
