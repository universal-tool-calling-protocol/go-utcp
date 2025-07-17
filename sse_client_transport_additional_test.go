package UTCP

import (
	"io"
	"strings"
	"testing"
)

func TestDecodeToolsResponse(t *testing.T) {
	r := io.NopCloser(strings.NewReader(`{"tools":[{"name":"t","description":"d"}]}`))
	tools, err := decodeToolsResponse(r)
	if err != nil || len(tools) != 1 || tools[0].Name != "t" {
		t.Fatalf("decode err %v tools %+v", err, tools)
	}
}

func TestDecodeToolsResponse_Error(t *testing.T) {
	r := io.NopCloser(strings.NewReader(`bad`))
	if _, err := decodeToolsResponse(r); err == nil {
		t.Fatalf("expected error for bad json")
	}
}
