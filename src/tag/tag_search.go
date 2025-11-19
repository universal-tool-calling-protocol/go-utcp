package tag

import (
	"context"
	"regexp"
	"sort"
	"strings"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/repository"
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
func (s *TagSearchStrategy) SearchTools(ctx context.Context, query string, limit int) ([]Tool, error) {
	// Normalize query
	queryLower := strings.ToLower(strings.TrimSpace(query))
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

	// Compute SUTCP score for each tool
	type scoredTool struct {
		tool  Tool
		score float64
	}
	var scored []scoredTool

	for _, t := range tools {
		var score float64

		// Match against tags
		for _, tag := range t.Tags {
			tagLower := strings.ToLower(tag)

			// Direct substring match
			if strings.Contains(queryLower, tagLower) {
				score += 1.0
			}

			// Word-level overlap
			tagWords := s.wordRegex.FindAllString(tagLower, -1)
			for _, w := range tagWords {
				if _, ok := queryWordSet[w]; ok {
					score += s.descriptionWeight
				}
			}
		}

		// Match against description
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

		scored = append(scored, scoredTool{tool: t, score: score})
	}

	// Sort descending by score
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Collect only positive matches
	var result []Tool
	for _, st := range scored {
		if st.score > 0 {
			result = append(result, st.tool)
			if len(result) >= limit {
				break
			}
		}
	}

	// If no matches, fallback to top N (for discoverability)
	if len(result) == 0 && len(scored) > 0 {
		for i, st := range scored {
			if i >= limit {
				break
			}
			result = append(result, st.tool)
		}
	}

	return result, nil
}
