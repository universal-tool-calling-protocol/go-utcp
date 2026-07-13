package tag

import (
	"context"
	"sort"
	"strings"
	"unicode"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/repository"
)

// TagSearchStrategy implements a tool search strategy based on tags and description keywords.
type TagSearchStrategy struct {
	toolRepository    ToolRepository
	descriptionWeight float64
}

// NewTagSearchStrategy creates a new TagSearchStrategy with the given repository and description weight.
func NewTagSearchStrategy(repo ToolRepository, descriptionWeight float64) *TagSearchStrategy {
	return &TagSearchStrategy{
		toolRepository:    repo,
		descriptionWeight: descriptionWeight,
	}
}

type scoredTool struct {
	index int
	score float64
}

// SearchTools returns tools ordered by relevance to the query, using explicit tags and description keywords.
func (s *TagSearchStrategy) SearchTools(ctx context.Context, query string, limit int) ([]Tool, error) {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	queryWordSet := wordSet(queryLower)

	tools, err := s.toolRepository.GetTools(ctx)
	if err != nil {
		return nil, err
	}
	if len(tools) == 0 {
		return nil, nil
	}

	resultLimit := limit
	if resultLimit <= 0 || resultLimit > len(tools) {
		resultLimit = len(tools)
	}
	useTopK := resultLimit <= 64 && resultLimit*4 < len(tools)
	capacity := len(tools)
	if useTopK {
		capacity = resultLimit
	}
	selected := make([]scoredTool, 0, capacity)

	for index, tool := range tools {
		if index&63 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		var score float64

		for _, tag := range tool.Tags {
			tagLower := strings.ToLower(tag)
			if tagLower != "" && strings.Contains(queryLower, tagLower) {
				score += 1.0
			}
			score += matchingWordScore(tagLower, queryWordSet, 0, s.descriptionWeight)
		}

		if tool.Description != "" {
			score += matchingWordScore(strings.ToLower(tool.Description), queryWordSet, 3, s.descriptionWeight)
		}

		if score <= 0 {
			continue
		}
		candidate := scoredTool{index: index, score: score}
		if !useTopK {
			selected = append(selected, candidate)
			continue
		}
		position := len(selected)
		for i := range selected {
			if betterScoredTool(candidate, selected[i]) {
				position = i
				break
			}
		}
		if position >= resultLimit {
			continue
		}
		if len(selected) < resultLimit {
			selected = append(selected, scoredTool{})
		}
		copy(selected[position+1:], selected[position:len(selected)-1])
		selected[position] = candidate
	}

	if len(selected) == 0 {
		return append([]Tool(nil), tools[:resultLimit]...), nil
	}
	if !useTopK {
		sort.SliceStable(selected, func(i, j int) bool {
			return betterScoredTool(selected[i], selected[j])
		})
		if len(selected) > resultLimit {
			selected = selected[:resultLimit]
		}
	}

	result := make([]Tool, len(selected))
	for i, candidate := range selected {
		result[i] = tools[candidate.index]
	}

	return result, nil
}

func betterScoredTool(left, right scoredTool) bool {
	return left.score > right.score || left.score == right.score && left.index < right.index
}

func wordSet(value string) map[string]struct{} {
	words := make(map[string]struct{})
	forEachWord(value, func(word string) {
		words[word] = struct{}{}
	})
	return words
}

func matchingWordScore(value string, queryWords map[string]struct{}, minLength int, weight float64) float64 {
	var score float64
	forEachWord(value, func(word string) {
		if len(word) < minLength {
			return
		}
		if _, ok := queryWords[word]; ok {
			score += weight
		}
	})
	return score
}

func forEachWord(value string, visit func(string)) {
	start := -1
	for index, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			if start < 0 {
				start = index
			}
			continue
		}
		if start >= 0 {
			visit(value[start:index])
			start = -1
		}
	}
	if start >= 0 {
		visit(value[start:])
	}
}
