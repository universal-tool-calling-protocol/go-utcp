package utcp

// ChainStep defines one step in a Go-native UTCP tool chain.
type ChainStep struct {
	ID          string         `json:"id,omitempty"` // alias for this step
	ToolName    string         `json:"tool_name"`
	Inputs      map[string]any `json:"inputs,omitempty"`
	UsePrevious bool           `json:"use_previous,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
}
