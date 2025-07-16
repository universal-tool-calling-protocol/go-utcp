package UTCP

import (
	"encoding/json"
	"fmt"
)

// UtcpManual represents a manual with a version and a set of tools.
type UtcpManual struct {
	Version string
	Tools   []Tool
	Name    string // optional, for OpenAPI-derived manuals
}

// NewUtcpManualFromMap constructs a UtcpManual from a raw map representation.
func NewUtcpManualFromMap(m map[string]interface{}) UtcpManual {
	manual := UtcpManual{}
	if v, ok := m["version"].(string); ok {
		manual.Version = v
	}

	// Parse tools array if present
	if rawTools, ok := m["tools"].([]interface{}); ok {
		for _, rt := range rawTools {
			if tm, ok := rt.(map[string]interface{}); ok {
				t := Tool{}
				if name, ok := tm["name"].(string); ok {
					t.Name = name
				}
				if desc, ok := tm["description"].(string); ok {
					t.Description = desc
				}
				manual.Tools = append(manual.Tools, t)
			}
		}
	}
	return manual
}

// Convert processes the OpenAPI spec and returns a UtcpManual.
func (c *OpenAPIConverter) Convert() UtcpManual {
	manual := UtcpManual{Name: c.name}

	// Attempt to coerce raw into a map
	var specMap map[string]interface{}
	switch v := c.raw.(type) {
	case map[string]interface{}:
		specMap = v
	case []byte:
		if err := json.Unmarshal(v, &specMap); err != nil {
			fmt.Printf("warning: failed to unmarshal OpenAPI bytes: %v", err)
			return manual
		}
	case string:
		if err := json.Unmarshal([]byte(v), &specMap); err != nil {
			fmt.Printf("warning: failed to unmarshal OpenAPI string: %v", err)
			return manual
		}
	default:
		fmt.Printf("warning: unsupported OpenAPI raw type %T", v)
		return manual
	}

	// Extract version from "openapi" field if present
	if v, ok := specMap["openapi"].(string); ok {
		manual.Version = v
	}

	// Example: convert each path into a Tool
	if paths, ok := specMap["paths"].(map[string]interface{}); ok {
		for path, entry := range paths {
			t := Tool{Name: path}
			// You could inspect entry (methods, parameters, descriptions, etc.)
			if ep, ok := entry.(map[string]interface{}); ok {
				if getOp, ok := ep["get"].(map[string]interface{}); ok {
					if desc, ok := getOp["description"].(string); ok {
						t.Description = desc
					}
				}
			}
			manual.Tools = append(manual.Tools, t)
		}
	}

	return manual
}

const Version = "1.0"
