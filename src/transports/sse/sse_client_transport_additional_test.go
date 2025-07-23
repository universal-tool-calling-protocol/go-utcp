package sse

import (
	"io"
	"strings"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/concepts"
)

func TestDecodeToolsResponse(t *testing.T) {
	r := io.NopCloser(strings.NewReader(`{"tools":[{"name":"t","description":"d"}]}`))
	tools, err := DecodeToolsResponse(r)
	if err != nil || len(tools) != 1 || tools[0].Name != "t" {
		t.Fatalf("decode err %v tools %+v", err, tools)
	}
}

func TestDecodeToolsResponse_Error(t *testing.T) {
	r := io.NopCloser(strings.NewReader(`bad`))
	if _, err := DecodeToolsResponse(r); err == nil {
		t.Fatalf("expected error for bad json")
	}
}
