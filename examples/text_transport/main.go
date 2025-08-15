package main

import (
	"context"
	"fmt"

	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/text"
	transport "github.com/universal-tool-calling-protocol/go-utcp/src/transports/text"
)

func main() {
	ctx := context.Background()
	t := transport.NewTextTransport(func(format string, args ...interface{}) {
		fmt.Printf("[LOG] "+format+"\n", args...)
	})
	p := &providers.TextProvider{Templates: map[string]string{"hello": "Hello, {{.name}}"}}
	tools, err := t.RegisterToolProvider(ctx, p)
	if err != nil {
		panic(err)
	}
	res, err := t.CallTool(ctx, tools[0].Name, map[string]any{"name": "UTCP"}, p, nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Result: %v\n", res)
}
