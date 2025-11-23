package codemode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// ToolCache provides thread-safe caching for tool specs and selection results
type ToolCache struct {
	// Tool specs cache
	toolSpecsMu    sync.RWMutex
	toolSpecsCache []tools.Tool
	toolSpecsTime  time.Time
	toolSpecsTTL   time.Duration

	// Tool selection cache (query -> selected tools)
	selectionMu    sync.RWMutex
	selectionCache map[string]*selectionCacheEntry
	selectionTTL   time.Duration

	// Stats for monitoring
	statsMu         sync.RWMutex
	specsHits       int64
	specsMisses     int64
	selectionHits   int64
	selectionMisses int64
}

type selectionCacheEntry struct {
	tools     []string
	timestamp time.Time
}

// NewToolCache creates a new tool cache with configurable TTLs
func NewToolCache() *ToolCache {
	// Default: cache tool specs for 5 minutes
	specsTTL := parseDurationFromEnv("UTCP_TOOL_SPECS_CACHE_TTL", 5*time.Minute)

	// Default: cache tool selections for 2 minutes
	selectionTTL := parseDurationFromEnv("UTCP_TOOL_SELECTION_CACHE_TTL", 2*time.Minute)

	return &ToolCache{
		toolSpecsTTL:   specsTTL,
		selectionTTL:   selectionTTL,
		selectionCache: make(map[string]*selectionCacheEntry),
	}
}

// GetToolSpecs retrieves cached tool specs or returns nil if expired/missing
func (tc *ToolCache) GetToolSpecs() []tools.Tool {
	tc.toolSpecsMu.RLock()
	defer tc.toolSpecsMu.RUnlock()

	if time.Since(tc.toolSpecsTime) > tc.toolSpecsTTL {
		tc.statsMu.Lock()
		tc.specsMisses++
		tc.statsMu.Unlock()
		return nil
	}

	if tc.toolSpecsCache == nil {
		tc.statsMu.Lock()
		tc.specsMisses++
		tc.statsMu.Unlock()
		return nil
	}

	tc.statsMu.Lock()
	tc.specsHits++
	tc.statsMu.Unlock()

	// Return a copy to prevent external modifications
	result := make([]tools.Tool, len(tc.toolSpecsCache))
	copy(result, tc.toolSpecsCache)
	return result
}

// SetToolSpecs stores tool specs in cache
func (tc *ToolCache) SetToolSpecs(specs []tools.Tool) {
	tc.toolSpecsMu.Lock()
	defer tc.toolSpecsMu.Unlock()

	// Store a copy to prevent external modifications
	tc.toolSpecsCache = make([]tools.Tool, len(specs))
	copy(tc.toolSpecsCache, specs)
	tc.toolSpecsTime = time.Now()
}

// GetSelectedTools retrieves cached tool selection for a query
func (tc *ToolCache) GetSelectedTools(query string, availableTools string) []string {
	key := tc.cacheKey(query, availableTools)

	tc.selectionMu.RLock()
	entry, exists := tc.selectionCache[key]
	tc.selectionMu.RUnlock()

	if !exists {
		tc.statsMu.Lock()
		tc.selectionMisses++
		tc.statsMu.Unlock()
		return nil
	}

	if time.Since(entry.timestamp) > tc.selectionTTL {
		// Expired - clean up in background
		go tc.removeExpiredSelection(key)
		tc.statsMu.Lock()
		tc.selectionMisses++
		tc.statsMu.Unlock()
		return nil
	}

	tc.statsMu.Lock()
	tc.selectionHits++
	tc.statsMu.Unlock()

	// Return a copy to prevent external modifications
	result := make([]string, len(entry.tools))
	copy(result, entry.tools)
	return result
}

// SetSelectedTools stores tool selection result in cache
func (tc *ToolCache) SetSelectedTools(query string, availableTools string, selectedTools []string) {
	key := tc.cacheKey(query, availableTools)

	tc.selectionMu.Lock()
	defer tc.selectionMu.Unlock()

	// Store a copy to prevent external modifications
	toolsCopy := make([]string, len(selectedTools))
	copy(toolsCopy, selectedTools)

	tc.selectionCache[key] = &selectionCacheEntry{
		tools:     toolsCopy,
		timestamp: time.Now(),
	}
}

