package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

func (cm *CodeModeUTCP) CallTool(
	ctx context.Context,
	prompt string,
) (bool, any, error) {
	toolSpecs, catalog := cm.toolSpecsAndCatalog()

	// A selection of zero tools also answers the former "are tools needed?"
	// question, saving one model round trip for every request.
	selected, err := cm.selectTools(ctx, prompt, catalog)
	if err != nil {
		return false, "", err
	}
	if len(selected) == 0 {
		return false, "", nil
	}

	selectedSpecs := selectedToolSpecs(toolSpecs, selected)
	if len(selectedSpecs) == 0 {
		return false, "", nil
	}

	selected = toolNames(selectedSpecs)
	snippet, ok, err := cm.generateSnippet(ctx, prompt, selected, renderUtcpToolsForPrompt(selectedSpecs))
	if err != nil && !ok {
		return true, "", err
	}

	timeout := 20000
	raw, err := cm.Execute(ctx, CodeModeArgs{
		Code:    snippet,
		Timeout: timeout,
	})
	if err != nil {
		return false, "", err
	}

	return true, raw, nil
}

func selectedToolSpecs(specs []tools.Tool, selected []string) []tools.Tool {
	byName := make(map[string]tools.Tool, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}

	result := make([]tools.Tool, 0, len(selected))
	seen := make(map[string]struct{}, len(selected))
	for _, name := range selected {
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		spec, ok := byName[name]
		if !ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, spec)
	}
	return result
}

func toolNames(specs []tools.Tool) []string {
	names := make([]string, len(specs))
	for i, spec := range specs {
		names[i] = spec.Name
	}
	return names
}

