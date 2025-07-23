package manual

import "testing"

func TestNewOpenAPIConverter(t *testing.T) {
	c := NewOpenAPIConverter(nil, "u", "n")
	if c.Url != "u" || c.Name != "n" || c.Raw != nil {
		t.Fatalf("unexpected converter: %+v", c)
	}
}
