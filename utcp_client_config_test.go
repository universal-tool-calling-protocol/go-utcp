package utcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUtcpVariableNotFound_Error(t *testing.T) {
	err := (&UtcpVariableNotFound{VariableName: "FOO"}).Error()
	if !strings.Contains(err, "FOO") {
		t.Errorf("error message should contain variable name; got %s", err)
	}
}

func TestUtcpDotEnv_LoadAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	fpath := filepath.Join(tmpDir, ".env")
	os.WriteFile(fpath, []byte("FOO=bar\n"), 0o644)

	d := NewDotEnv(fpath)
	vars, err := d.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if vars["FOO"] != "bar" {
		t.Errorf("expected FOO=bar, got %v", vars["FOO"])
	}

	val, err := d.Get("FOO")
	if err != nil || val != "bar" {
		t.Errorf("Get returned %s,%v", val, err)
	}
	_, err = d.Get("MISSING")
	if err == nil {
		t.Errorf("expected error for missing variable")
	}
}

func TestNewClientConfig_Defaults(t *testing.T) {
	cfg := NewClientConfig()
	if cfg.ProvidersFilePath != "" || len(cfg.Variables) != 0 {
		t.Errorf("unexpected defaults %+v", cfg)
	}
}
