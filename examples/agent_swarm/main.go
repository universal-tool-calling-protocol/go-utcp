package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	base "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"
	transports "github.com/universal-tool-calling-protocol/go-utcp/src/transports/http"
)

type location struct {
	ID       string
	Signal   float64
	Terrain  string
	Resource string
	Risk     float64
}

type evaluation struct {
	ID                string
	Score             float64
	Confidence        float64
	RecommendedAction string
	SupportAgents     []string
}

func main() {
	scoutServer := startScoutServer(":9081")
	defer shutdownServer("scout", scoutServer)

	analystServer := startAnalystServer(":9082")
	defer shutdownServer("analyst", analystServer)

	// Give the HTTP servers a moment to boot before running the swarm coordinator.
	time.Sleep(250 * time.Millisecond)

	logger := func(format string, args ...interface{}) {
		log.Printf(format, args...)
	}
	transport := transports.NewHttpClientTransport(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Discover tools from both worker agents.
	scoutManual := &providers.HttpProvider{
		BaseProvider: base.BaseProvider{Name: "scout-manual", ProviderType: base.ProviderHTTP},
		URL:          "http://localhost:9081/tools",
		HTTPMethod:   http.MethodGet,
		Headers:      map[string]string{"Accept": "application/json"},
	}
	scoutTools, err := transport.RegisterToolProvider(ctx, scoutManual)
	if err != nil {
		log.Fatalf("failed to register scout provider: %v", err)
	}
	log.Printf("Scout agent exposes %d tool(s)", len(scoutTools))
	for _, t := range scoutTools {
		log.Printf(" • %s — %s", t.Name, t.Description)
	}

	analystManual := &providers.HttpProvider{
		BaseProvider: base.BaseProvider{Name: "analyst-manual", ProviderType: base.ProviderHTTP},
		URL:          "http://localhost:9082/tools",
		HTTPMethod:   http.MethodGet,
		Headers:      map[string]string{"Accept": "application/json"},
	}
	analystTools, err := transport.RegisterToolProvider(ctx, analystManual)
	if err != nil {
		log.Fatalf("failed to register analyst provider: %v", err)
	}
	log.Printf("Analyst agent exposes %d tool(s)", len(analystTools))
	for _, t := range analystTools {
		log.Printf(" • %s — %s", t.Name, t.Description)
	}

	// Coordinate the swarm by chaining tool calls between the agents.
	plan, err := orchestrateSwarm(ctx, transport)
	if err != nil {
		log.Fatalf("swarm coordination failed: %v", err)
	}

	rendered, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		log.Fatalf("failed to render plan: %v", err)
	}
	fmt.Println("\nFinal swarm plan:")
	fmt.Println(string(rendered))
}

func orchestrateSwarm(ctx context.Context, transport *transports.HttpClientTransport) (map[string]any, error) {
	scoutCall := &providers.HttpProvider{
		BaseProvider: base.BaseProvider{Name: "scout-call", ProviderType: base.ProviderHTTP},
		URL:          "http://localhost:9081/tools/scan-terrain/call",
		HTTPMethod:   http.MethodPost,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
	}

	const region = "oasis-basin"
	scanResult, err := transport.CallTool(ctx, "scan-terrain", map[string]any{"region": region}, scoutCall, nil)
	if err != nil {
		return nil, fmt.Errorf("scout scan failed: %w", err)
	}

	locations, err := parseLocations(scanResult)
	if err != nil {
		return nil, err
	}
	if len(locations) == 0 {
		return nil, errors.New("scout agent did not discover any locations")
	}

	analystCall := &providers.HttpProvider{
		BaseProvider: base.BaseProvider{Name: "analyst-call", ProviderType: base.ProviderHTTP},
		URL:          "http://localhost:9082/tools/evaluate-location/call",
		HTTPMethod:   http.MethodPost,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
	}

	evaluations := make([]evaluation, 0, len(locations))
	var evalMu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	for _, loc := range locations {
		loc := loc
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := map[string]any{"location": map[string]any{
				"id":       loc.ID,
				"signal":   loc.Signal,
				"terrain":  loc.Terrain,
				"resource": loc.Resource,
				"risk":     loc.Risk,
			}}
			res, err := transport.CallTool(ctx, "evaluate-location", payload, analystCall, nil)
			if err != nil {
				errOnce.Do(func() { firstErr = fmt.Errorf("analyst evaluation failed for %s: %w", loc.ID, err) })
				return
			}
			eval, err := parseEvaluation(res)
			if err != nil {
				errOnce.Do(func() { firstErr = fmt.Errorf("invalid evaluation for %s: %w", loc.ID, err) })
				return
			}

			evalMu.Lock()
			evaluations = append(evaluations, eval)
			evalMu.Unlock()
		}()
	}

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	if len(evaluations) == 0 {
		return nil, errors.New("no evaluations returned by analyst agent")
	}

	sort.SliceStable(evaluations, func(i, j int) bool {
		return evaluations[i].Score > evaluations[j].Score
	})

	topTargets := evaluations
	if len(topTargets) > 2 {
		topTargets = topTargets[:2]
	}

	directives := []string{}
	if len(topTargets) > 0 {
		directives = append(directives, fmt.Sprintf("Deploy harvesters to %s with action '%s' (score %.2f, confidence %.2f)", topTargets[0].ID, topTargets[0].RecommendedAction, topTargets[0].Score, topTargets[0].Confidence))
	}
	if len(topTargets) > 1 {
		directives = append(directives, fmt.Sprintf("Assign reconnaissance drones to %s for follow-up sweeps", topTargets[1].ID))
	}
	directives = append(directives, "Synchronize agent telemetry every 30s and rebalance squads if signal strength drops")

	selected := make([]map[string]any, 0, len(topTargets))
	for _, t := range topTargets {
		selected = append(selected, map[string]any{
			"id":                 t.ID,
			"score":              round2(t.Score),
			"confidence":         round2(t.Confidence),
			"recommended_action": t.RecommendedAction,
			"support_agents":     t.SupportAgents,
		})
	}

	plan := map[string]any{
		"region":           region,
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
		"selected_targets": selected,
		"swarm_directives": directives,
		"participating_agents": []string{
			"scout-delta", "analyst-omega", "coordinator-hub",
		},
	}

	return plan, nil
}

