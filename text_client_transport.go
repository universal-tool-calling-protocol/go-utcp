package utcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
)

// TextClientTransport is a simple in-memory/text-based transport.
type TextClientTransport struct {
	prefix   string
	basePath string
	tools    map[string]Tool
}

// SetBasePath allows the UTCP client to set the directory for relative file paths.
func (t *TextClientTransport) SetBasePath(path string) {
	t.basePath = path
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
	path := textProv.FilePath
	if t.basePath != "" && !filepath.IsAbs(path) {
		path = filepath.Join(t.basePath, path)
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	manual := NewUtcpManualFromMap(raw)
	t.tools = make(map[string]Tool, len(manual.Tools))
	for i, tool := range manual.Tools {
		fullName := textProv.Name + "." + tool.Name
		tool.Name = fullName
		if fullName == textProv.Name+".hello" {
			tool.Handler = func(_ map[string]interface{}, args map[string]interface{}) (map[string]interface{}, error) {
				name, _ := args["name"].(string)
				return map[string]interface{}{"greeting": fmt.Sprintf("Hello, %s!", name)}, nil
			}
		}
		manual.Tools[i] = tool
		t.tools[fullName] = tool
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
