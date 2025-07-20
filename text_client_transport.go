package utcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
)

// TextClientTransport is a simple in-memory/text-based transport.
type TextClientTransport struct {
	prefix string
	tools  map[string]Tool
}

// NewTextTransport constructs a TextClientTransport.
func NewTextTransport(prefix string) *TextClientTransport {
	return &TextClientTransport{
		prefix: prefix,
		tools:  make(map[string]Tool),
	}
}

// RegisterHandler associates a handler function with a tool name
func (t *TextClientTransport) RegisterHandler(toolName string, handler ToolHandler) error {
	tool, exists := t.tools[toolName]
	if !exists {
		return fmt.Errorf("tool %q not found", toolName)
	}
	tool.Handler = handler
	t.tools[toolName] = tool
	return nil
}

// RegisterHandlers associates multiple handlers at once
func (t *TextClientTransport) RegisterHandlers(handlers map[string]ToolHandler) error {
	for toolName, handler := range handlers {
		if err := t.RegisterHandler(toolName, handler); err != nil {
			return err
		}
	}
	return nil
}

// RegisterToolProvider loads tool definitions from a local text (JSON) file.
func (t *TextClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	textProv, ok := prov.(*TextProvider)
	if !ok {
		return nil, errors.New("TextClientTransport can only be used with TextProvider")
	}

	data, err := ioutil.ReadFile(textProv.FilePath)
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	manual := NewUtcpManualFromMap(raw)
	prefix := textProv.Name + "."

	t.tools = make(map[string]Tool, len(manual.Tools))
	for i, tool := range manual.Tools {
		name := tool.Name
		if !strings.HasPrefix(name, prefix) {
			name = prefix + name
		}
		tool.Name = name

		// CRITICAL FIX: Ensure Handler is not nil
		if tool.Handler == nil {
			// For now, just log the issue and continue - handler will be checked in CallTool
			fmt.Printf("Warning: tool %q has no handler implementation\n", tool.Name)
		}

		t.tools[name] = tool
		manual.Tools[i] = tool
	}

	return manual.Tools, nil
}

// DeregisterToolProvider cleans up in-memory tools.
func (t *TextClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	t.tools = make(map[string]Tool)
	return nil
}

// CallTool invokes a named in-memory tool handler.
func (t *TextClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov Provider, l *string) (interface{}, error) {
	tool, ok := t.tools[toolName]
	if !ok {
		return nil, fmt.Errorf("tool %q not registered", toolName)
	}

	// CRITICAL: Return error immediately if handler is nil
	if tool.Handler == nil {
		return nil, fmt.Errorf("tool %q has no handler implementation", toolName)
	}

	// Try the original call signature first (seems like it expects nil as first param)
	return tool.Handler(nil, args)
}
