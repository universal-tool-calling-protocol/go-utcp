package tag

import (
	"context"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/repository"
)

// TagSearchStrategy implements a tool search strategy based on tags and description keywords.
type TagSearchStrategy struct {
	toolRepository    ToolRepository
	descriptionWeight float64
	indexMu           sync.RWMutex
	indexRevision     uint64
	indexReady        bool
	index             []indexedTool
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
	slot  int
}

type indexedTool struct {
	tool             Tool
	tags             []indexedTag
	descriptionWords []string
}

type indexedTag struct {
	value string
	words []string
}

// SearchTools returns tools ordered by relevance to the query, using explicit tags and description keywords.
func (s *TagSearchStrategy) SearchTools(ctx context.Context, query string, limit int) ([]Tool, error) {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	queryWordSet := wordSet(queryLower)

	state := newSearchState(queryLower, queryWordSet, s.descriptionWeight, limit)
	if iterator, ok := s.toolRepository.(ToolRepositoryIterator); ok {
		if versioned, ok := s.toolRepository.(ToolRepositoryRevision); ok {
			indexed, err := s.indexedTools(ctx, iterator, versioned.ToolRevision())
			if err != nil {
				return nil, err
			}
			for index := range indexed {
				if index&63 == 0 {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					default:
					}
				}
				entry := &indexed[index]
				state.addScored(&entry.tool, scoreIndexedTool(entry, queryLower, queryWordSet, s.descriptionWeight))
			}
			return state.results(), nil
		}
		if err := iterator.RangeTools(ctx, func(tool Tool) bool {
			state.add(&tool)
			return true
		}); err != nil {
			return nil, err
		}
		return state.results(), nil
	}

	tools, err := s.toolRepository.GetTools(ctx)
	if err != nil {
		return nil, err
	}
	for index, tool := range tools {
		if index&63 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		state.add(&tool)
	}
	return state.results(), nil
}

type searchState struct {
	queryLower        string
	queryWords        map[string]struct{}
	descriptionWeight float64
	limit             int
	index             int
	useTopK           bool
	selected          []scoredTool
	tools             []Tool
	fallback          []Tool
}

func newSearchState(queryLower string, queryWords map[string]struct{}, descriptionWeight float64, limit int) *searchState {
	capacity := 64
	useTopK := limit > 0 && limit <= 64
	if useTopK {
		capacity = limit
	}
	return &searchState{
		queryLower:        queryLower,
		queryWords:        queryWords,
		descriptionWeight: descriptionWeight,
		limit:             limit,
		useTopK:           useTopK,
		selected:          make([]scoredTool, 0, capacity),
		tools:             make([]Tool, 0, capacity),
	}
}

func (s *searchState) add(tool *Tool) {
	s.addScored(tool, scoreTool(tool, s.queryLower, s.queryWords, s.descriptionWeight))
}

func (s *searchState) addScored(tool *Tool, score float64) {
	index := s.index
	s.index++
	if score <= 0 {
		if len(s.selected) == 0 && (s.limit <= 0 || len(s.fallback) < s.limit) {
			if s.fallback == nil {
				capacity := 64
				if s.limit > 0 && s.limit < capacity {
					capacity = s.limit
				}
				s.fallback = make([]Tool, 0, capacity)
			}
			s.fallback = append(s.fallback, *tool)
		}
		return
	}
	s.fallback = nil
	candidate := scoredTool{index: index, score: score}
	if !s.useTopK {
		candidate.slot = len(s.tools)
		s.tools = append(s.tools, *tool)
		s.selected = append(s.selected, candidate)
		return
	}

	position := len(s.selected)
	for i := range s.selected {
		if betterScoredTool(candidate, s.selected[i]) {
			position = i
			break
		}
	}
	if position >= s.limit {
		return
	}
	if len(s.selected) < s.limit {
		candidate.slot = len(s.tools)
		s.tools = append(s.tools, *tool)
		s.selected = append(s.selected, scoredTool{})
	} else {
		candidate.slot = s.selected[len(s.selected)-1].slot
		s.tools[candidate.slot] = *tool
	}
	copy(s.selected[position+1:], s.selected[position:len(s.selected)-1])
	s.selected[position] = candidate
}

func (s *searchState) results() []Tool {
	if s.index == 0 {
		return nil
	}
	if len(s.selected) == 0 {
		return s.fallback
	}
	if !s.useTopK {
		sort.SliceStable(s.selected, func(i, j int) bool {
			return betterScoredTool(s.selected[i], s.selected[j])
		})
		if s.limit > 0 && len(s.selected) > s.limit {
			s.selected = s.selected[:s.limit]
		}
	}

	result := make([]Tool, len(s.selected))
	for i, candidate := range s.selected {
		result[i] = s.tools[candidate.slot]
	}
	return result
}

