package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

type generatedPlan struct {
	Tools  []string `json:"tools"`
	Code   string   `json:"code"`
	Stream bool     `json:"stream"`
}

type scoredTool struct {
	score int
	index int
}

func (cm *CodeModeUTCP) CallTool(
	ctx context.Context,
	prompt string,
) (bool, any, error) {
	toolSpecs, _ := cm.toolSpecsAndCatalog()
	candidates := rankToolSpecs(prompt, toolSpecs, codeModeCandidateLimit())
	if len(candidates) == 0 {
		return false, "", nil
	}

	plan, err := cm.planAndGenerate(
		ctx,
		prompt,
		candidates,
		renderUtcpToolsForPrompt(candidates),
	)
	if err != nil {
		return false, "", err
	}

	usedTools := extractGeneratedToolNames(plan.Code)
	if len(usedTools) == 0 {
		return false, "", nil
	}
	if err := validateGeneratedPlan(plan, usedTools, toolNames(candidates)); err != nil {
		return true, "", err
	}

	raw, err := cm.Execute(ctx, CodeModeArgs{
		Code:    plan.Code,
		Timeout: 20000,
	})
	if err != nil {
		return true, "", err
	}

	return true, raw, nil
}

func (cm *CodeModeUTCP) planAndGenerate(
	ctx context.Context,
	query string,
	candidates []tools.Tool,
	toolSpecs string,
) (generatedPlan, error) {
	candidateNames := toolNames(candidates)
	toolsJSON, err := json.Marshal(candidateNames)
	if err != nil {
		return generatedPlan{}, fmt.Errorf("marshal candidate tools: %w", err)
	}

	prompt := fmt.Sprintf(`
Decide which UTCP tools are required and generate the complete CodeMode Go snippet in this same response.

USER QUERY:
%q

AVAILABLE TOOLS:
%s

TOOL SPECS:
%s

------------------------------------------------------------
SELECTION AND SNIPPET RULES
------------------------------------------------------------
- If no available tool is required, return exactly:
  {"tools":[],"code":"","stream":false}
- Use ONLY tool names listed in AVAILABLE TOOLS.
- The "tools" array must contain every tool called by the snippet and no unused tools.
- Use EXACT input keys from the tool schemas. Do NOT invent fields.
- Use ONLY these helper functions:
  - codemode.CallTool(name, args)
  - codemode.CallToolStream(name, args)
- NEVER use codemode.SearchTools, codemode.Sprintf, codemode.Errorf, fmt.Sprintf, or fmt.Errorf.
- No imports and no package declaration. Return ONLY Go statements in "code".
- String literals may contain Go source such as package main or import "fmt".
- Do not declare var __out.
- For early exits, assign __out and use return __out. NEVER use return nil.
- Always assign the final result to __out using =, not :=.
- For shell.run, when the schema contains argv, use:
      map[string]any{"argv": []string{"go", "run", "main.go"}}
- Set "stream" to true when any codemode.CallToolStream call is used.

------------------------------------------------------------
NON-STREAMING CALL EXAMPLE
------------------------------------------------------------
r1, err := codemode.CallTool("<tool_name>", map[string]any{
    "field": "value",
})
if err != nil {
    __out = err
    return __out
}
__out = r1

------------------------------------------------------------
CHAINING
------------------------------------------------------------
r1, err := codemode.CallTool("<first_tool>", map[string]any{
    "a": 5,
})
if err != nil {
    __out = err
    return __out
}

var value any
if m, ok := r1.(map[string]any); ok {
    value = m["result"]
}

r2, err := codemode.CallTool("<second_tool>", map[string]any{
    "value": value,
})
if err != nil {
    __out = err
    return __out
}
__out = r2

------------------------------------------------------------
STREAMING
------------------------------------------------------------
stream, err := codemode.CallToolStream("<stream_tool>", map[string]any{
    "input": "hello",
})
if err != nil {
    __out = err
    return __out
}
var items []any
for {
    chunk, err := stream.Next()
    if err != nil {
        break
    }
    items = append(items, chunk)
}
__out = items

Respond ONLY with one JSON object:
{
  "tools": ["provider.tool"],
  "code": "<Go statements>",
  "stream": false
}
`, query, string(toolsJSON), toolSpecs)

	raw, err := cm.model.Generate(ctx, prompt)
	if err != nil {
		return generatedPlan{}, err
	}

	jsonStr := extractJSON(fmt.Sprint(raw))
	if jsonStr == "" {
		return generatedPlan{}, fmt.Errorf("plan generation returned no JSON")
	}

	var plan generatedPlan
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return generatedPlan{}, fmt.Errorf("decode generated plan: %w", err)
	}

	plan.Code = normalizeSnippet(plan.Code)
	if strings.TrimSpace(plan.Code) == "" {
		if len(plan.Tools) == 0 {
			return generatedPlan{}, nil
		}
		return generatedPlan{}, fmt.Errorf("generated plan selected tools but returned empty code")
	}

	if !isValidSnippet(plan.Code) {
		log.Println("Skipping invalid snippet after normalization:", plan.Code)
		return generatedPlan{}, fmt.Errorf("snippet validation failed")
	}

	return plan, nil
}

