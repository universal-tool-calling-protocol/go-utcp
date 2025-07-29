package utcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tag"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

type miniTransport struct{ used bool }

func (m *miniTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	m.used = true
	return []Tool{{Name: "x"}}, nil
}
func (m *miniTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error { return nil }
func (m *miniTransport) CallTool(ctx context.Context, tool string, args map[string]any, prov Provider, l *string) (any, error) {
	return nil, errors.ErrUnsupported
}

func (m *miniTransport) CallToolStream(ctx context.Context, toolName string, args map[string]any, p Provider) (transports.StreamResult, error) {
	return nil, errors.ErrUnsupported
}

func TestParseProvidersJSON(t *testing.T) {
	tests := []struct {
		data    string
		want    int
		wantErr bool
	}{
		{`[{"provider_type":"cli","command_name":"e"}]`, 1, false},
		{`{"providers":[{"provider_type":"cli","command_name":"e"}]}`, 1, false},
		{`{"providers":{"provider_type":"cli","command_name":"e"}}`, 1, false},
		{`{"provider_type":"cli","command_name":"e"}`, 1, false},
		{`42`, 0, true},
		{`{"providers":"bad"}`, 0, true},
	}
	for _, tt := range tests {
		res, err := parseProvidersJSON([]byte(tt.data))
		if tt.wantErr && err == nil {
			t.Fatalf("expected error for %s", tt.data)
		}
		if !tt.wantErr && (err != nil || len(res) != tt.want) {
			t.Fatalf("parseProvidersJSON failed for %s: %v %v", tt.data, res, err)
		}
	}
}

func TestProcessProviderDefaultName(t *testing.T) {
	repo := NewInMemoryToolRepository()
	mt := &miniTransport{}
	c := &UtcpClient{
		config:         NewClientConfig(),
		transports:     map[string]ClientTransport{"cli": mt},
		toolRepository: repo,
		searchStrategy: NewTagSearchStrategy(repo, 1.0),
	}
	raw := map[string]any{"provider_type": "cli", "command_name": "echo"}
	if err := c.processProvider(context.Background(), raw, 0); err != nil {
		t.Fatalf("processProvider error: %v", err)
	}
	if !mt.used {
		t.Fatalf("transport not used")
	}
	if p, _ := repo.GetProvider(context.Background(), "cli_0"); p == nil {
		t.Fatalf("provider not saved with default name")
	}
}

func TestProcessProviderError(t *testing.T) {
	c := &UtcpClient{config: NewClientConfig()}
	if err := c.processProvider(context.Background(), map[string]any{}, 0); err == nil {
		t.Fatalf("expected error on missing provider_type")
	}
	bad := map[string]any{"provider_type": "cli", "command_name": json.Number("1")}
	// Use no transports so registration fails inside UnmarshalProvider
	if err := c.processProvider(context.Background(), bad, 0); err == nil {
		t.Fatalf("expected error on decode")
	}
}
