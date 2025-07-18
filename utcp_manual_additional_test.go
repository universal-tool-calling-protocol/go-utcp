package utcp

import "testing"

func TestNewOpenAPIConverter(t *testing.T) {
	c := NewOpenAPIConverter(nil, "u", "n")
	if c.url != "u" || c.name != "n" || c.raw != nil {
		t.Fatalf("unexpected converter: %+v", c)
	}
}
