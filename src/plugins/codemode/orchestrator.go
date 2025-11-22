package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

func (cm *CodeModeUTCP) CallTool(
	ctx context.Context,
	prompt string,
) (bool, any, error) {

	toolSpecs := cm.ToolSpecs()
	detailed := renderUtcpToolsForPrompt(toolSpecs)

	// --------------------------------------------
	// 1) Decide whether tools are needed
	// --------------------------------------------
	need, err := cm.decideIfToolsNeeded(ctx, prompt, detailed)
	if err != nil {
		return false, "", err
	}
	if !need {
		return false, "", nil
	}

	// --------------------------------------------
	// 2) Select tools (exact names)
	// --------------------------------------------
	selected, err := cm.selectTools(ctx, prompt, detailed)
	if err != nil {
		return true, "", err
	}
	if len(selected) == 0 {
		return false, "", nil
	}

	// --------------------------------------------
	// 3) Generate snippet using chosen tools only
	// --------------------------------------------
	snippet, ok, err := cm.generateSnippet(ctx, prompt, selected, detailed)
	if err != nil && !ok {
		return true, "", err
	}

	// --------------------------------------------
	// 4) Execute snippet via CodeMode UTCP
	// --------------------------------------------
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

func (cm *CodeModeUTCP) generateSnippet(
	ctx context.Context,
	query string,
	tools []string,
	toolSpecs string,
) (string, bool, error) {

	toolsJSON, _ := json.Marshal(tools)

	prompt := fmt.Sprintf(`
CRITICAL: Your code will be executed INSIDE a function wrapper.

Generate a Go snippet that uses ONLY the following UTCP tools:
%v

USER QUERY:
%q

TOOL SPECS:
%s

============================================================
CRITICAL RULES (VIOLATIONS WILL CAUSE RUNTIME ERRORS)
============================================================

1. SCOPE CONTEXT
   - Your code executes INSIDE a function body - write ONLY function body code
   - NEVER include: package declarations, import statements, func definitions
   - The wrapper already provides: package main, imports, and func run()

2. VARIABLE DECLARATIONS
   - For new variables use 'var' keyword OR walrus operator ':='
   - CORRECT: var result any
   - CORRECT: result := someValue
   - CORRECT: r1, err := codemode.CallTool(...)
   - WRONG: result any (missing 'var')
   
3. __out ASSIGNMENT (MOST CRITICAL)
   - __out is ALREADY DECLARED in the wrapper - DO NOT declare it again
   - ALWAYS use '=' (assignment) NEVER ':=' (declaration+assignment)
   - CORRECT: __out = result
   - CORRECT: __out = map[string]any{"key": value}
   - WRONG: var __out any (already declared)
   - WRONG: __out := result (redeclaration error)
   
4. MULTI-VALUE ASSIGNMENT WITH __out
   - If assigning multiple values including __out, declare error first:
     CORRECT:
       var err error
       __out, err = codemode.CallTool(...)
   - Or use separate lines:
     CORRECT:
       result, err := codemode.CallTool(...)
       __out = result

5. TOOL USAGE
   - Use ONLY the tool names listed above - NO modifications
   - Use EXACT input keys from tool schemas - NO invented fields
   - Available helpers: codemode.CallTool, codemode.CallToolStream, 
     codemode.SearchTools, codemode.Sprintf, codemode.Errorf
   - sync.WaitGroup and sync.Mutex are available (no import needed)

6. FINAL RESULT
   - The LAST line must assign to __out
   - __out should contain all intermediate and final results
   - If streaming tools used, set "stream": true in response

============================================================
EXAMPLES - LEARN FROM THESE
============================================================

EXAMPLE 1: Simple Tool Call
----------------------------
result, err := codemode.CallTool("math.add", map[string]any{
    "a": 5,
    "b": 7,
})
if err != nil {
    return err
}
__out = result

EXAMPLE 2: Chaining Tools (Extract Output → Pass to Next)
----------------------------------------------------------
// Step 1: Call first tool
r1, err := codemode.CallTool("math.add", map[string]any{
    "a": 5,
    "b": 7,
})
if err != nil {
    return err
}

// Step 2: Extract value using EXACT output schema key
var sum any
if m, ok := r1.(map[string]any); ok {
    sum = m["result"]  // 'result' must match tool's output schema
}

// Step 3: Use extracted value in next tool
r2, err := codemode.CallTool("math.multiply", map[string]any{
    "a": sum,
    "b": 3,
})
if err != nil {
    return err
}

// Step 4: Assign final result (USE = NOT :=)
__out = map[string]any{
    "sum": sum,
    "product": r2,
}

EXAMPLE 3: Parallel Tool Calls
-------------------------------
var wg sync.WaitGroup
var mu sync.Mutex
results := map[string]any{}

// Launch parallel calls
wg.Add(2)

go func() {
    defer wg.Done()
    r1, err := codemode.CallTool("api.fetch", map[string]any{"url": "example.com"})
    if err != nil {
        return
    }
    mu.Lock()
    results["fetch"] = r1
    mu.Unlock()
}()

go func() {
    defer wg.Done()
    r2, err := codemode.CallTool("db.query", map[string]any{"table": "users"})
    if err != nil {
        return
    }
    mu.Lock()
    results["query"] = r2
    mu.Unlock()
}()

wg.Wait()
__out = results

EXAMPLE 4: Streaming Tool
--------------------------
stream, err := codemode.CallToolStream("api.stream", map[string]any{
    "input": "hello",
})
if err != nil {
    return err
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

EXAMPLE 5: Streaming → Non-Streaming Chain
-------------------------------------------
// Collect streaming results
stream, err := codemode.CallToolStream("logs.fetch", map[string]any{
    "source": "server",
})
if err != nil {
    return err
}

var logs []any
for {
    chunk, err := stream.Next()
    if err != nil {
        break
    }
    logs = append(logs, chunk)
}

// Pass to summarization tool
summary, err := codemode.CallTool("text.summarize", map[string]any{
    "items": logs,
})
if err != nil {
    return err
}

__out = map[string]any{
    "log_count": len(logs),
    "summary": summary,
}

============================================================
COMMON MISTAKES TO AVOID
============================================================

❌ WRONG: __out := result
✅ CORRECT: __out = result

❌ WRONG: var __out any
✅ CORRECT: (don't declare __out at all, just assign to it)

❌ WRONG: __out, err := codemode.CallTool(...)
✅ CORRECT: 
   var err error
   __out, err = codemode.CallTool(...)

❌ WRONG: package main
✅ CORRECT: (no package declaration)

❌ WRONG: import "fmt"
✅ CORRECT: (no imports)

❌ WRONG: func run() { ... }
✅ CORRECT: (no function definitions)

❌ WRONG: return (bare return)
✅ CORRECT: return err (or return __out if needed)

❌ WRONG: map[string]any{"key" value}  (missing colon)
✅ CORRECT: map[string]any{"key": value}

❌ WRONG: var result := value
✅ CORRECT: result := value  OR  var result = value

============================================================
RESPONSE FORMAT
============================================================

Respond ONLY in JSON:
{
  "code": "<go snippet>",
  "stream": false
}

Set "stream": true ONLY if you use codemode.CallToolStream.

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
	if !isValidSnippet(resp.Code) {
		log.Println("Skipping invalid snippet:", resp.Code)
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

func (a *CodeModeUTCP) ToolSpecs() []tools.Tool {
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
	// invalid if LLM emits standalone maps like: map[value:hello world]
	if strings.Contains(code, "map[value:") {
		return false
	}

	// invalid if no __out assignment exists
	if !strings.Contains(code, "__out") {
		return false
	}

	return true
}

func (cm *CodeModeUTCP) selectTools(
	ctx context.Context,
	query string,
	tools string,
) ([]string, error) {

	prompt := fmt.Sprintf(`
Select ALL UTCP tools that match the user's intent.

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
- If multiple tools apply, include all.
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
	return resp.Tools, nil
}
