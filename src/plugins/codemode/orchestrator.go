package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
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

func (cm *CodeModeUTCP) CallTool(ctx context.Context, prompt string) (bool, any, error) {
	toolSpecs, _ := cm.toolSpecsAndCatalog()
	candidates := rankToolSpecs(prompt, toolSpecs, codeModeCandidateLimit())
	if len(candidates) == 0 {
		return false, "", nil
	}
	plan, err := cm.planAndGenerate(ctx, prompt, candidates, renderUtcpToolsForPrompt(candidates))
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
	result, err := cm.Execute(ctx, CodeModeArgs{Code: plan.Code, Timeout: 20000})
	if err != nil {
		return true, "", err
	}
	return true, result, nil
}

func (cm *CodeModeUTCP) planAndGenerate(ctx context.Context, query string, candidates []tools.Tool, toolSpecs string) (generatedPlan, error) {
	candidateNames := toolNames(candidates)
	toolsJSON, err := json.Marshal(candidateNames)
	if err != nil {
		return generatedPlan{}, fmt.Errorf("marshal candidate tools: %w", err)
	}

	prompt := fmt.Sprintf(`
You are a strict UTCP CodeMode planner and executor.

The CodeMode runtime is Expr v1.17. Generate VALID EXPR SOURCE, never Go.
The generated program is executed directly by github.com/expr-lang/expr.

Analyze USER QUERY and produce:
1. the exact UTCP tools that must be called
2. one executable Expr program calling only those tools
3. whether streaming is required

CLOSED-WORLD TOOL AUTHORITY
AVAILABLE TOOLS is the immutable whitelist. A tool is valid only when its exact
fully-qualified name appears in that list. Never invent, rename, abbreviate,
autocomplete, infer, or substitute a tool name.

AVAILABLE TOOLS:
%s

TOOL SPECS:
%s

Before generating code, verify every required capability against AVAILABLE TOOLS,
every input key against TOOL SPECS, and every referenced tool name byte-for-byte.
If the request cannot be satisfied, return exactly:
{"tools":[],"code":"","stream":false}

EXPR CODEMODE CONTRACT
The program MUST be valid Expr syntax.

Allowed runtime API:
- codemode.CallTool("EXACT_TOOL_NAME", {"field": value})
- codemode.CallToolStream("EXACT_TOOL_NAME", {"field": value})
- codemode.Get(value, "field")

Expr syntax:
- sequential expressions separated by ';'
- variables use Expr let bindings
- maps use {"field": value}
- arrays use [value1, value2]
- final expression is the program result

The generated source MUST contain only Expr syntax and the allowed CodeMode API.
Do not emit Go syntax, Go declarations, Go control-flow, Go error-handling,
Go imports, Go type assertions, or Go-specific runtime APIs.
Do not use internal output variables, explicit returns, stream iteration,
or tool-discovery APIs inside the generated program.

Tool errors are propagated by the Expr runtime. Do not write explicit error handling.
Do not use markdown fences.

USER QUERY:
%q

AVAILABLE TOOLS:
%s

TOOL SPECS:
%s

EXAMPLES
Single call:
let result = codemode.CallTool("<EXACT_TOOL_NAME>", {"field": "value"}); result

Sequential chain:
let r1 = codemode.CallTool("<EXACT_FIRST_TOOL_NAME>", {"a": 5});
let value = codemode.Get(r1, "result");
let r2 = codemode.CallTool("<EXACT_SECOND_TOOL_NAME>", {"value": value}); r2

Streaming:
codemode.CallToolStream("<EXACT_STREAM_TOOL_NAME>", {"input": "hello"})
A stream call already returns the collected stream value. Do not iterate the stream.

FINAL CHECK
1. every declared tool is in AVAILABLE TOOLS
2. every referenced tool is declared
3. every declared tool is referenced
4. every argument key exists in the schema
5. stream=true iff CallToolStream is used
6. code is valid Expr and contains no Go constructs
7. only CallTool, CallToolStream and Get are used

Return exactly one JSON object:
{"tools":["provider.tool"],"code":"<Expr source>","stream":false}
`, string(toolsJSON), toolSpecs, query, string(toolsJSON), toolSpecs)

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
	selected := make([]scoredTool, 0, len(specs))
	for index, spec := range specs {
		if spec.Name == CodeModeToolName {
			continue
		}
		name := strings.ToLower(spec.Name)
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
			if containsFoldASCII(spec.Description, term) {
				score += 4
			}
			for field := range spec.Inputs.Properties {
				if containsFoldASCII(field, term) {
					score += 6
				}
			}
		}
		selected = append(selected, scoredTool{score: score, index: index})
	}
	sort.SliceStable(selected, func(i, j int) bool { return betterScoredTool(selected[i], selected[j]) })
	if len(selected) > limit {
		selected = selected[:limit]
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
	parts := strings.FieldsFunc(query, func(r rune) bool { return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' })
	terms := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if len(part) < 2 || isToolQueryStopWord(part) {
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
	case "a", "an", "and", "are", "as", "at", "be", "by", "for", "from", "in", "is", "it", "of", "on", "or", "the", "to", "use", "using", "with":
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
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	var sb strings.Builder
	sb.WriteString("------------------------------------------------------------\nUTCP TOOL REFERENCE (INPUT + OUTPUT SCHEMAS)\nUse EXACT field names listed below. Do NOT invent new keys.\n------------------------------------------------------------\n\n")
	for _, t := range ordered {
		sb.WriteString(fmt.Sprintf("TOOL: %s\nDESCRIPTION: %s\n\n", t.Name, t.Description))
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
				propType := "any"
				if m, ok := t.Inputs.Properties[key].(map[string]any); ok {
					if value, ok := m["type"].(string); ok {
						propType = value
					}
				}
				sb.WriteString(fmt.Sprintf("- %s: %s\n", key, propType))
			}
		}
		if len(t.Inputs.Required) > 0 {
			sb.WriteString("\nREQUIRED FIELDS:\n")
			for _, required := range t.Inputs.Required {
				sb.WriteString(fmt.Sprintf("- %s\n", required))
			}
		}
		inBytes, _ := json.MarshalIndent(t.Inputs, "", "  ")
		sb.WriteString("\nFULL INPUT SCHEMA (JSON):\n")
		sb.Write(inBytes)
		sb.WriteString("\n\nOUTPUT SCHEMA (EXACT SHAPE RETURNED BY TOOL):\n")
		if t.Outputs.Type != "" || len(t.Outputs.Properties) > 0 {
			outBytes, _ := json.MarshalIndent(t.Outputs, "", "  ")
			sb.Write(outBytes)
		} else {
			sb.WriteString(`{"result": <any>}`)
		}
		sb.WriteString("\n\n------------------------------------------------------------\n\n")
	}
	return sb.String()
}

func renderUtcpToolCatalog(specs []tools.Tool) string {
	ordered := append([]tools.Tool(nil), specs...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
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
	if a.cache != nil {
		if cached := a.cache.GetToolSpecs(); cached != nil {
			return cached
		}
	}
	allSpecs := a.loadToolSpecs()
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
	if err != nil || limit == 0 {
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
