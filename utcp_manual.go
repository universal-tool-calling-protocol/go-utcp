package UTCP

// OpenAPIConverter helps converting OpenAPI specs into a UtcpManual.
type OpenAPIConverter struct {
	raw  interface{}
	url  string
	name string
}

// NewOpenAPIConverter creates a new converter for OpenAPI raw definitions.
func NewOpenAPIConverter(raw interface{}, url, name string) *OpenAPIConverter {
	return &OpenAPIConverter{raw: raw, url: url, name: name}
}
