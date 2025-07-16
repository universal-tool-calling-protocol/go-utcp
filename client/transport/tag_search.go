package transport

import (
	"context"
	"regexp"
	"server"
	"sort"
	"strings"
)

// TagSearchStrategy implements a tool search strategy based on tags and description keywords.
type TagSearchStrategy struct {
	toolRepository    ToolRepository
	descriptionWeight float64
	wordRegex         *regexp.Regexp
}

// NewTagSearchStrategy creates a new TagSearchStrategy with the given repository and description weight.
func NewTagSearchStrategy(repo ToolRepository, descriptionWeight float64) *TagSearchStrategy {
	return &TagSearchStrategy{
		toolRepository:    repo,
		descriptionWeight: descriptionWeight,
		wordRegex:         regexp.MustCompile(`\w+`),
	}
}

// SearchTools returns tools ordered by relevance to the query, using explicit tags and description keywords.
func (s *TagSearchStrategy) SearchTools(ctx context.Context, query string, limit int) ([]server.Tool, error) {
	// Normalize query
	queryLower := strings.ToLower(query)
	words := s.wordRegex.FindAllString(queryLower, -1)
	queryWordSet := make(map[string]struct{}, len(words))
	for _, w := range words {
		queryWordSet[w] = struct{}{}
	}

	// Retrieve all tools
	tools, err := s.toolRepository.GetTools(context.Background())
	if err != nil {
		return nil, err
	}

	// Score each tool
	type scoredTool struct {
		t     server.Tool
		score float64
	}
	var scored []scoredTool
	for _, t := range tools {
		var score float64

		// Score from tags
		for _, tag := range t.Tags {
			tagLower := strings.ToLower(tag)
			if strings.Contains(queryLower, tagLower) {
				score += 1.0
			}
			// Partial matches on tag words
			tagWords := s.wordRegex.FindAllString(tagLower, -1)
			for _, w := range tagWords {
				if _, ok := queryWordSet[w]; ok {
					score += s.descriptionWeight
				}
			}
		}

		// Score from description
		if t.Description != "" {
			descWords := s.wordRegex.FindAllString(strings.ToLower(t.Description), -1)
			for _, w := range descWords {
				if len(w) > 2 {
					if _, ok := queryWordSet[w]; ok {
						score += s.descriptionWeight
					}
				}
			}
		}

		scored = append(scored, scoredTool{t: t, score: score})
	}

	// Sort by descending score
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Return up to limit
	var result []server.Tool
	for i, st := range scored {
		if i >= limit {
			break
		}
		result = append(result, st.t)
	}
	return result, nil
}
