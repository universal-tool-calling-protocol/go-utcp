package codemode

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
)

// extractJSON extracts the first complete JSON object from model output.
// Models may wrap the plan in prose or markdown, so only the balanced JSON
// object is passed to encoding/json.
func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Strip markdown fences if the model ignored the instruction.
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
				lines = lines[:len(lines)-1]
			}
			raw = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}

	start := strings.IndexByte(raw, '{')
	if start < 0 {
		return ""
	}

	// Do not validate JSON here. json.Unmarshal is responsible for that.
	// We only locate the candidate JSON object.
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(raw); i++ {
		c := raw[i]

		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1]
			}
		}
	}

	// There was an object start, but it was malformed/incomplete.
	// Return the candidate so json.Unmarshal produces the useful error.
	return raw[start:]
}

// isValidSnippet validates generated source against the Expr runtime rather
// than treating it as Go source. The runtime API is represented by a small
// compile-time environment; execution supplies the real implementation.
func isValidSnippet(code string) bool {
	code = normalizeSnippet(code)
	if strings.TrimSpace(code) == "" {
		return false
	}

	// Keep the generated program closed over the CodeMode API. These checks
	// provide an explicit boundary in addition to Expr compilation.
	for _, forbidden := range []string{
		"package ", "import ", "__out", "stream.Next(",
		"codemode.SearchTools(", "codemode.Sprintf(", "codemode.Errorf(",
		"fmt.Sprintf(", "fmt.Errorf(",
	} {
		if strings.Contains(code, forbidden) {
			return false
		}
	}
	if strings.Contains(code, ":=") {
		return false
	}
	if strings.Contains(code, "return ") || strings.HasPrefix(strings.TrimSpace(code), "return") {
		return false
	}

	// Compile with a minimal environment exposing exactly the supported API.
	env := map[string]any{"codemode": validationCodeModeAPI{}}
	if _, err := expr.Compile(code, expr.Env(env), expr.AsAny()); err != nil {
		return false
	}
	return true
}

type validationCodeModeAPI struct{}

func (validationCodeModeAPI) CallTool(string, map[string]any) (any, error) {
	return nil, fmt.Errorf("validation runtime")
}
func (validationCodeModeAPI) CallToolStream(string, map[string]any) ([]any, error) {
	return nil, fmt.Errorf("validation runtime")
}
func (validationCodeModeAPI) Get(any, string) any { return nil }
