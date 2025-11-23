package codemode

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

func TestToolCache_ToolSpecs(t *testing.T) {
	cache := NewToolCache()

	// Test cache miss
	specs := cache.GetToolSpecs()
	if specs != nil {
		t.Error("Expected nil for empty cache")
	}

	// Set tool specs
	testSpecs := []tools.Tool{
		{Name: "test.tool1", Description: "Test tool 1"},
		{Name: "test.tool2", Description: "Test tool 2"},
	}
	cache.SetToolSpecs(testSpecs)

	// Test cache hit
	cached := cache.GetToolSpecs()
	if cached == nil {
		t.Fatal("Expected cached specs, got nil")
	}
	if len(cached) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(cached))
	}

	// Verify copy semantics - modifying returned value shouldn't affect cache
	cached[0].Name = "modified"
	cached2 := cache.GetToolSpecs()
	if cached2[0].Name == "modified" {
		t.Error("Cache returned same slice instead of copy")
	}
}

func TestToolCache_ToolSpecs_Expiration(t *testing.T) {
	// Set very short TTL
	os.Setenv("UTCP_TOOL_SPECS_CACHE_TTL", "100ms")
	defer os.Unsetenv("UTCP_TOOL_SPECS_CACHE_TTL")

	cache := NewToolCache()

	testSpecs := []tools.Tool{
		{Name: "test.tool1", Description: "Test tool 1"},
	}
	cache.SetToolSpecs(testSpecs)

	// Should be cached immediately
	if cache.GetToolSpecs() == nil {
		t.Error("Expected cached value immediately after set")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	if cache.GetToolSpecs() != nil {
		t.Error("Expected nil after TTL expiration")
	}
}

func TestToolCache_SelectedTools(t *testing.T) {
	cache := NewToolCache()

	query := "find memory tools"
	availableTools := "tool1, tool2, tool3"

	// Test cache miss
	selected := cache.GetSelectedTools(query, availableTools)
	if selected != nil {
		t.Error("Expected nil for empty cache")
	}

	// Set selection
	testSelection := []string{"tool1", "tool3"}
	cache.SetSelectedTools(query, availableTools, testSelection)

	// Test cache hit
	cached := cache.GetSelectedTools(query, availableTools)
	if cached == nil {
		t.Fatal("Expected cached selection, got nil")
	}
	if len(cached) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(cached))
	}

	// Verify copy semantics
	cached[0] = "modified"
	cached2 := cache.GetSelectedTools(query, availableTools)
	if cached2[0] == "modified" {
		t.Error("Cache returned same slice instead of copy")
	}
}

func TestToolCache_SelectedTools_DifferentQueries(t *testing.T) {
	cache := NewToolCache()

	availableTools := "tool1, tool2, tool3"

	// Cache different selections for different queries
	cache.SetSelectedTools("query1", availableTools, []string{"tool1"})
	cache.SetSelectedTools("query2", availableTools, []string{"tool2"})

	// Verify separation
	sel1 := cache.GetSelectedTools("query1", availableTools)
	sel2 := cache.GetSelectedTools("query2", availableTools)

	if len(sel1) != 1 || sel1[0] != "tool1" {
		t.Error("Query1 returned wrong selection")
	}
	if len(sel2) != 1 || sel2[0] != "tool2" {
		t.Error("Query2 returned wrong selection")
	}
}

func TestToolCache_SelectedTools_DifferentAvailableTools(t *testing.T) {
	cache := NewToolCache()

	query := "find tools"

	// Cache selections with different available tools
	cache.SetSelectedTools(query, "tool1, tool2", []string{"tool1"})
	cache.SetSelectedTools(query, "tool3, tool4", []string{"tool3"})

	// Verify separation based on available tools
	sel1 := cache.GetSelectedTools(query, "tool1, tool2")
	sel2 := cache.GetSelectedTools(query, "tool3, tool4")

	if len(sel1) != 1 || sel1[0] != "tool1" {
		t.Error("First available tools set returned wrong selection")
	}
	if len(sel2) != 1 || sel2[0] != "tool3" {
		t.Error("Second available tools set returned wrong selection")
	}
}

