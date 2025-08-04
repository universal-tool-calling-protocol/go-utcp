package openapi

import (
	"reflect"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"
)

func buildTestConverter() *OpenApiConverter {
	spec := map[string]interface{}{
		"info": map[string]interface{}{"title": "Test"},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"Obj": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{"type": "string"},
					},
				},
			},
			"securitySchemes": map[string]interface{}{
				"apiKey": map[string]interface{}{
					"type": "apiKey",
					"name": "X-Token",
					"in":   "header",
				},
				"basicAuth": map[string]interface{}{
					"type":   "http",
					"scheme": "basic",
				},
			},
		},
		"security": []interface{}{map[string]interface{}{"apiKey": []interface{}{}}},
	}
	return NewConverter(spec, "https://api.example.com/spec.json", "test")
}

func TestResolveRefAndSchema(t *testing.T) {
	c := buildTestConverter()
	obj, err := c.resolveRef("#/components/schemas/Obj")
	if err != nil || obj["type"] != "object" {
		t.Fatalf("resolveRef failed: %v %v", obj, err)
	}
	if _, err := c.resolveRef("#/bad/ref"); err == nil {
		t.Fatalf("expected error for bad ref")
	}
	m := map[string]interface{}{"$ref": "#/components/schemas/Obj"}
	out := c.resolveSchema(m).(map[string]interface{})
	if _, ok := out["properties"].(map[string]interface{}); !ok {
		t.Fatalf("resolveSchema map failed: %v", out)
	}
	arr := []interface{}{map[string]interface{}{"$ref": "#/components/schemas/Obj"}}
	sl := c.resolveSchema(arr).([]interface{})
	if len(sl) != 1 || sl[0].(map[string]interface{})["type"] != "object" {
		t.Fatalf("resolveSchema slice failed: %v", sl)
	}
}

func TestCreateAuthFromSchemeAndExtract(t *testing.T) {
	c := buildTestConverter()
	api := c.createAuthFromScheme(map[string]interface{}{"type": "apiKey", "in": "header", "name": "X"}).(*ApiKeyAuth)
	if api.VarName != "X" || api.Location != "header" || api.APIKey != "${TEST_API_KEY}" {
		t.Fatalf("apiKey auth mismatch: %+v", api)
	}
	bas := c.createAuthFromScheme(map[string]interface{}{"type": "http", "scheme": "basic"}).(*BasicAuth)
	if bas.Type() != BasicType {
		t.Fatalf("basic auth wrong type")
	}
	bear := c.createAuthFromScheme(map[string]interface{}{"type": "http", "scheme": "bearer"}).(*ApiKeyAuth)
	if bear.APIKey == "" || bear.VarName != "Authorization" {
		t.Fatalf("bearer auth wrong: %+v", bear)
	}
	oauth := c.createAuthFromScheme(map[string]interface{}{
		"type": "oauth2",
		"flows": map[string]interface{}{
			"client": map[string]interface{}{
				"tokenUrl": "https://t",
				"scopes":   map[string]interface{}{"s": "d"},
			},
		},
	}).(*OAuth2Auth)
	if oauth.TokenURL != "https://t" || oauth.ClientID == "" || oauth.Scope == nil {
		t.Fatalf("oauth2 auth wrong: %+v", oauth)
	}
	oauth2 := c.createAuthFromScheme(map[string]interface{}{
		"type":     "oauth2",
		"flow":     "client",
		"tokenUrl": "https://t",
		"scopes":   map[string]interface{}{"r": "d"},
	}).(*OAuth2Auth)
	if oauth2.TokenURL != "https://t" || oauth2.ClientID == "" || oauth2.Scope == nil {
		t.Fatalf("oauth2 (oas2) wrong: %+v", oauth2)
	}

	auth := c.extractAuth(map[string]interface{}{})
	if _, ok := auth.(*ApiKeyAuth); !ok {
		t.Fatalf("extractAuth did not use global security")
	}
	auth = c.extractAuth(map[string]interface{}{
		"security": []interface{}{map[string]interface{}{"basicAuth": []interface{}{}}},
	})
	if _, ok := auth.(*BasicAuth); !ok {
		t.Fatalf("extractAuth op override failed")
	}
}

func TestGetSecuritySchemes(t *testing.T) {
	c := buildTestConverter()
	ss := c.getSecuritySchemes()
	if len(ss) != 2 || ss["apiKey"] == nil {
		t.Fatalf("unexpected schemes: %+v", ss)
	}
}

func TestInputsOutputsAndCreateTool(t *testing.T) {
	c := buildTestConverter()
	op := map[string]interface{}{
		"operationId": "ping",
		"summary":     "Ping",
		"tags":        []interface{}{"t"},
		"parameters": []interface{}{
			map[string]interface{}{
				"name":     "id",
				"in":       "query",
				"required": true,
				"schema":   map[string]interface{}{"type": "string"},
			},
			map[string]interface{}{
				"name":   "X",
				"in":     "header",
				"schema": map[string]interface{}{"type": "string"},
			},
		},
		"requestBody": map[string]interface{}{
			"required": true,
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{
					"schema": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{"foo": map[string]interface{}{"type": "string"}},
					},
				},
			},
		},
		"responses": map[string]interface{}{
			"200": map[string]interface{}{
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"type":        "object",
							"description": "desc",
							"properties":  map[string]interface{}{"ok": map[string]interface{}{"type": "boolean"}},
						},
					},
				},
			},
		},
	}

	schema, headers, body := c.extractInputs(op)
	if len(schema.Properties) != 3 || len(headers) != 1 || body == nil || *body != "body" {
		t.Fatalf("extractInputs bad: %+v %+v %v", schema, headers, body)
	}
	out := c.extractOutputs(op)
	if out.Type != "object" || out.Description != "desc" || out.Properties["ok"] == nil {
		t.Fatalf("extractOutputs bad: %+v", out)
	}

	tool, err := c.createTool("/ping", "get", op, "https://api.example.com")
	if err != nil || tool == nil || tool.Provider.(*HttpProvider).URL != "https://api.example.com/ping" {
		t.Fatalf("createTool failed: %v %+v", err, tool)
	}
}

func TestCastHelpers(t *testing.T) {
	if castString(123, "def") != "def" {
		t.Fatalf("castString")
	}
	if castMap("x") != nil {
		t.Fatalf("castMap")
	}
	if !reflect.DeepEqual(castStringSlice([]interface{}{"a", "b"}), []string{"a", "b"}) {
		t.Fatalf("castStringSlice")
	}
	if castInterfaceSlice("x") != nil {
		t.Fatalf("castInterfaceSlice")
	}
	if f := castFloat(5); f == nil || *f != 5 {
		t.Fatalf("castFloat int")
	}
	if f := castFloat(1.5); f == nil || *f != 1.5 {
		t.Fatalf("castFloat float")
	}
}

func TestConvert_Basic(t *testing.T) {
	spec := map[string]interface{}{
		"info":    map[string]interface{}{"title": "Test API"},
		"servers": []interface{}{map[string]interface{}{"url": "https://api.example.com"}},
		"paths": map[string]interface{}{
			"/ping": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "ping",
					"summary":     "Ping",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c := NewConverter(spec, "", "")
	manual := c.Convert()
	if manual.Version != "1.0" {
		t.Fatalf("unexpected version: %s", manual.Version)
	}
	if len(manual.Tools) != 1 || manual.Tools[0].Name != "ping" {
		t.Fatalf("unexpected tools: %+v", manual.Tools)
	}
}
