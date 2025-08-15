package text

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/text"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

// TextTransport executes tools defined as text templates.
type TextTransport struct {
	log      func(string, ...interface{})
	basePath string
}

// NewTextTransport creates a new TextTransport.
func NewTextTransport(logger func(string, ...interface{})) *TextTransport {
	if logger == nil {
		logger = func(string, ...interface{}) {}
	}
	return &TextTransport{log: logger}
}

// SetBasePath sets the base path for resolving files (unused but kept for compatibility).
func (t *TextTransport) SetBasePath(path string) { t.basePath = path }

// RegisterToolProvider generates tools from the provider's templates.
func (t *TextTransport) RegisterToolProvider(ctx context.Context, manual Provider) ([]Tool, error) {
	p, ok := manual.(*providers.TextProvider)
	if !ok {
		return nil, fmt.Errorf("unsupported provider type %T", manual)
	}
	tools := make([]Tool, 0, len(p.Templates))
	for name := range p.Templates {
		tools = append(tools, Tool{
			Name:        name,
			Description: "Text template tool",
			Inputs:      ToolInputOutputSchema{Type: "object"},
			Outputs:     ToolInputOutputSchema{Type: "string"},
		})
	}
	return tools, nil
}

// DeregisterToolProvider is a no-op for TextTransport.
func (t *TextTransport) DeregisterToolProvider(ctx context.Context, manual Provider) error {
	return nil
}

// CallTool renders the template with the provided arguments.
func (t *TextTransport) CallTool(ctx context.Context, toolName string, args map[string]any, manual Provider, l *string) (any, error) {
	p, ok := manual.(*providers.TextProvider)
	if !ok {
		return nil, fmt.Errorf("unsupported provider type %T", manual)
	}
	tmplStr, ok := p.Templates[toolName]
	if !ok {
		return nil, fmt.Errorf("tool %s not found", toolName)
	}
	tpl, err := template.New(toolName).Parse(tmplStr)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, args); err != nil {
		return nil, err
	}
	return buf.String(), nil
}

// CallToolStream is not supported for TextTransport.
func (t *TextTransport) CallToolStream(ctx context.Context, toolName string, args map[string]any, p Provider) (transports.StreamResult, error) {
	return nil, fmt.Errorf("streaming not supported")
}