func validateGeneratedPlan(plan generatedPlan, usedTools, allowedTools []string) error {
	allowed := make(map[string]struct{}, len(allowedTools))
	for _, name := range allowedTools {
		allowed[name] = struct{}{}
	}

	declared := make(map[string]struct{}, len(plan.Tools))
	for _, name := range plan.Tools {
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("generated plan selected unavailable tool %q", name)
		}
		declared[name] = struct{}{}
	}

	for _, name := range usedTools {
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("generated code references unavailable tool %q", name)
		}
		if _, ok := declared[name]; !ok {
			return fmt.Errorf("generated code references tool %q missing from tools list", name)
		}
	}

	for name := range declared {
		found := false
		for _, used := range usedTools {
			if used == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("generated plan declared unused tool %q", name)
		}
	}

	usesStream := strings.Contains(plan.Code, "codemode.CallToolStream(")
	if usesStream != plan.Stream {
		return fmt.Errorf("generated stream flag does not match generated code")
	}

	return nil
}

func rankToolSpecs(query string, specs []tools.Tool, limit int) []tools.Tool {
	if limit <= 0 {
		limit = 16
	}
	if limit > len(specs) {
		limit = len(specs)
	}

	queryLower := strings.ToLower(query)
	terms := toolQueryTerms(queryLower)
	useTopK := limit <= 64 && limit*4 < len(specs)
	capacity := len(specs)
	if useTopK {
		capacity = limit
	}
	selected := make([]scoredTool, 0, capacity)

	for index, spec := range specs {
		if spec.Name == CodeModeToolName {
			continue
		}

		name := strings.ToLower(spec.Name)
		description := spec.Description
		score := 0

		if name != "" && strings.Contains(queryLower, name) {
			score += 200
		}
		if provider, _, ok := strings.Cut(name, "."); ok && strings.Contains(queryLower, provider) {
			score += 30
		}

		for _, term := range terms {
			if strings.Contains(name, term) {
				score += 20
			}
			for _, tag := range spec.Tags {
				if containsFoldASCII(tag, term) {
					score += 8
					break
				}
			}
			if containsFoldASCII(description, term) {
				score += 4
			}
			for field := range spec.Inputs.Properties {
				if containsFoldASCII(field, term) {
					score += 6
				}
			}
		}

		candidate := scoredTool{score: score, index: index}
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
		if position >= limit {
			continue
		}
		if len(selected) < limit {
			selected = append(selected, scoredTool{})
		}
		copy(selected[position+1:], selected[position:len(selected)-1])
		selected[position] = candidate
	}
	if !useTopK {
		sort.SliceStable(selected, func(i, j int) bool {
			return betterScoredTool(selected[i], selected[j])
		})
		if len(selected) > limit {
			selected = selected[:limit]
		}
	}

	result := make([]tools.Tool, len(selected))
	for i, candidate := range selected {
		result[i] = specs[candidate.index]
	}
	return result
}

