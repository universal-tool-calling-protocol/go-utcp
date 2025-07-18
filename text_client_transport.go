package utcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
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
	t.tools = make(map[string]Tool, len(manual.Tools))
	for _, tool := range manual.Tools {
		t.tools[tool.Name] = tool
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
	return tool.Handler(nil, args)
}
