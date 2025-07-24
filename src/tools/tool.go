package tools

import (
	"reflect"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// ProviderUnion is your existing Go interface/union for providers.
type ToolProvider interface {
	Type() string
	Name() string
}

// ToolInputOutputSchema mirrors your JSON schema description.
type ToolInputOutputSchema struct {
	Type        string                 `json:"type"`                 // e.g. "object", "array", "string"
	Properties  map[string]interface{} `json:"properties,omitempty"` // field schemas
	Required    []string               `json:"required,omitempty"`
	Description string                 `json:"description,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Items       map[string]interface{} `json:"items,omitempty"` // for arrays
	Enum        []interface{}          `json:"enum,omitempty"`
	Minimum     *float64               `json:"minimum,omitempty"`
	Maximum     *float64               `json:"maximum,omitempty"`
	Format      string                 `json:"format,omitempty"` // e.g. "date-time"
}

// Tool holds the metadata for a single UTCP tool.
type Tool struct {
	Name                string                `json:"name"`
	Description         string                `json:"description"`
	Inputs              ToolInputOutputSchema `json:"inputs"`
	Outputs             ToolInputOutputSchema `json:"outputs"`
	Tags                []string              `json:"tags"`
	AverageResponseSize *int                  `json:"average_response_size,omitempty"`
	Provider            Provider              `json:"tool_provider"`
	Handler             ToolHandler           `json:"-"`
}

// ToolHandler is the signature your Go tool functions must satisfy.
// The first argument is your execution context (if any), here we use a generic map.
// You can replace `map[string]interface{}` with a concrete struct or interface as needed.
type ToolHandler func(ctx map[string]interface{}, inputs map[string]interface{}) (outputs map[string]interface{}, err error)

// ToolContext keeps the registry of all tools.
var (
	Tools []Tool
)

// AddTool registers a new tool in the global context.
func AddTool(t Tool) {
	if t.Name == "" {
		panic("tool must have a name")
	}
	Tools = append(Tools, t)
}

// GetTools returns all registered tools.
func GetTools() []Tool {
	return Tools
}

// RegisterTool is the Go equivalent of your @utcp_tool decorator.
// Call this from an init() function in the same package as your handler.
func RegisterTool(
	provider Provider,
	name string,
	description string,
	tags []string,
	inputs *ToolInputOutputSchema,
	outputs *ToolInputOutputSchema,
	handler ToolHandler,
) {
	// Populate defaults if necessary
	if inputs == nil {
		inputs = inferSchema(handler, true)
		inputs.Title = name
		inputs.Description = description
	}
	if outputs == nil {
		outputs = inferSchema(handler, false)
		outputs.Title = name
		outputs.Description = description
	}

	tool := Tool{
		Name:        name,
		Description: description,
		Inputs:      *inputs,
		Outputs:     *outputs,
		Tags:        tags,
		Provider:    provider,
		Handler:     handler,
	}
	AddTool(tool)
}

// inferSchema uses reflection to build a minimal schema for inputs or outputs.
// For real JSON‑schema generation you’d plug in a library; here’s a stub.
func inferSchema(handler interface{}, forInputs bool) *ToolInputOutputSchema {
	// This is just a placeholder; you’d replace with real logic
	schema := &ToolInputOutputSchema{
		Type:       "object",
		Properties: map[string]interface{}{},
		Required:   []string{},
	}
	// Example: look at the function signature
	fnType := reflect.TypeOf(handler)
	if fnType.Kind() == reflect.Func {
		var idx int
		if forInputs {
			// first argument is ctx, skip it
			idx = 1
		} else {
			// outputs: assume single return value of map[string]interface{}
			idx = -1
		}
		if forInputs && fnType.NumIn() > idx {
			// You could inspect fnType.In(idx) here
		}
		if !forInputs && fnType.NumOut() > 0 {
			// Inspect fnType.Out(0)
		}
	}
	return schema
}