func containsFoldASCII(value, lowerNeedle string) bool {
	if lowerNeedle == "" {
		return true
	}
	if len(lowerNeedle) > len(value) {
		return false
	}
	for start := 0; start <= len(value)-len(lowerNeedle); start++ {
		matched := true
		for offset := range len(lowerNeedle) {
			char := value[start+offset]
			if char >= utf8.RuneSelf {
				return strings.Contains(strings.ToLower(value), lowerNeedle)
			}
			if char >= 'A' && char <= 'Z' {
				char += 'a' - 'A'
			}
			if char != lowerNeedle[offset] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func betterScoredTool(left, right scoredTool) bool {
	return left.score > right.score || left.score == right.score && left.index < right.index
}

func toolQueryTerms(query string) []string {
	parts := strings.FieldsFunc(query, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' && r != '-'
	})

	terms := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if len(part) < 2 {
			continue
		}
		if isToolQueryStopWord(part) {
			continue
		}
		if _, duplicate := seen[part]; duplicate {
			continue
		}
		seen[part] = struct{}{}
		terms = append(terms, part)
	}
	return terms
}

func isToolQueryStopWord(word string) bool {
	switch word {
	case "a", "an", "and", "are", "as", "at",
		"be", "by", "for", "from", "in", "is",
		"it", "of", "on", "or", "the", "to",
		"use", "using", "with":
		return true
	default:
		return false
	}
}

func codeModeCandidateLimit() int {
	const defaultLimit = 16
	value := os.Getenv("UTCP_CODEMODE_CANDIDATE_LIMIT")
	if value == "" {
		return defaultLimit
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return defaultLimit
	}
	return limit
}

func toolNames(specs []tools.Tool) []string {
	names := make([]string, len(specs))
	for i, spec := range specs {
		names[i] = spec.Name
	}
	return names
}

func renderUtcpToolsForPrompt(specs []tools.Tool) string {
	ordered := append([]tools.Tool(nil), specs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Name < ordered[j].Name
	})

	var sb strings.Builder

	sb.WriteString("------------------------------------------------------------\n")
	sb.WriteString("UTCP TOOL REFERENCE (INPUT + OUTPUT SCHEMAS)\n")
	sb.WriteString("Use EXACT field names listed below. Do NOT invent new keys.\n")
	sb.WriteString("------------------------------------------------------------\n\n")

	for _, t := range ordered {

		sb.WriteString(fmt.Sprintf("TOOL: %s\n", t.Name))
		sb.WriteString(fmt.Sprintf("DESCRIPTION: %s\n\n", t.Description))

		// -------------------------------
		// INPUT FIELD LIST
		// -------------------------------
		sb.WriteString("INPUT FIELDS (USE EXACTLY THESE KEYS):\n")

		if len(t.Inputs.Properties) == 0 {
			sb.WriteString("- (no fields)\n")
		} else {
			keys := make([]string, 0, len(t.Inputs.Properties))
			for key := range t.Inputs.Properties {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				raw := t.Inputs.Properties[key]

				// Try to extract "type" from nested schema if present
				propType := "any"
				if m, ok := raw.(map[string]any); ok {
					if v, ok := m["type"]; ok {
						if s, ok := v.(string); ok {
							propType = s
						}
					}
				}

				sb.WriteString(fmt.Sprintf("- %s: %s\n", key, propType))
			}
		}

		// Required field list
		if len(t.Inputs.Required) > 0 {
			sb.WriteString("\nREQUIRED FIELDS:\n")
			for _, r := range t.Inputs.Required {
				sb.WriteString(fmt.Sprintf("- %s\n", r))
			}
		}

		sb.WriteString("\n")

		// Full JSON schema for LLM clarity
		inBytes, _ := json.MarshalIndent(t.Inputs, "", "  ")
		sb.WriteString("FULL INPUT SCHEMA (JSON):\n")
		sb.WriteString(string(inBytes))
		sb.WriteString("\n\n")

		// -------------------------------
		// OUTPUT SCHEMA
		// -------------------------------
		sb.WriteString("OUTPUT SCHEMA (EXACT SHAPE RETURNED BY TOOL):\n")

		if t.Outputs.Type != "" || len(t.Outputs.Properties) > 0 {
			outBytes, _ := json.MarshalIndent(t.Outputs, "", "  ")
			sb.WriteString(string(outBytes))
		} else {
			// Generic fallback
			sb.WriteString("{ \"result\": <any> }\n")
		}

		sb.WriteString("\n")
		sb.WriteString("------------------------------------------------------------\n\n")
	}

	return sb.String()
}

