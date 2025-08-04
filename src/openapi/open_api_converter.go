package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/manual"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// sanitizeName converts a path into a safe identifier: strips braces, replaces slashes and invalid chars with underscores, collapses repeats.
func sanitizeName(p string) string {
	out := strings.ReplaceAll(p, "{", "")
	out = strings.ReplaceAll(out, "}", "")
	out = strings.ReplaceAll(out, "/", "_")
	// replace any non-alphanumeric or underscore with underscore
	valid := func(r rune) rune {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '_' {
			return r
		}
		return '_'
	}
	out = strings.Map(valid, out)
	// collapse multiple underscores
	out = collapseUnderscores(out)
	out = strings.Trim(out, "_")
	if out == "" {
		return "root"
	}
	return out
}

func collapseUnderscores(s string) string {
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	return s
}

// OpenApiConverter converts an OpenAPI JSON/YAML spec into a UtcpManual.
type OpenApiConverter struct {
	spec         map[string]interface{}
	specURL      string
	providerName string
	nameCounts   map[string]int
}

// NewConverter creates a new converter.
// If providerName is empty, it will be derived from spec.info.title.
func NewConverter(
	openapiSpec map[string]interface{},
	specURL string,
	providerName string,
) *OpenApiConverter {
	if providerName == "" {
		// derive from title
		info, _ := openapiSpec["info"].(map[string]interface{})
		title, _ := info["title"].(string)
		if title == "" {
			title = "openapi_provider"
		}
		invalid := " -.,!?'\"\\/()[]{}#@$%^&*+=~`|;:<>"
		providerName = strings.Map(func(r rune) rune {
			if strings.ContainsRune(invalid, r) {
				return '_'
			}
			return r
		}, title)
	}

	return &OpenApiConverter{
		spec:         openapiSpec,
		specURL:      specURL,
		providerName: providerName,
		nameCounts:   make(map[string]int),
	}
}

// NewConverterFromURL fetches the spec (YAML or JSON) from the given URL and returns a converter.
// providerName can be empty to auto-derive from the spec.
func NewConverterFromURL(specURL string, providerName string) (*OpenApiConverter, error) {
	spec, finalURL, err := LoadSpecFromURL(specURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load spec from URL %s: %w", specURL, err)
	}
	return NewConverter(spec, finalURL, providerName), nil
}

// LoadSpecFromURL fetches the content at the URL and attempts to decode it first as JSON,
// and if that fails, as YAML. Returns the spec as a map and the normalized URL used.
func LoadSpecFromURL(rawURL string) (map[string]interface{}, string, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return nil, rawURL, fmt.Errorf("http GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, rawURL, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, rawURL, fmt.Errorf("reading body failed: %w", err)
	}

	var spec map[string]interface{}
	// Try JSON first
	if err := json.Unmarshal(bodyBytes, &spec); err == nil {
		return spec, resp.Request.URL.String(), nil
	}

	// Fallback to YAML
	var yamlRaw interface{}
	if err := yaml.Unmarshal(bodyBytes, &yamlRaw); err != nil {
		return nil, rawURL, fmt.Errorf("failed to parse as JSON (%v) or YAML (%v)", err, err)
	}

	// Convert YAML parsed structure into map[string]interface{} with proper types (via intermediate JSON).
	intermediate, err := json.Marshal(yamlRaw)
	if err != nil {
		return nil, rawURL, fmt.Errorf("failed to re-marshal YAML content: %w", err)
	}
	if err := json.Unmarshal(intermediate, &spec); err != nil {
		return nil, rawURL, fmt.Errorf("failed to unmarshal intermediate YAML->JSON: %w", err)
	}

	return spec, resp.Request.URL.String(), nil
}

// Convert parses the OpenAPI spec and builds a UtcpManual.
func (c *OpenApiConverter) Convert() UtcpManual {
	var tools []Tool

	// determine baseURL
	baseURL := "/"
	if servers, ok := c.spec["servers"].([]interface{}); ok && len(servers) > 0 {
		if srv0, ok := servers[0].(map[string]interface{}); ok {
			if u, _ := srv0["url"].(string); u != "" {
				baseURL = u
			}
		}
	} else if c.specURL != "" {
		if pu, err := url.Parse(c.specURL); err == nil {
			baseURL = fmt.Sprintf("%s://%s", pu.Scheme, pu.Host)
		}
	}

	paths, _ := c.spec["paths"].(map[string]interface{})
	for rawPath, rawItem := range paths {
		if pathItem, ok := rawItem.(map[string]interface{}); ok {
			for method, rawOp := range pathItem {
				lm := strings.ToLower(method)
				if !(lm == "get" || lm == "post" || lm == "put" ||
					lm == "delete" || lm == "patch") {
					continue
				}
				if op, ok := rawOp.(map[string]interface{}); ok {
					if t, err := c.createTool(rawPath, method, op, baseURL); err == nil && t != nil {
						tools = append(tools, *t)
					}
				}
			}
		}
	}

	return UtcpManual{
		Tools:       tools,
		Provider:    c.providerName,
		OriginalURL: c.specURL,
	}
}