func TestToolCache_SelectedTools_Expiration(t *testing.T) {
	// Set very short TTL
	os.Setenv("UTCP_TOOL_SELECTION_CACHE_TTL", "100ms")
	defer os.Unsetenv("UTCP_TOOL_SELECTION_CACHE_TTL")

	cache := NewToolCache()

	query := "find tools"
	availableTools := "tool1, tool2"
	cache.SetSelectedTools(query, availableTools, []string{"tool1"})

	// Should be cached immediately
	if cache.GetSelectedTools(query, availableTools) == nil {
		t.Error("Expected cached value immediately after set")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	if cache.GetSelectedTools(query, availableTools) != nil {
		t.Error("Expected nil after TTL expiration")
	}
}

func TestToolCache_Invalidation(t *testing.T) {
	cache := NewToolCache()

	// Set up cache
	testSpecs := []tools.Tool{{Name: "test.tool1"}}
	cache.SetToolSpecs(testSpecs)
	cache.SetSelectedTools("query", "tools", []string{"tool1"})

	// Test InvalidateToolSpecs
	cache.InvalidateToolSpecs()
	if cache.GetToolSpecs() != nil {
		t.Error("Tool specs should be invalidated")
	}

	// Set up again
	cache.SetToolSpecs(testSpecs)
	cache.SetSelectedTools("query", "tools", []string{"tool1"})

	// Test InvalidateSelections
	cache.InvalidateSelections()
	if cache.GetSelectedTools("query", "tools") != nil {
		t.Error("Selections should be invalidated")
	}
	if cache.GetToolSpecs() == nil {
		t.Error("Tool specs should still be cached")
	}

	// Test InvalidateAll
	cache.SetSelectedTools("query", "tools", []string{"tool1"})
	cache.InvalidateAll()
	if cache.GetToolSpecs() != nil {
		t.Error("Tool specs should be invalidated")
	}
	if cache.GetSelectedTools("query", "tools") != nil {
		t.Error("Selections should be invalidated")
	}
}

func TestToolCache_Stats(t *testing.T) {
	cache := NewToolCache()

	// Initial stats should be zero
	stats := cache.Stats()
	if stats.SpecsHits != 0 || stats.SpecsMisses != 0 {
		t.Error("Initial stats should be zero")
	}

	// Cache miss
	cache.GetToolSpecs()
	stats = cache.Stats()
	if stats.SpecsMisses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.SpecsMisses)
	}

	// Cache hit
	cache.SetToolSpecs([]tools.Tool{{Name: "test"}})
	cache.GetToolSpecs()
	cache.GetToolSpecs()
	stats = cache.Stats()
	if stats.SpecsHits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.SpecsHits)
	}
	if stats.SpecsMisses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.SpecsMisses)
	}

	// Test hit rate
	hitRate := stats.SpecsHitRate()
	expectedRate := 2.0 / 3.0
	if hitRate < expectedRate-0.01 || hitRate > expectedRate+0.01 {
		t.Errorf("Expected hit rate ~%.2f, got %.2f", expectedRate, hitRate)
	}
}

func TestToolCache_SelectionStats(t *testing.T) {
	cache := NewToolCache()

	query := "test query"
	tools := "tool1, tool2"

	// Miss
	cache.GetSelectedTools(query, tools)
	stats := cache.Stats()
	if stats.SelectionMisses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.SelectionMisses)
	}

	// Hits
	cache.SetSelectedTools(query, tools, []string{"tool1"})
	cache.GetSelectedTools(query, tools)
	cache.GetSelectedTools(query, tools)

	stats = cache.Stats()
	if stats.SelectionHits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.SelectionHits)
	}

	hitRate := stats.SelectionHitRate()
	expectedRate := 2.0 / 3.0
	if hitRate < expectedRate-0.01 || hitRate > expectedRate+0.01 {
		t.Errorf("Expected hit rate ~%.2f, got %.2f", expectedRate, hitRate)
	}
}