func renderUtcpToolCatalog(specs []tools.Tool) string {
	ordered := append([]tools.Tool(nil), specs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Name < ordered[j].Name
	})

	var sb strings.Builder

	sb.WriteString("AVAILABLE UTCP TOOLS:\n")
	for _, t := range ordered {
		sb.WriteString("- ")
		sb.WriteString(t.Name)
		if t.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(t.Description)
		}
		sb.WriteByte('\n')
	}

	return sb.String()
}

func (cm *CodeModeUTCP) toolSpecsAndCatalog() ([]tools.Tool, string) {
	if cm.cache != nil {
		if specs, catalog := cm.cache.getToolSpecsAndCatalogShared(); specs != nil {
			if catalog == "" {
				catalog = renderUtcpToolCatalog(specs)
				cm.cache.SetToolCatalog(catalog)
			}
			return specs, catalog
		}
	}

	specs := cm.loadToolSpecs()
	catalog := renderUtcpToolCatalog(specs)
	if cm.cache != nil {
		cm.cache.SetToolSpecsAndCatalog(specs, catalog)
	}
	return specs, catalog
}

func (a *CodeModeUTCP) ToolSpecs() []tools.Tool {
	// Check cache first
	if a.cache != nil {
		if cached := a.cache.GetToolSpecs(); cached != nil {
			return cached
		}
	}

	allSpecs := a.loadToolSpecs()

	// Store in cache
	if a.cache != nil {
		a.cache.SetToolSpecs(allSpecs)
	}

	return allSpecs
}

func (a *CodeModeUTCP) loadToolSpecs() []tools.Tool {
	var allSpecs []tools.Tool
	seen := make(map[string]bool)

	if cmTools, err := a.Tools(); err == nil {
		for _, t := range cmTools {
			key := strings.ToLower(strings.TrimSpace(t.Name))
			if key == "" || seen[key] {
				continue
			}
			allSpecs = append(allSpecs, t)
			seen[key] = true
		}
	}

	limit, err := strconv.Atoi(os.Getenv("utcp_search_tools_limit"))
	if err != nil {
		limit = 50
	}
	if limit == 0 {
		limit = 50
	}

	if a.client != nil {
		utcpTools, _ := a.client.SearchTools("", limit)
		for _, tool := range utcpTools {
			key := strings.ToLower(tool.Name)
			if !seen[key] {
				allSpecs = append(allSpecs, tool)
				seen[key] = true
			}
		}
	}

	return allSpecs
}

