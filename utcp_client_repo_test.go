package utcp

import (
	"context"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/internal"
	. "github.com/universal-tool-calling-protocol/go-utcp/internal/providers"
)

func TestInMemoryToolRepository_CRUD(t *testing.T) {
	repo := NewInMemoryToolRepository().(*InMemoryToolRepository)
	ctx := context.Background()
	prov := &CliProvider{BaseProvider: BaseProvider{Name: "cli", ProviderType: ProviderCLI}}
	tools := []Tool{{Name: "cli.echo"}}

	if err := repo.SaveProviderWithTools(ctx, prov, tools); err != nil {
		t.Fatalf("save error: %v", err)
	}

	p, err := repo.GetProvider(ctx, "cli")
	if err != nil || p == nil {
		t.Fatalf("get provider error: %v", err)
	}

	if _, err := repo.GetProviders(ctx); err != nil {
		t.Fatalf("get providers error: %v", err)
	}

	if _, err := repo.GetTool(ctx, "cli.echo"); err != nil {
		t.Fatalf("get tool error: %v", err)
	}

	if _, err := repo.GetTools(ctx); err != nil {
		t.Fatalf("get tools error: %v", err)
	}

	if _, err := repo.GetToolsByProvider(ctx, "cli"); err != nil {
		t.Fatalf("get tools by provider error: %v", err)
	}

	if err := repo.RemoveTool(ctx, "cli.echo"); err != nil {
		t.Fatalf("remove tool error: %v", err)
	}

	if err := repo.RemoveProvider(ctx, "cli"); err != nil {
		t.Fatalf("remove provider error: %v", err)
	}
}

func TestInMemoryToolRepository_Errors(t *testing.T) {
	repo := NewInMemoryToolRepository().(*InMemoryToolRepository)
	ctx := context.Background()
	if err := repo.RemoveProvider(ctx, "missing"); err == nil {
		t.Errorf("expected error removing missing provider")
	}
	if err := repo.RemoveTool(ctx, "none"); err == nil {
		t.Errorf("expected error removing missing tool")
	}
}
