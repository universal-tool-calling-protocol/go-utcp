package helpers

import (
	"io"
	"strings"
	"testing"
)

func TestDecodeToolsResponse(t *testing.T) {
	jsonData := `{"tools":[{"name":"t1","description":"d1"}]}`
	tools, err := DecodeToolsResponse(io.NopCloser(strings.NewReader(jsonData)))
	if err != nil {
		t.Fatalf("DecodeToolsResponse error: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "t1" || tools[0].Description != "d1" {
		t.Fatalf("unexpected tool: %+v", tools[0])
	}
}

func TestDecodeToolsResponse_BadJSON(t *testing.T) {
	_, err := DecodeToolsResponse(io.NopCloser(strings.NewReader("bad")))
	if err == nil {
		t.Fatalf("expected error for bad JSON")
	}
}
