package openapi

import (
	"testing"
)

func TestOptionalString(t *testing.T) {
	if optionalString("") != nil {
		t.Fatalf("expected nil for empty string")
	}
	val := optionalString("x")
	if val == nil || *val != "x" {
		t.Fatalf("unexpected value: %v", val)
	}
}
