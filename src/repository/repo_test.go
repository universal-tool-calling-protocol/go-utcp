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
	if tool, err := repo.GetTool(ctx, "echo"); err != nil || tool != nil {
		t.Fatalf("removed tool remained indexed: tool=%v err=%v", tool, err)
	}
	if err := repo.RemoveProvider(ctx, "cli"); err != nil {
		t.Fatalf("remove provider failed: %v", err)
	}
}

func TestInMemoryToolRepository_ReplacesProviderIndex(t *testing.T) {
	repo := NewInMemoryToolRepository().(*InMemoryToolRepository)
	ctx := context.Background()
	provider := &cli.CliProvider{BaseProvider: BaseProvider{Name: "cli", ProviderType: ProviderCLI}}
	if err := repo.SaveProviderWithTools(ctx, provider, []Tool{{Name: "cli.old"}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveProviderWithTools(ctx, provider, []Tool{{Name: "cli.new"}}); err != nil {
		t.Fatal(err)
	}
	if old, _ := repo.GetTool(ctx, "cli.old"); old != nil {
		t.Fatalf("replaced tool remained indexed: %+v", old)
	}
	if current, _ := repo.GetTool(ctx, "cli.new"); current == nil {
		t.Fatal("replacement tool was not indexed")
	}
}

func TestInMemoryToolRepository_IndexesLegacyInitialMapWrites(t *testing.T) {
	repo := NewInMemoryToolRepository().(*InMemoryToolRepository)
	provider := &cli.CliProvider{BaseProvider: BaseProvider{Name: "cli", ProviderType: ProviderCLI}}
	repo.Providers["cli"] = provider
	repo.Tools["cli"] = []Tool{{Name: "cli.old"}}

	if tool, _ := repo.GetTool(context.Background(), "cli.old"); tool == nil {
		t.Fatal("directly populated tool was not indexed")
	}
	if err := repo.RemoveProvider(context.Background(), "cli"); err != nil {
		t.Fatalf("remove after direct map writes failed: %v", err)
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
