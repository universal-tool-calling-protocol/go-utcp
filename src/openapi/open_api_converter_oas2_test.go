package openapi

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"
)

func TestConvert_OAS2(t *testing.T) {
	spec := map[string]interface{}{
		"swagger":  "2.0",
		"info":     map[string]interface{}{"title": "Test API"},
		"schemes":  []interface{}{"https"},
		"host":     "api.example.com",
		"basePath": "/v1",
		"paths": map[string]interface{}{
			"/ping": map[string]interface{}{
				"post": map[string]interface{}{
					"operationId": "ping",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":     "payload",
							"in":       "body",
							"required": true,
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"id": map[string]interface{}{"type": "string"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "ok",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"success": map[string]interface{}{"type": "boolean"},
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
	if len(manual.Tools) != 1 {
		t.Fatalf("expected one tool, got %d", len(manual.Tools))
	}
	tool := manual.Tools[0]
	hp := tool.Provider.(*HttpProvider)
	if hp.URL != "https://api.example.com/v1/ping" {
		t.Fatalf("unexpected URL: %s", hp.URL)
	}
	if hp.BodyField == nil || *hp.BodyField != "payload" {
		t.Fatalf("expected body field 'payload', got %v", hp.BodyField)
	}
	if tool.Outputs.Properties["success"] == nil {
		t.Fatalf("expected output property 'success'")
	}
}