func scoreTool(tool *Tool, queryLower string, queryWords map[string]struct{}, descriptionWeight float64) float64 {
	var score float64
	for _, tag := range tool.Tags {
		tagLower := strings.ToLower(tag)
		if tagLower != "" && strings.Contains(queryLower, tagLower) {
			score++
		}
		score += matchingWordScore(tag, queryWords, 0, descriptionWeight)
	}
	if tool.Description != "" {
		score += matchingWordScore(tool.Description, queryWords, 3, descriptionWeight)
	}
	return score
}

func scoreIndexedTool(tool *indexedTool, queryLower string, queryWords map[string]struct{}, descriptionWeight float64) float64 {
	var score float64
	for _, tag := range tool.tags {
		if tag.value != "" && strings.Contains(queryLower, tag.value) {
			score++
		}
		for _, word := range tag.words {
			if _, ok := queryWords[word]; ok {
				score += descriptionWeight
			}
		}
	}
	for _, word := range tool.descriptionWords {
		if _, ok := queryWords[word]; ok {
			score += descriptionWeight
		}
	}
	return score
}

func (s *TagSearchStrategy) indexedTools(ctx context.Context, iterator ToolRepositoryIterator, revision uint64) ([]indexedTool, error) {
	s.indexMu.RLock()
	if s.indexReady && s.indexRevision == revision {
		indexed := s.index
		s.indexMu.RUnlock()
		return indexed, nil
	}
	s.indexMu.RUnlock()

	indexed := make([]indexedTool, 0, 64)
	if err := iterator.RangeTools(ctx, func(tool Tool) bool {
		indexed = append(indexed, indexTool(tool))
		return true
	}); err != nil {
		return nil, err
	}

	s.indexMu.Lock()
	if !s.indexReady || revision > s.indexRevision {
		s.index = indexed
		s.indexRevision = revision
		s.indexReady = true
	} else if s.indexRevision == revision {
		indexed = s.index
	}
	s.indexMu.Unlock()
	return indexed, nil
}

func indexTool(tool Tool) indexedTool {
	indexed := indexedTool{tool: tool}
	if len(tool.Tags) > 0 {
		indexed.tags = make([]indexedTag, len(tool.Tags))
		for i, tag := range tool.Tags {
			lower := strings.ToLower(tag)
			indexed.tags[i] = indexedTag{value: lower, words: indexWords(lower, 0)}
		}
	}
	if tool.Description != "" {
		indexed.descriptionWords = indexWords(strings.ToLower(tool.Description), 3)
	}
	return indexed
}

func indexWords(value string, minLength int) []string {
	var words []string
	forEachWord(value, func(word string) {
		if len(word) >= minLength {
			words = append(words, word)
		}
	})
	return words
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
	start := -1
	needsFold := false
	nonASCII := false
	for index, r := range value {
		if isWordRune(r) {
			if start < 0 {
				start = index
				needsFold = false
				nonASCII = false
			}
			if r >= utf8.RuneSelf {
				nonASCII = true
				needsFold = needsFold || unicode.IsUpper(r)
			} else if r >= 'A' && r <= 'Z' {
				needsFold = true
			}
			continue
		}
		if start >= 0 {
			score += matchingSingleWordScore(value[start:index], queryWords, minLength, weight, needsFold, nonASCII)
			start = -1
		}
	}
	if start >= 0 {
		score += matchingSingleWordScore(value[start:], queryWords, minLength, weight, needsFold, nonASCII)
	}
	return score
}

func matchingSingleWordScore(word string, queryWords map[string]struct{}, minLength int, weight float64, needsFold, nonASCII bool) float64 {
	if len(word) < minLength {
		return 0
	}
	if _, ok := queryWords[word]; ok {
		return weight
	}
	if !needsFold {
		return 0
	}

	// Tool metadata is overwhelmingly ASCII. Fold short ASCII words on the
	// stack so mixed-case descriptions do not allocate a lowercase string.
	if !nonASCII && len(word) <= 64 {
		var folded [64]byte
		for i := range len(word) {
			char := word[i]
			if char >= 'A' && char <= 'Z' {
				char += 'a' - 'A'
			}
			folded[i] = char
		}
		if _, ok := queryWords[string(folded[:len(word)])]; ok {
			return weight
		}
		return 0
	}

	// Preserve Unicode case-insensitive matching for the uncommon non-ASCII
	// path without penalizing normal tool catalogs.
	for queryWord := range queryWords {
		if strings.EqualFold(word, queryWord) {
			return weight
		}
	}
	return 0
}

func forEachWord(value string, visit func(string)) {
	start := -1
	for index, r := range value {
		if isWordRune(r) {
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

func isWordRune(r rune) bool {
	if r < utf8.RuneSelf {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '_'
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
