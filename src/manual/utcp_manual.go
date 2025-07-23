package manual

// OpenAPIConverter helps converting OpenAPI specs into a UtcpManual.
type OpenAPIConverter struct {
	Raw  interface{}
	Url  string
	Name string
}

// NewOpenAPIConverter creates a new converter for OpenAPI raw definitions.
func NewOpenAPIConverter(raw interface{}, url, name string) *OpenAPIConverter {
	return &OpenAPIConverter{Raw: raw, Url: url, Name: name}
}
