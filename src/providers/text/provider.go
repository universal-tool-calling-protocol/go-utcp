package text

import (
	"encoding/json"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// TextProvider represents a provider that serves text templates.
type TextProvider struct {
	BaseProvider
	Templates map[string]string `json:"templates"`
}

func UnmarshalTextProvider(data []byte) (*TextProvider, error) {
	p := &TextProvider{}
	if err := json.Unmarshal(data, p); err != nil {
		return nil, err
	}
	if p.ProviderType == "" {
		p.ProviderType = ProviderText
	}
	return p, nil
}
