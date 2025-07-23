package concepts

import (
	"encoding/json"
	"testing"
)

func TestNewUtcpManualFromMap(t *testing.T) {
	toolsData := []interface{}{map[string]interface{}{"name": "ToolA", "description": "DescA"}, map[string]interface{}{"name": "ToolB"}}
	m := map[string]interface{}{
		"version": "v2.3",
		"tools":   toolsData,
	}
	manual := NewUtcpManualFromMap(m)

	if manual.Version != "v2.3" {
		t.Errorf("expected version v2.3, got %s", manual.Version)
	}

	if len(manual.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(manual.Tools))
	}

	expectedNames := []string{"ToolA", "ToolB"}
	expectedDescs := []string{"DescA", ""}
	for i, tool := range manual.Tools {
		if tool.Name != expectedNames[i] {
			t.Errorf("tool %d name: expected %s; got %s", i, expectedNames[i], tool.Name)
		}
		if tool.Description != expectedDescs[i] {
			t.Errorf("tool %d description: expected %s; got %s", i, expectedDescs[i], tool.Description)
		}
	}
}

func TestNewUtcpManualFromMapEmpty(t *testing.T) {
	manual := NewUtcpManualFromMap(map[string]interface{}{})
	if manual.Version != "" {
		t.Errorf("expected empty version, got %s", manual.Version)
	}
	if len(manual.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(manual.Tools))
	}
}

func TestOpenAPIConverterConvert_MapRaw(t *testing.T) {
	// prepare a minimal OpenAPI spec
	spec := map[string]interface{}{
		"openapi": "3.0.1",
		"paths": map[string]interface{}{
			"/example": map[string]interface{}{
				"get": map[string]interface{}{"description": "Example endpoint"},
			},
		},
	}
	converter := &OpenAPIConverter{name: "MyAPI", raw: spec}
	manual := converter.Convert()

	if manual.Name != "MyAPI" {
		t.Errorf("expected Name MyAPI, got %s", manual.Name)
	}
	if manual.Version != "3.0.1" {
		t.Errorf("expected Version 3.0.1, got %s", manual.Version)
	}
	if len(manual.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(manual.Tools))
	}
	tool := manual.Tools[0]
	if tool.Name != "/example" {
		t.Errorf("expected tool Name /example, got %s", tool.Name)
	}
	if tool.Description != "Example endpoint" {
		t.Errorf("expected tool Description Example endpoint, got %s", tool.Description)
	}
}

func TestOpenAPIConverterConvert_JSONBytes(t *testing.T) {
	spec := map[string]interface{}{"openapi": "2.0"}
	b, _ := json.Marshal(spec)
	converter := &OpenAPIConverter{name: "BytesAPI", raw: b}
	manual := converter.Convert()

	if manual.Version != "2.0" {
		t.Errorf("expected Version 2.0, got %s", manual.Version)
	}
}

func TestOpenAPIConverterConvert_JSONString(t *testing.T) {
	s := `{"openapi":"1.2"}`
	converter := &OpenAPIConverter{name: "StringAPI", raw: s}
	manual := converter.Convert()

	if manual.Version != "1.2" {
		t.Errorf("expected Version 1.2, got %s", manual.Version)
	}
}

func TestOpenAPIConverterConvert_UnsupportedType(t *testing.T) {
	converter := &OpenAPIConverter{name: "BadAPI", raw: 123}
	manual := converter.Convert()

	if manual.Name != "BadAPI" {
		t.Errorf("expected Name BadAPI, got %s", manual.Name)
	}
	if manual.Version != "" {
		t.Errorf("expected empty Version, got %s", manual.Version)
	}
}