func parseLocations(res any) ([]location, error) {
	payload, ok := res.(map[string]any)
	if !ok {
		return nil, errors.New("unexpected response from scout agent")
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		return nil, errors.New("scout agent response missing result payload")
	}
	rawLocations, ok := result["locations"].([]any)
	if !ok {
		return nil, errors.New("scout agent returned locations in unexpected format")
	}

	var locations []location
	for _, raw := range rawLocations {
		locMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := locMap["id"].(string)
		terrain, _ := locMap["terrain"].(string)
		resource, _ := locMap["resource"].(string)

		loc := location{
			ID:       id,
			Signal:   getFloat(locMap["signal"]),
			Terrain:  terrain,
			Resource: resource,
			Risk:     getFloat(locMap["risk"]),
		}
		locations = append(locations, loc)
	}

	return locations, nil
}

func parseEvaluation(res any) (evaluation, error) {
	payload, ok := res.(map[string]any)
	if !ok {
		return evaluation{}, errors.New("unexpected response type from analyst agent")
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		return evaluation{}, errors.New("analyst response missing result payload")
	}

	id, _ := result["id"].(string)
	action, _ := result["recommended_action"].(string)
	support := []string{}
	if rawSupport, ok := result["support_agents"].([]any); ok {
		for _, item := range rawSupport {
			if name, ok := item.(string); ok {
				support = append(support, name)
			}
		}
	}

	eval := evaluation{
		ID:                id,
		Score:             getFloat(result["score"]),
		Confidence:        getFloat(result["confidence"]),
		RecommendedAction: action,
		SupportAgents:     support,
	}
	return eval, nil
}

func getFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	default:
		return 0
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func startScoutServer(addr string) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, map[string]any{
			"version": "1.0",
			"tools": []map[string]any{
				{
					"name":        "scan-terrain",
					"description": "Survey a region and report back interesting resource signals.",
					"input_schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"region": map[string]any{
								"type":        "string",
								"description": "Region identifier to scan",
							},
						},
					},
				},
			},
		})
	})

	mux.HandleFunc("/tools/scan-terrain/call", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Region string `json:"region"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		region := req.Region
		if region == "" {
			region = "unknown"
		}

		locations := []map[string]any{
			{
				"id":       "oasis-alpha",
				"signal":   0.91,
				"terrain":  "oasis",
				"resource": "water",
				"risk":     0.12,
			},
			{
				"id":       "ridge-sierra",
				"signal":   0.82,
				"terrain":  "ridge",
				"resource": "minerals",
				"risk":     0.28,
			},
			{
				"id":       "canyon-echo",
				"signal":   0.74,
				"terrain":  "canyon",
				"resource": "energy",
				"risk":     0.35,
			},
		}

		respondJSON(w, map[string]any{
			"result": map[string]any{
				"region":    region,
				"locations": locations,
				"summary":   fmt.Sprintf("Detected %d promising signals in %s", len(locations), region),
			},
		})
	})

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Scout agent listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("scout server error: %v", err)
		}
	}()

	return srv
}

func startAnalystServer(addr string) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, map[string]any{
			"version": "1.0",
			"tools": []map[string]any{
				{
					"name":        "evaluate-location",
					"description": "Score a scanned location and propose the best swarm action.",
					"input_schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "object",
								"description": "The location payload from the scout agent.",
							},
						},
					},
				},
			},
		})
	})

	mux.HandleFunc("/tools/evaluate-location/call", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Location map[string]any `json:"location"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
		location := req.Location
		id, _ := location["id"].(string)
		signal := getFloat(location["signal"])
		risk := getFloat(location["risk"])
		resource, _ := location["resource"].(string)

		baseScore := 0.6*signal + 0.25*(1-risk)
		if resource == "water" {
			baseScore += 0.08
		}
		if resource == "energy" {
			baseScore += 0.04
		}
		score := math.Min(0.99, math.Max(0.0, baseScore))

		confidence := math.Min(0.95, 0.55+signal*0.35-risk*0.15)

		action := "survey"
		switch {
		case score >= 0.85:
			action = "harvest"
		case score >= 0.7:
			action = "deploy"
		case risk >= 0.4:
			action = "monitor"
		}

		support := []string{"drone-a1", "bot-b4"}
		if action == "harvest" {
			support = []string{"harvester-h3", "guardian-g2"}
		}

		respondJSON(w, map[string]any{
			"result": map[string]any{
				"id":                 id,
				"score":              score,
				"confidence":         confidence,
				"recommended_action": action,
				"support_agents":     support,
			},
		})
	})

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Analyst agent listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("analyst server error: %v", err)
		}
	}()

	return srv
}

func respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func shutdownServer(name string, srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		log.Printf("error shutting down %s server: %v", name, err)
	}
}
