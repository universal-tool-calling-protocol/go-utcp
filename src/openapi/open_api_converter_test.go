package openapi

import (
	"testing"
)

func TestOptionalString(t *testing.T) {
	if optionalString("") != nil {
		t.Fatalf("expected nil for empty string")
	}
	val := optionalString("x")
	if val == nil || *val != "x" {
		t.Fatalf("unexpected value: %v", val)
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
