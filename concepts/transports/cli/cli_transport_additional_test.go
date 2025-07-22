package cli

import (
	"context"
	"fmt"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/concepts/providers"
	. "github.com/universal-tool-calling-protocol/go-utcp/concepts/transports/graphql"
	. "github.com/universal-tool-calling-protocol/go-utcp/concepts/transports/http"
)

func TestCliTransportLogging(t *testing.T) {
	msgs := []string{}
	tr := NewCliTransport(func(f string, args ...interface{}) { msgs = append(msgs, fmt.Sprintf(f, args...)) })
	tr.logInfo("hello")
	tr.logError("bad")
	if len(msgs) != 2 || msgs[0] == msgs[1] {
		t.Fatalf("unexpected log messages: %v", msgs)
	}
}

func TestHttpAndGraphQLDeregister(t *testing.T) {
	h := NewHttpClientTransport(nil)
	if err := h.DeregisterToolProvider(context.Background(), &HttpProvider{}); err != nil {
		t.Fatalf("http deregister err: %v", err)
	}
	g := NewGraphQLClientTransport(nil)
	if err := g.DeregisterToolProvider(context.Background(), &GraphQLProvider{}); err != nil {
		t.Fatalf("gql deregister err: %v", err)
	}
}

func TestExtractManualAdditional(t *testing.T) {
	tr := NewCliTransport(nil)
	mixed := `line
{"tools":[{"name":"a","description":"d"}]}
other`
	tools := tr.extractManual(mixed, "p")
	if len(tools) != 1 || tools[0].Name != "a" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
}

func TestExecuteCommandFailures(t *testing.T) {
	tr := NewCliTransport(nil)
	ctx := context.Background()

	_, _, code, err := tr.executeCommand(ctx, "sh", []string{"-c", "echo hi; exit 1"}, nil, "", "")
	if err != nil || code != 1 {
		t.Fatalf("expected exit code 1, got %d err %v", code, err)
	}

	_, _, _, err = tr.executeCommand(ctx, "nonexistent_command_xyz", nil, nil, "", "")
	if err == nil {
		t.Fatalf("expected error for missing command")
	}
}
