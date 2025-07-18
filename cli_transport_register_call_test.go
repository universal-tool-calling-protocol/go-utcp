package utcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCliTransport_RegisterAndCall(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "prov.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nif [ \"$1\" = call ]; then\n echo '{\"ok\":true}'\nelse\n echo '{\"tools\":[{\"name\":\"echo\",\"description\":\"Echo\"}]}'\nfi\n"), 0o755)

	prov := &CliProvider{
		BaseProvider: BaseProvider{Name: "cli", ProviderType: ProviderCLI},
		CommandName:  script,
	}
	tr := NewCliTransport(nil)
	ctx := context.Background()

	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil || len(tools) != 1 {
		t.Fatalf("register error %v tools %v", err, tools)
	}

	res, err := tr.CallTool(ctx, "echo", map[string]interface{}{}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	m, ok := res.(map[string]interface{})
	if !ok || m["ok"] != true {
		t.Fatalf("unexpected result %v", res)
	}

	if err := tr.DeregisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("deregister error: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
}