func TestToolCache_CleanExpired(t *testing.T) {
	// Set very short TTL
	os.Setenv("UTCP_TOOL_SELECTION_CACHE_TTL", "50ms")
	defer os.Unsetenv("UTCP_TOOL_SELECTION_CACHE_TTL")

	cache := NewToolCache()

	// Add multiple entries
	for i := 0; i < 5; i++ {
		cache.SetSelectedTools("query"+string(rune(i)), "tools", []string{"tool1"})
	}

	stats := cache.Stats()
	if stats.SelectionSize != 5 {
		t.Errorf("Expected 5 entries, got %d", stats.SelectionSize)
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Clean expired
	cache.CleanExpired()

	stats = cache.Stats()
	if stats.SelectionSize != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", stats.SelectionSize)
	}
}

func TestToolCache_CleanupRoutine(t *testing.T) {
	// Set very short TTL
	os.Setenv("UTCP_TOOL_SELECTION_CACHE_TTL", "50ms")
	defer os.Unsetenv("UTCP_TOOL_SELECTION_CACHE_TTL")

	cache := NewToolCache()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start cleanup routine with short interval
	cache.StartCleanupRoutine(ctx, 75*time.Millisecond)

	// Add entries
	for i := 0; i < 3; i++ {
		cache.SetSelectedTools("query"+string(rune(i)), "tools", []string{"tool1"})
	}

	// Wait for cleanup to run
	time.Sleep(200 * time.Millisecond)

	stats := cache.Stats()
	if stats.SelectionSize != 0 {
		t.Errorf("Expected 0 entries after automatic cleanup, got %d", stats.SelectionSize)
	}

	// Cancel context and ensure routine stops
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestToolCache_ConcurrentAccess(t *testing.T) {
	cache := NewToolCache()
	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(n int) {
			cache.SetToolSpecs([]tools.Tool{{Name: "tool" + string(rune(n))}})
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			_ = cache.GetToolSpecs()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// If we get here without deadlock, test passes
}

func TestParseDurationFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		defaultDur  time.Duration
		expectedDur time.Duration
	}{
		{"empty", "", 5 * time.Minute, 5 * time.Minute},
		{"duration string", "10s", 5 * time.Minute, 10 * time.Second},
		{"minutes", "2m", 5 * time.Minute, 2 * time.Minute},
		{"hours", "1h", 5 * time.Minute, 1 * time.Hour},
		{"seconds as int", "30", 5 * time.Minute, 30 * time.Second},
		{"invalid", "invalid", 5 * time.Minute, 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVar := "TEST_DURATION_VAR"
			if tt.envValue != "" {
				os.Setenv(envVar, tt.envValue)
				defer os.Unsetenv(envVar)
			}

			result := parseDurationFromEnv(envVar, tt.defaultDur)
			if result != tt.expectedDur {
				t.Errorf("Expected %v, got %v", tt.expectedDur, result)
			}
		})
	}
}

func BenchmarkToolCache_GetToolSpecs(b *testing.B) {
	cache := NewToolCache()
	testSpecs := make([]tools.Tool, 50)
	for i := 0; i < 50; i++ {
		testSpecs[i] = tools.Tool{Name: "tool" + string(rune(i))}
	}
	cache.SetToolSpecs(testSpecs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.GetToolSpecs()
	}
}

func BenchmarkToolCache_SetToolSpecs(b *testing.B) {
	cache := NewToolCache()
	testSpecs := make([]tools.Tool, 50)
	for i := 0; i < 50; i++ {
		testSpecs[i] = tools.Tool{Name: "tool" + string(rune(i))}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetToolSpecs(testSpecs)
	}
}

func BenchmarkToolCache_GetSelectedTools(b *testing.B) {
	cache := NewToolCache()
	query := "find memory tools"
	tools := "tool1, tool2, tool3, tool4, tool5"
	cache.SetSelectedTools(query, tools, []string{"tool1", "tool3"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.GetSelectedTools(query, tools)
	}
}

func BenchmarkToolCache_SetSelectedTools(b *testing.B) {
	cache := NewToolCache()
	query := "find memory tools"
	tools := "tool1, tool2, tool3, tool4, tool5"
	selection := []string{"tool1", "tool3"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetSelectedTools(query, tools, selection)
	}
}
