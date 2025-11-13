package utcp

// ChainStep defines one step in a Go-native UTCP tool chain.
type ChainStep struct {
	ToolName    string         `json:"tool_name"`
	Inputs      map[string]any `json:"inputs,omitempty"`
	UsePrevious bool           `json:"use_previous,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
}