// InvalidateToolSpecs clears the tool specs cache
func (tc *ToolCache) InvalidateToolSpecs() {
	tc.toolSpecsMu.Lock()
	defer tc.toolSpecsMu.Unlock()

	tc.toolSpecsCache = nil
	tc.toolSpecsTime = time.Time{}
}

// InvalidateSelections clears all tool selection cache entries
func (tc *ToolCache) InvalidateSelections() {
	tc.selectionMu.Lock()
	defer tc.selectionMu.Unlock()

	tc.selectionCache = make(map[string]*selectionCacheEntry)
}

// InvalidateAll clears all caches
func (tc *ToolCache) InvalidateAll() {
	tc.InvalidateToolSpecs()
	tc.InvalidateSelections()
}

// CleanExpired removes expired entries from selection cache
func (tc *ToolCache) CleanExpired() {
	tc.selectionMu.Lock()
	defer tc.selectionMu.Unlock()

	now := time.Now()
	for key, entry := range tc.selectionCache {
		if now.Sub(entry.timestamp) > tc.selectionTTL {
			delete(tc.selectionCache, key)
		}
	}
}

// Stats returns cache performance statistics
func (tc *ToolCache) Stats() CacheStats {
	tc.statsMu.RLock()
	defer tc.statsMu.RUnlock()

	return CacheStats{
		SpecsHits:       tc.specsHits,
		SpecsMisses:     tc.specsMisses,
		SelectionHits:   tc.selectionHits,
		SelectionMisses: tc.selectionMisses,
		SelectionSize:   tc.getSelectionCacheSize(),
	}
}

// CacheStats holds cache performance metrics
type CacheStats struct {
	SpecsHits       int64
	SpecsMisses     int64
	SelectionHits   int64
	SelectionMisses int64
	SelectionSize   int
}

// HitRate returns the cache hit rate for tool specs
func (cs CacheStats) SpecsHitRate() float64 {
	total := cs.SpecsHits + cs.SpecsMisses
	if total == 0 {
		return 0.0
	}
	return float64(cs.SpecsHits) / float64(total)
}

// SelectionHitRate returns the cache hit rate for tool selections
func (cs CacheStats) SelectionHitRate() float64 {
	total := cs.SelectionHits + cs.SelectionMisses
	if total == 0 {
		return 0.0
	}
	return float64(cs.SelectionHits) / float64(total)
}

// cacheKey creates a hash-based cache key from query and available tools
func (tc *ToolCache) cacheKey(query string, availableTools string) string {
	hasher := sha256.New()
	hasher.Write([]byte(query))
	hasher.Write([]byte("\n---\n"))
	hasher.Write([]byte(availableTools))
	return hex.EncodeToString(hasher.Sum(nil))
}

// removeExpiredSelection removes a specific cache entry
func (tc *ToolCache) removeExpiredSelection(key string) {
	tc.selectionMu.Lock()
	defer tc.selectionMu.Unlock()
	delete(tc.selectionCache, key)
}

// getSelectionCacheSize returns the current size of selection cache
func (tc *ToolCache) getSelectionCacheSize() int {
	tc.selectionMu.RLock()
	defer tc.selectionMu.RUnlock()
	return len(tc.selectionCache)
}

// StartCleanupRoutine starts a background goroutine to periodically clean expired entries
func (tc *ToolCache) StartCleanupRoutine(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tc.CleanExpired()
			}
		}
	}()
}

// parseDurationFromEnv parses a duration from environment variable or returns default
func parseDurationFromEnv(envVar string, defaultDuration time.Duration) time.Duration {
	val := os.Getenv(envVar)
	if val == "" {
		return defaultDuration
	}

	// Try parsing as duration string (e.g., "5m", "10s")
	if d, err := time.ParseDuration(val); err == nil {
		return d
	}

	// Try parsing as seconds
	if seconds, err := strconv.Atoi(val); err == nil {
		return time.Duration(seconds) * time.Second
	}

	return defaultDuration
}