func (c *OpenApiConverter) resolveRef(ref string) (map[string]interface{}, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("unsupported external ref %q", ref)
	}
	parts := strings.Split(ref[2:], "/")
	node := c.spec
	for _, p := range parts {
		if next, ok := node[p].(map[string]interface{}); ok {
			node = next
		} else {
			return nil, fmt.Errorf("ref %q not found", ref)
		}
	}
	return node, nil
}

// resolveSchema recurses into maps and slices to inline any {"$ref":...}.
func (c *OpenApiConverter) resolveSchema(schema interface{}) interface{} {
	switch val := schema.(type) {
	case map[string]interface{}:
		if ref, has := val["$ref"].(string); has {
			if sub, err := c.resolveRef(ref); err == nil {
				return c.resolveSchema(sub)
			}
			return val
		}
		out := make(map[string]interface{}, len(val))
		for k, v := range val {
			out[k] = c.resolveSchema(v)
		}
		return out

	case []interface{}:
		for i, item := range val {
			val[i] = c.resolveSchema(item)
		}
		return val

	default:
		return val
	}
}

// extractAuth pulls the first security requirement and builds an auth.Auth.
func (c *OpenApiConverter) extractAuth(operation map[string]interface{}) Auth {
	var reqs []interface{}
	if opSec, ok := operation["security"].([]interface{}); ok && len(opSec) > 0 {
		reqs = opSec
	} else if globalSec, ok := c.spec["security"].([]interface{}); ok {
		reqs = globalSec
	}
	if len(reqs) == 0 {
		return nil
	}

	schemes := c.getSecuritySchemes()
	for _, raw := range reqs {
		if secMap, ok := raw.(map[string]interface{}); ok {
			for name := range secMap {
				if scheme, found := schemes[name]; found {
					if authObj := c.createAuthFromScheme(scheme.(map[string]interface{})); authObj != nil {
						return authObj
					}
				}
			}
		}
	}
	return nil
}

// getSecuritySchemes reads either components.securitySchemes or securityDefinitions.
func (c *OpenApiConverter) getSecuritySchemes() map[string]interface{} {
	if comp, ok := c.spec["components"].(map[string]interface{}); ok {
		if schemes, ok := comp["securitySchemes"].(map[string]interface{}); ok {
			return schemes
		}
	}
	if defs, ok := c.spec["securityDefinitions"].(map[string]interface{}); ok {
		return defs
	}
	return nil
}

// createAuthFromScheme inspects a single OAS security-scheme object.
func (c *OpenApiConverter) createAuthFromScheme(scheme map[string]interface{}) Auth {
	typ, _ := scheme["type"].(string)
	switch strings.ToLower(typ) {
	case "apikey":
		loc, _ := scheme["in"].(string)
		name, _ := scheme["name"].(string)
		return &ApiKeyAuth{
			AuthType: APIKeyType,
			APIKey:   fmt.Sprintf("${%s_API_KEY}", strings.ToUpper(c.providerName)),
			VarName:  name,
			Location: loc,
		}

	case "basic":
		return &BasicAuth{
			AuthType: BasicType,
			Username: fmt.Sprintf("${%s_USERNAME}", strings.ToUpper(c.providerName)),
			Password: fmt.Sprintf("${%s_PASSWORD}", strings.ToUpper(c.providerName)),
		}

	case "http":
		schemeName, _ := scheme["scheme"].(string)
		switch strings.ToLower(schemeName) {
		case "basic":
			return &BasicAuth{
				AuthType: BasicType,
				Username: fmt.Sprintf("${%s_USERNAME}", strings.ToUpper(c.providerName)),
				Password: fmt.Sprintf("${%s_PASSWORD}", strings.ToUpper(c.providerName)),
			}
		case "bearer":
			return &ApiKeyAuth{
				AuthType: APIKeyType,
				APIKey:   fmt.Sprintf("Bearer ${%s_API_KEY}", strings.ToUpper(c.providerName)),
				VarName:  "Authorization",
				Location: "header",
			}
		}
	case "oauth2":
		// OpenAPI 3.x
		if flows, ok := scheme["flows"].(map[string]interface{}); ok {
			for _, rawFlow := range flows {
				if flow, ok2 := rawFlow.(map[string]interface{}); ok2 {
					if tokenURL, _ := flow["tokenUrl"].(string); tokenURL != "" {
						var scope string
						if scopes, ok3 := flow["scopes"].(map[string]interface{}); ok3 {
							var s []string
							for k := range scopes {
								s = append(s, k)
							}
							scope = strings.Join(s, " ")
						}
						return &OAuth2Auth{
							AuthType:     OAuth2Type,
							TokenURL:     tokenURL,
							ClientID:     fmt.Sprintf("${%s_CLIENT_ID}", strings.ToUpper(c.providerName)),
							ClientSecret: fmt.Sprintf("${%s_CLIENT_SECRET}", strings.ToUpper(c.providerName)),
							Scope:        optionalString(scope),
						}
					}
				}
			}
		}
		// OpenAPI 2.0 fallback
		if flowType, _ := scheme["flow"].(string); flowType != "" {
			if tokenURL, _ := scheme["tokenUrl"].(string); tokenURL != "" {
				var scope string
				if scopes, ok := scheme["scopes"].(map[string]interface{}); ok {
					var s []string
					for k := range scopes {
						s = append(s, k)
					}
					scope = strings.Join(s, " ")
				}
				return &OAuth2Auth{
					AuthType:     OAuth2Type,
					TokenURL:     tokenURL,
					ClientID:     fmt.Sprintf("${%s_CLIENT_ID}", strings.ToUpper(c.providerName)),
					ClientSecret: fmt.Sprintf("${%s_CLIENT_SECRET}", strings.ToUpper(c.providerName)),
					Scope:        optionalString(scope),
				}
			}
		}
	}

	return nil
}

