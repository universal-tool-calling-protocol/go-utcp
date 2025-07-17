package main

import (
	"context"
	"fmt"

	"github.com/Raezil/UTCP"
)

func main() {
	ctx := context.Background()

	// Example: Using Text transport for local testing (tools.json defines a "hello" tool)
	textTransport := UTCP.NewTextTransport("local-")
	textProvider := &UTCP.TextProvider{FilePath: "./tools.json"}
	localTools, err := textTransport.RegisterToolProvider(ctx, textProvider)
	if err != nil {
		panic(fmt.Errorf("failed to register local tools: %w", err))
	}
	fmt.Println("Local tools discovered:")
	for _, t := range localTools {
		fmt.Printf(" - %s: %s\n", t.Name, t.Description)
	}

	// Manually invoke the "hello" tool logic (JSON-defined Handler is not set in TextTransport by default)
	input := map[string]interface{}{"name": "World"}
	for _, tool := range localTools {
		if tool.Name == "hello" {
			nameVal, ok := input["name"].(string)
			if !ok {
				panic("invalid input for name")
			}
			// Generate greeting directly
			greeting := fmt.Sprintf("Hello, %s!", nameVal)
			result := map[string]interface{}{"greeting": greeting}
			fmt.Printf("Tool result: %v\n", result)
		}
	}
}
