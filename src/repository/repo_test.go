package repository

import (
	"context"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/cli"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

func TestInMemoryToolRepository_CRUD(t *testing.T) {
	repo := NewInMemoryToolRepository().(*InMemoryToolRepository)
	ctx := context.Background()
	prov := &cli.CliProvider{BaseProvider: BaseProvider{Name: "cli", ProviderType: ProviderCLI}}
	tools := []Tool{{Name: "echo"}}

	if err := repo.SaveProviderWithTools(ctx, prov, tools); err != nil {
		t.Fatalf("save error: %v", err)
	}

	if p, err := repo.GetProvider(ctx, "cli"); err != nil || p == nil {
		t.Fatalf("get provider failed: %v", err)
	}
	if ts, err := repo.GetTools(ctx); err != nil || len(ts) != 1 {
		t.Fatalf("get tools failed: %v", err)
	}
	if _, err := repo.GetToolsByProvider(ctx, "cli"); err != nil {
		t.Fatalf("get tools by provider failed: %v", err)
	}
	if _, err := repo.GetTool(ctx, "echo"); err != nil {
		t.Fatalf("get tool failed: %v", err)
	}

	if err := repo.RemoveTool(ctx, "echo"); err != nil {
		t.Fatalf("remove tool failed: %v", err)
	}
	if err := repo.RemoveProvider(ctx, "cli"); err != nil {
		t.Fatalf("remove provider failed: %v", err)
	}
}

type bogusProvider struct{ BaseProvider }

func TestInMemoryToolRepository_Unsupported(t *testing.T) {
	repo := NewInMemoryToolRepository().(*InMemoryToolRepository)
	ctx := context.Background()
	bp := &bogusProvider{BaseProvider{Name: "bogus"}}
	if err := repo.SaveProviderWithTools(ctx, bp, nil); err == nil {
		t.Fatalf("expected error for unsupported provider")
	}
}
