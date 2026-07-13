package utcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/cli"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

type benchmarkTransport struct {
	tools []tools.Tool
}

func (t *benchmarkTransport) RegisterToolProvider(context.Context, base.Provider) ([]tools.Tool, error) {
	return t.tools, nil
}

func (*benchmarkTransport) DeregisterToolProvider(context.Context, base.Provider) error {
	return nil
}

func (*benchmarkTransport) CallTool(context.Context, string, map[string]any, base.Provider, *string) (any, error) {
	return nil, nil
}

func (*benchmarkTransport) CallToolStream(context.Context, string, map[string]any, base.Provider) (transports.StreamResult, error) {
	return transports.NewSliceStreamResult(nil, nil), nil
}

func BenchmarkCachedToolDispatch(b *testing.B) {
	const toolCount = 1_000
	registered := make([]tools.Tool, toolCount)
	for i := range registered {
		registered[i].Name = fmt.Sprintf("tool_%04d", i)
	}

	repo := repository.NewInMemoryToolRepository()
	transport := &benchmarkTransport{tools: registered}
	client := &UtcpClient{
		config:         NewClientConfig(),
		transports:     map[string]repository.ClientTransport{"cli": transport},
		toolRepository: repo,
	}
	provider := &cli.CliProvider{BaseProvider: base.BaseProvider{Name: "bench", ProviderType: base.ProviderCLI}}
	if _, err := client.RegisterToolProvider(context.Background(), provider); err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	args := map[string]any{"value": 42}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := client.CallTool(ctx, "bench.tool_0999", args); err != nil {
			b.Fatal(err)
		}
	}
}