func (c *OpenApiConverter) createTool(
	path, method string,
	op map[string]interface{},
	baseURL string,
) (*Tool, error) {
	opID, _ := op["operationId"].(string)
	if opID == "" {
		// synthesize operation ID from method+path
		sanitizedPath := sanitizeName(path)
		opID = fmt.Sprintf("%s_%s", strings.ToLower(method), sanitizedPath)
		// dedupe if we've seen it before
		if count, exists := c.nameCounts[opID]; exists {
			c.nameCounts[opID] = count + 1
			opID = fmt.Sprintf("%s_%d", opID, count+1)
		} else {
			c.nameCounts[opID] = 1
		}
	}

	desc, _ := op["summary"].(string)
	if desc == "" {
		desc, _ = op["description"].(string)
	}
	var tags []string
	if rawTags, ok := op["tags"].([]interface{}); ok {
		for _, t := range rawTags {
			if s, ok2 := t.(string); ok2 {
				tags = append(tags, s)
			}
		}
	}

	// inputs
	inputSchema, headers, bodyField := c.extractInputs(op)

	// outputs
	outputSchema := c.extractOutputs(op)

	// auth
	authObj := c.extractAuth(op)
	var prov Provider
	if authObj != nil {
		prov = &HTTPAuthProvider{
			Delegate: &HTTPProvider{
				URL: fmt.Sprintf("%s%s", strings.TrimRight(baseURL, "/"), path),
			},
			Auth: authObj,
		}
	} else {
		prov = &HTTPProvider{
			URL: fmt.Sprintf("%s%s", strings.TrimRight(baseURL, "/"), path),
		}
	}

	// incorporate header overrides if any
	if len(headers) > 0 {
		inputSchema.Properties["headers"] = map[string]interface{}{
			"type": "object",
		}
	}

	return &Tool{
		Name:        opID,
		Description: desc,
		Inputs:      inputSchema,
		Outputs:     outputSchema,
		Tags:        tags,
		Provider:    prov,
	}, nil
}

// extractInputs returns (schema, headerFields, bodyFieldName).
func (c *OpenApiConverter) extractInputs(
	op map[string]interface{},
) (ToolInputOutputSchema, []string, *string) {
	props := map[string]ToolInputOutputSchema{}
	required := []string{}
	headers := []string{}
	var bodyField *string

	if parameters, ok := op["parameters"].([]interface{}); ok {
		for _, rawParam := range parameters {
			if param, ok2 := rawParam.(map[string]interface{}); ok2 {
				in, _ := param["in"].(string)
				name, _ := param["name"].(string)
				schema := map[string]interface{}{}
				if rawSchema, ok3 := param["schema"].(map[string]interface{}); ok3 {
					schema = rawSchema
				}
				prop := ToolInputOutputSchema{
					Type: "object",
				}
				// naive: assign underlying schema directly
				if len(schema) > 0 {
					prop = ToolInputOutputSchema{
						Type:       "object",
						Properties: map[string]any{},
					}
				}
				props[name] = prop
				if in == "header" {
					headers = append(headers, name)
				}
				if req, _ := param["required"].(bool); req {
					required = append(required, name)
				}
			}
		}
	}

	// requestBody (OpenAPI 3)
	if rb, ok := op["requestBody"].(map[string]interface{}); ok {
		if content, ok2 := rb["content"].(map[string]interface{}); ok2 {
			for mediaType, raw := range content {
				if mtObj, ok3 := raw.(map[string]interface{}); ok3 {
					if schema, ok4 := mtObj["schema"].(map[string]interface{}); ok4 {
						field := "body"
						bodyField = &field
						props[field] = ToolInputOutputSchema{
							Type:       "object",
							Properties: map[string]ToolInputOutputSchema{},
						}
						if rbReq, _ := rb["required"].(bool); rbReq {
							required = append(required, field)
						}
						_ = mediaType // could use to differentiate
					}
				}
			}
		}
	}

	var reqPtr []string
	if len(required) > 0 {
		reqPtr = required
	}
	return ToolInputOutputSchema{
		Type:       "object",
		Properties: props,
		Required:   reqPtr,
	}, headers, bodyField
}

// extractOutputs is assumed similar to extractInputs and is defined elsewhere in the file or package.
// (Not fully shown here; retain your original implementation.)