func (cm *CodeModeUTCP) generateSnippet(
	ctx context.Context,
	query string,
	tools []string,
	toolSpecs string,
) (string, bool, error) {

	toolsJSON, _ := json.Marshal(tools)

	prompt := fmt.Sprintf(`
Generate a Go snippet that uses ONLY the following UTCP tools:

%v

USER QUERY:
%q

TOOL SPECS:
%s

------------------------------------------------------------
SNIPPET RULES
------------------------------------------------------------
- Use ONLY the tool names listed above.
- Use EXACT input keys from the tool schemas. Do NOT invent new fields.
- Use ONLY these helper functions:
  - codemode.CallTool(name, args)
  - codemode.CallToolStream(name, args)
  - codemode.SearchTools(query, limit)
- NEVER use codemode.Sprintf, codemode.Errorf, fmt.Sprintf, or fmt.Errorf.
- No imports, no package declarations — ONLY Go statements.
- It is OK for string literals to contain Go source text such as package main and import "fmt".
- Do not declare var __out.
- For early exits, use return __out. NEVER use return nil.
- Always assign the final result to __out using =, not :=.
- For shell.run, if the schema contains argv, use:
      map[string]any{"argv": []string{"go", "run", "main.go"}}
- If ANY streaming tool is used, set "stream": true.

------------------------------------------------------------
CHAINING (NON-STREAMING) — STRICT RULES
------------------------------------------------------------
To pass output of one tool into another:

1. Call the tool:
    r1, err := codemode.CallTool("<tool_name>", map[string]any{
        "a": 5,
        "b": 7,
    })
    if err != nil {
        __out = err
        return
    }

2. Extract value using EXACT output-schema keys:
    var sum any
    if m, ok := r1.(map[string]any); ok {
        sum = m["result"]   // key MUST match schema
    }

3. Use this value as input to the next tool:
    r2, err := codemode.CallTool("<another_tool_name>", map[string]any{
        "a": sum,
        "b": 3,
    })
    if err != nil {
        __out = err
        return
    }

4. The final line must set:
    __out = map[string]any{ // USE = NOT :=
        "sum": sum,
        "product": r2,
    }

------------------------------------------------------------
AGENT TOOLS (e.g. 'specialist.specialist') — STRICT RULES
------------------------------------------------------------
Agent tools ALWAYS require an 'instruction' key.

    fact, err := codemode.CallTool("specialist.specialist", map[string]any{
        "instruction": "Tell me a fun fact about the Eiffel Tower.",
    })
------------------------------------------------------------
STREAMING TOOLS — STRICT RULES
------------------------------------------------------------
When calling a streaming tool:

1. Start the stream:
    stream, err := codemode.CallToolStream("<stream_tool>", map[string]any{
        "input": "hello",
    })
    if err != nil {
        __out = err
        return
    }

2. Read chunks in a loop:
    var items []any
    for {
        chunk, err := stream.Next()
        if err != nil {
            break
        }
        items = append(items, chunk)
    }

3. You may chain streaming results into non-streaming tools:
    r2, err := codemode.CallTool("provider.summarize", map[string]any{
        "values": items,
    })

4. Or output directly:
    __out = items

------------------------------------------------------------
Respond ONLY in JSON:
{
  "code": "<go snippet>",
  "stream": false
}
`, string(toolsJSON), query, toolSpecs)

	raw, err := cm.model.Generate(ctx, prompt)
	if err != nil {
		return "", false, err
	}

	jsonStr := extractJSON(fmt.Sprint(raw))
	if jsonStr == "" {
		return "", false, fmt.Errorf("snippet empty")
	}

	var resp struct {
		Code   string `json:"code"`
		Stream bool   `json:"stream"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return "", false, err
	}

	resp.Code = normalizeSnippet(resp.Code)
	if !isValidSnippet(resp.Code) {
		log.Println("Skipping invalid snippet after normalization:", resp.Code)
		return "", false, fmt.Errorf("snippet validation failed")
	}

	return resp.Code, resp.Stream, nil
}

func renderUtcpToolsForPrompt(specs []tools.Tool) string {
	var sb strings.Builder

	sb.WriteString("------------------------------------------------------------\n")
	sb.WriteString("UTCP TOOL REFERENCE (INPUT + OUTPUT SCHEMAS)\n")
	sb.WriteString("Use EXACT field names listed below. Do NOT invent new keys.\n")
	sb.WriteString("------------------------------------------------------------\n\n")

	for _, t := range specs {

		sb.WriteString(fmt.Sprintf("TOOL: %s\n", t.Name))
		sb.WriteString(fmt.Sprintf("DESCRIPTION: %s\n\n", t.Description))

		// -------------------------------
		// INPUT FIELD LIST
		// -------------------------------
		sb.WriteString("INPUT FIELDS (USE EXACTLY THESE KEYS):\n")

		if len(t.Inputs.Properties) == 0 {
			sb.WriteString("- (no fields)\n")
		} else {
			for key, raw := range t.Inputs.Properties {

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
	var sb strings.Builder

	sb.WriteString("AVAILABLE UTCP TOOLS:\n")
	for _, t := range specs {
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
		if specs, catalog := cm.cache.GetToolSpecsAndCatalog(); specs != nil {
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

func (cm *CodeModeUTCP) decideIfToolsNeeded(
	ctx context.Context,
	query string,
	tools string,
) (bool, error) {

	prompt := fmt.Sprintf(`
Decide if the following user query requires using ANY UTCP tools.

USER QUERY:
%q

AVAILABLE UTCP TOOLS:
%s

Respond ONLY in JSON:
{ "needs": true } or { "needs": false }
`, query, tools)

	raw, err := cm.model.Generate(ctx, prompt)
	if err != nil {
		return false, err
	}

	jsonStr := extractJSON(fmt.Sprint(raw))
	if jsonStr == "" {
		return false, nil
	}

	var resp struct {
		Needs bool `json:"needs"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return false, nil
	}

	return resp.Needs, nil
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

func (cm *CodeModeUTCP) selectTools(
	ctx context.Context,
	query string,
	tools string,
) ([]string, error) {

	// Check cache first
	if cm.cache != nil {
		if cached := cm.cache.GetSelectedTools(query, tools); cached != nil {
			return cached, nil
		}
	}

	prompt := fmt.Sprintf(`
Select the UTCP tools required to fulfill the user's intent.

USER QUERY:
%q

AVAILABLE UTCP TOOLS:
%s

Respond ONLY in JSON:
{
  "tools": ["provider.tool", ...]
}

Rules:
- Use ONLY names listed above.
- NO modifications, NO guessing.
- Include every tool required for the request, and no unrelated tools.
- If no available tool is required, return an empty array: {"tools": []}.
`, query, tools)

	raw, err := cm.model.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}

	jsonStr := extractJSON(fmt.Sprint(raw))
	if jsonStr == "" {
		return nil, nil
	}

	var resp struct {
		Tools []string `json:"tools"`
	}

	_ = json.Unmarshal([]byte(jsonStr), &resp)

	// Store in cache
	if cm.cache != nil && resp.Tools != nil {
		cm.cache.SetSelectedTools(query, tools, resp.Tools)
	}

	return resp.Tools, nil
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