func extractJSON(response string) string {
	response = strings.TrimSpace(response)

	// Case 1: Pure JSON (starts and ends with braces)
	if strings.HasPrefix(response, "{") && strings.HasSuffix(response, "}") {
		return response
	}

	// Case 2: JSON wrapped in markdown code fence
	// ```json\n{...}\n```
	if strings.Contains(response, "```") {
		// Remove opening fence
		response = strings.TrimSpace(response)
		response = strings.TrimPrefix(response, "```json")
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSpace(response)

		// Remove closing fence
		if idx := strings.Index(response, "```"); idx != -1 {
			response = response[:idx]
		}
		response = strings.TrimSpace(response)

		if strings.HasPrefix(response, "{") && strings.HasSuffix(response, "}") {
			return response
		}
	}

	// Case 3: JSON followed by extra content (e.g., " | prompt text")
	// Find the first { and try to extract a complete JSON object
	startIdx := strings.Index(response, "{")
	if startIdx == -1 {
		return ""
	}

	// Find the matching closing brace
	depth := 0
	inString := false
	escaped := false

	for i := startIdx; i < len(response); i++ {
		ch := response[i]

		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				// Found the matching closing brace
				candidate := response[startIdx : i+1]
				// Validate it's actually valid JSON
				var test interface{}
				if json.Unmarshal([]byte(candidate), &test) == nil {
					return candidate
				}
			}
		}
	}

	return ""
}

func isValidSnippet(code string) bool {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return false
	}

	// Disallow package/import declarations only when they appear as the first
	// actual snippet statement. Do not scan every line because snippets often
	// contain Go source code inside string literals, for example a filesystem.write
	// call that writes package main and import "fmt" into main.go.
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "package ") || line == "package" {
			return false
		}
		if strings.HasPrefix(line, "import ") || line == "import" || strings.HasPrefix(line, "import(") || strings.HasPrefix(line, "import (") {
			return false
		}
		break
	}
	if containsBareCallTool(trimmed) {
		return false
	}
	// Disallow map literals printed with fmt as map[value:...].
	if strings.Contains(trimmed, "map[value:") {
		return false
	}

	// Disallow declaring __out as a variable.
	if strings.Contains(trimmed, "var __out") {
		return false
	}

	// Ensure __out is assigned using '=' not ':=' unless '__out, err :=' pattern.
	if strings.Contains(trimmed, "__out :=") && !strings.Contains(trimmed, "__out, err :=") {
		return false
	}

	// Ensure there is at least one assignment to __out using '=' or '__out, err :='.
	if !strings.Contains(trimmed, "__out =") && !strings.Contains(trimmed, "__out, err :=") {
		return false
	}

	return true
}

// ───────────────────────────────────────────────────────────
//   Cache Management Methods
// ───────────────────────────────────────────────────────────

// InvalidateToolSpecsCache clears the cached tool specifications
func (cm *CodeModeUTCP) InvalidateToolSpecsCache() {
	if cm.cache != nil {
		cm.cache.InvalidateToolSpecs()
	}
}

// InvalidateSelectionsCache clears all cached tool selection results
func (cm *CodeModeUTCP) InvalidateSelectionsCache() {
	if cm.cache != nil {
		cm.cache.InvalidateSelections()
	}
}

// InvalidateAllCaches clears all caches (tool specs and selections)
func (cm *CodeModeUTCP) InvalidateAllCaches() {
	if cm.cache != nil {
		cm.cache.InvalidateAll()
	}
}

// CacheStats returns performance statistics for the tool cache
func (cm *CodeModeUTCP) CacheStats() CacheStats {
	if cm.cache == nil {
		return CacheStats{}
	}
	return cm.cache.Stats()
}

// StartCacheCleanup starts a background routine to clean expired cache entries
// Call this with a context to control the cleanup lifecycle
func (cm *CodeModeUTCP) StartCacheCleanup(ctx context.Context, interval time.Duration) {
	if cm.cache != nil {
		cm.cache.StartCleanupRoutine(ctx, interval)
	}
}

func containsBareCallTool(code string) bool {
	badForms := []string{
		"CallTool(",
		"CallToolStream(",
	}

	for _, bad := range badForms {
		idx := strings.Index(code, bad)
		for idx >= 0 {
			before := ""
			if idx >= len("codemode.") {
				before = code[idx-len("codemode.") : idx]
			}

			if before != "codemode." {
				return true
			}

			nextStart := idx + len(bad)
			next := strings.Index(code[nextStart:], bad)
			if next < 0 {
				break
			}
			idx = nextStart + next
		}
	}

	return false
}
