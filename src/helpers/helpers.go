package helpers

import (
	"encoding/json"
	"io"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// decodeToolsResponse parses a common tools discovery response.
func DecodeToolsResponse(r io.ReadCloser) ([]Tool, error) {
	defer r.Close()
	var resp struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		return nil, err
	}
	return resp.Tools, nil
}
