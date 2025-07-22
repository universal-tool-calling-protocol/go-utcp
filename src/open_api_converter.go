//go:build ignore

package src

import (
	"fmt"
	"net/url"
	"strings"
)

// OpenApiConverter converts an OpenAPI JSON spec into a UtcpManual.
type OpenApiConverter struct {
	spec         map[string]interface{}
	specURL      string
	providerName string
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
	}
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
		Version: Version,
		Tools:   tools,
	}
}

// resolveRef follows a local JSON Pointer (only "#/...").
func (c *OpenApiConverter) resolveRef(ref string) (map[string]interface{}, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("only local refs supported, got %q", ref)
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
		if ss, ok2 := comp["securitySchemes"].(map[string]interface{}); ok2 {
			return ss
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
		// OpenAPI 2.0
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

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// createTool builds a tool.Tool from a single OpenAPI operation.
func (c *OpenApiConverter) createTool(
	path, method string,
	op map[string]interface{},
	baseURL string,
) (*Tool, error) {
	opID, _ := op["operationId"].(string)
	if opID == "" {
		return nil, nil // skip unnamed ops
	}

	desc, _ := op["summary"].(string)
	if desc == "" {
		desc, _ = op["description"].(string)
	}
	var tags []string
	if rawTags, ok := op["tags"].([]interface{}); ok {
		for _, t := range rawTags {
			if ts, ok2 := t.(string); ok2 {
				tags = append(tags, ts)
			}
		}
	}

	inputSchema, headers, bodyField := c.extractInputs(op)
	outputSchema := c.extractOutputs(op)
	authObj := c.extractAuth(op)

	fullURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")

	prov := &HttpProvider{
		BaseProvider: BaseProvider{
			Name:         c.providerName,
			ProviderType: ProviderHTTP,
		},
		HTTPMethod:   strings.ToUpper(method),
		URL:          fullURL,
		ContentType:  "application/json",
		Auth:         &authObj,
		Headers:      nil,
		BodyField:    bodyField,
		HeaderFields: headers,
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
	props := map[string]interface{}{}
	var required []string
	var headers []string
	var bodyField *string

	if rawParams, ok := op["parameters"].([]interface{}); ok {
		for _, rp := range rawParams {
			param := c.resolveSchema(rp).(map[string]interface{})
			name, _ := param["name"].(string)
			loc, _ := param["in"].(string)
			if name == "" {
				continue
			}
			if loc == "header" {
				headers = append(headers, name)
			}
			sch := c.resolveSchema(param["schema"]).(map[string]interface{})
			entry := map[string]interface{}{
				"type":        sch["type"],
				"description": param["description"],
			}
			for k, v := range sch {
				entry[k] = v
			}
			props[name] = entry
			if req, _ := param["required"].(bool); req {
				required = append(required, name)
			}
		}
	}

	if rb, ok := op["requestBody"].(map[string]interface{}); ok {
		rb = c.resolveSchema(rb).(map[string]interface{})
		if content, ok2 := rb["content"].(map[string]interface{}); ok2 {
			if appJSON, ok3 := content["application/json"].(map[string]interface{}); ok3 {
				if schema, ok4 := appJSON["schema"].(map[string]interface{}); ok4 {
					name := "body"
					bodyField = &name
					sch := c.resolveSchema(schema).(map[string]interface{})
					entry := map[string]interface{}{"description": rb["description"]}
					for k, v := range sch {
						entry[k] = v
					}
					props[name] = entry
					if req, _ := rb["required"].(bool); req {
						required = append(required, name)
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

// extractOutputs builds the response schema.
func (c *OpenApiConverter) extractOutputs(
	op map[string]interface{},
) ToolInputOutputSchema {
	resp := map[string]interface{}{}
	if r200, ok := op["responses"].(map[string]interface{})["200"].(map[string]interface{}); ok {
		resp = r200
	} else if r201, ok := op["responses"].(map[string]interface{})["201"].(map[string]interface{}); ok {
		resp = r201
	} else {
		return ToolInputOutputSchema{}
	}

	resp = c.resolveSchema(resp).(map[string]interface{})
	if content, ok := resp["content"].(map[string]interface{}); ok {
		if appJSON, ok2 := content["application/json"].(map[string]interface{}); ok2 {
			if schema, ok3 := appJSON["schema"].(map[string]interface{}); ok3 {
				sch := c.resolveSchema(schema).(map[string]interface{})
				out := ToolInputOutputSchema{
					Type:        castString(sch["type"], "object"),
					Properties:  castMap(sch["properties"]),
					Required:    castStringSlice(sch["required"]),
					Description: castString(sch["description"], ""),
					Title:       castString(sch["title"], ""),
				}
				if out.Type == "array" {
					out.Items = castMap(sch["items"])
				}
				for _, attr := range []string{"enum", "minimum", "maximum", "format"} {
					if v, ok := sch[attr]; ok {
						switch attr {
						case "enum":
							out.Enum = castInterfaceSlice(v)
						case "minimum":
							out.Minimum = castFloat(v)
						case "maximum":
							out.Maximum = castFloat(v)
						case "format":
							out.Format = castString(v, "")
						}
					}
				}
				return out
			}
		}
	}
	return ToolInputOutputSchema{}
}

// ---- small casting src ----

func castString(v interface{}, def string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func castMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func castStringSlice(v interface{}) []string {
	if arr, ok := v.([]interface{}); ok {
		var out []string
		for _, e := range arr {
			if s, ok2 := e.(string); ok2 {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func castInterfaceSlice(v interface{}) []interface{} {
	if arr, ok := v.([]interface{}); ok {
		return arr
	}
	return nil
}

func castFloat(v interface{}) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case int:
		f := float64(n)
		return &f
	}
	return nil
}
