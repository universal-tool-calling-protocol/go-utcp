package text

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestTextProvider_Basic(t *testing.T) {
	p := &TextProvider{
		BaseProvider: BaseProvider{Name: "txt", ProviderType: ProviderText},
		Templates:    map[string]string{"hello": "Hello"},
	}
	if p.Type() != ProviderText {
		t.Fatalf("Type mismatch")
	}
	if p.Templates["hello"] != "Hello" {
		t.Fatalf("template not set")
	}
}
