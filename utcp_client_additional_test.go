package utcp

import (
	"context"
	"os"
	"testing"
)

// stubLoader implements UtcpVariablesConfig for tests.
type stubLoader struct{ vars map[string]string }

func (s stubLoader) Load() (map[string]string, error) { return s.vars, nil }
func (s stubLoader) Get(key string) (string, error) {
	if v, ok := s.vars[key]; ok {
		return v, nil
	}
	return "", &UtcpVariableNotFound{VariableName: key}
}

// stubTransport implements ClientTransport for testing UtcpClient.
type stubTransport struct {
	registerCalled   bool
	deregisterCalled bool
	callCalled       bool
}

func (s *stubTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	s.registerCalled = true
	return []Tool{{Name: "echo"}}, nil
}

func (s *stubTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	s.deregisterCalled = true
	return nil
}

func (s *stubTransport) CallTool(ctx context.Context, toolName string, args map[string]any, prov Provider, l *string) (any, error) {
	s.callCalled = true
	return "ok", nil
}

func TestGetVariableSources(t *testing.T) {
	c := &UtcpClient{}
	loader := stubLoader{vars: map[string]string{"BAR": "loader"}}
	cfg := &UtcpClientConfig{
		Variables:         map[string]string{"FOO": "inline"},
		LoadVariablesFrom: []UtcpVariablesConfig{loader},
	}

	if v, err := c.getVariable("FOO", cfg); err != nil || v != "inline" {
		t.Fatalf("inline variable failed: %v %v", v, err)
	}
	if v, err := c.getVariable("BAR", cfg); err != nil || v != "loader" {
		t.Fatalf("loader variable failed: %v %v", v, err)
	}
	os.Setenv("BAZ", "env")
	defer os.Unsetenv("BAZ")
	if v, err := c.getVariable("BAZ", cfg); err != nil || v != "env" {
		t.Fatalf("env variable failed: %v %v", v, err)
	}
	if _, err := c.getVariable("MISSING", cfg); err == nil {
		t.Fatalf("expected error for missing variable")
	}
}

func TestReplaceVarsInAny(t *testing.T) {
	cfg := &UtcpClientConfig{Variables: map[string]string{"X": "1", "Y": "2"}}
	os.Setenv("Z", "3")
	defer os.Unsetenv("Z")
	c := &UtcpClient{}
	input := map[string]any{
		"a": "${X}",
		"b": []any{"$Y", map[string]any{"c": "${Z}"}},
	}
	out := c.replaceVarsInAny(input, cfg).(map[string]any)
	if out["a"] != "1" {
		t.Fatalf("a not replaced: %v", out["a"])
	}
	b := out["b"].([]any)
	if b[0] != "2" || b[1].(map[string]any)["c"] != "3" {
		t.Fatalf("nested replacement failed: %+v", b)
	}
}

func TestProviderToMapAndCreateProviderOfType(t *testing.T) {
	c := &UtcpClient{}
	p := &CliProvider{BaseProvider: BaseProvider{Name: "cli", ProviderType: ProviderCLI}, CommandName: "cmd"}
	m := c.providerToMap(p)
	if m["command_name"] != "cmd" {
		t.Fatalf("providerToMap failed: %+v", m)
	}
	if _, ok := c.createProviderOfType(ProviderCLI).(*CliProvider); !ok {
		t.Fatalf("createProviderOfType wrong type")
	}
}

func TestSubstituteProviderVariables(t *testing.T) {
	cfg := &UtcpClientConfig{Variables: map[string]string{"HOST": "example.com", "X": "hdr"}}
	c := &UtcpClient{config: cfg}
	prov := &HttpProvider{
		BaseProvider: BaseProvider{Name: "p", ProviderType: ProviderHTTP},
		URL:          "http://${HOST}/",
		Headers:      map[string]string{"X": "${X}"},
	}
	out := c.substituteProviderVariables(prov).(*HttpProvider)
	if out.URL != "http://example.com/" || out.Headers["X"] != "hdr" {
		t.Fatalf("substitution failed: %+v", out)
	}
}

func TestGetAndSetProviderName(t *testing.T) {
	c := &UtcpClient{}
	providers := []Provider{
		&HttpProvider{BaseProvider: BaseProvider{Name: "h", ProviderType: ProviderHTTP}},
		&CliProvider{BaseProvider: BaseProvider{Name: "c", ProviderType: ProviderCLI}},
		&SSEProvider{BaseProvider: BaseProvider{Name: "s", ProviderType: ProviderSSE}},
		&StreamableHttpProvider{BaseProvider: BaseProvider{Name: "sh", ProviderType: ProviderHTTPStream}},
		&WebSocketProvider{BaseProvider: BaseProvider{Name: "ws", ProviderType: ProviderWebSocket}},
		&GRPCProvider{BaseProvider: BaseProvider{Name: "g", ProviderType: ProviderGRPC}},
		&GraphQLProvider{BaseProvider: BaseProvider{Name: "gql", ProviderType: ProviderGraphQL}},
		&TCPProvider{BaseProvider: BaseProvider{Name: "tcp", ProviderType: ProviderTCP}},
		&UDPProvider{BaseProvider: BaseProvider{Name: "udp", ProviderType: ProviderUDP}},
		&WebRTCProvider{BaseProvider: BaseProvider{Name: "rtc", ProviderType: ProviderWebRTC}},
		NewMCPProvider("m"),
		&TextProvider{BaseProvider: BaseProvider{Name: "txt", ProviderType: ProviderText}},
	}
	for _, p := range providers {
		c.setProviderName(p, "new")
		if name := c.getProviderName(p); name != "new" {
			t.Fatalf("name mismatch for %T: %s", p, name)
		}
	}
}

func TestUtcpClientFlow(t *testing.T) {
	repo := NewInMemoryToolRepository()
	tr := &stubTransport{}
	client := &UtcpClient{
		config:         NewClientConfig(),
		transports:     map[string]ClientTransport{"cli": tr},
		toolRepository: repo,
		searchStrategy: NewTagSearchStrategy(repo, 1.0),
	}
	ctx := context.Background()
	prov := &CliProvider{BaseProvider: BaseProvider{Name: "my.cli", ProviderType: ProviderCLI}, CommandName: "echo"}
	tools, err := client.RegisterToolProvider(ctx, prov)
	if err != nil || len(tools) != 1 || tools[0].Name != "my_cli.echo" || !tr.registerCalled {
		t.Fatalf("register failed: %v %v", tools, err)
	}
	if _, err := client.CallTool(ctx, "my_cli.echo", map[string]any{"a": 1}); err != nil || !tr.callCalled {
		t.Fatalf("call failed: %v", err)
	}
	res, err := client.SearchTools("my_cli", 10)
	if err != nil || len(res) == 0 {
		t.Fatalf("search failed: %v %v", res, err)
	}
	if err := client.DeregisterToolProvider(ctx, "my_cli"); err != nil || !tr.deregisterCalled {
		t.Fatalf("deregister failed: %v", err)
	}
}

func TestCreateProviderOfTypeAll(t *testing.T) {
	c := &UtcpClient{}
	types := []ProviderType{
		ProviderHTTP, ProviderCLI, ProviderSSE, ProviderHTTPStream,
		ProviderWebSocket, ProviderGRPC, ProviderGraphQL, ProviderTCP,
		ProviderUDP, ProviderWebRTC, ProviderMCP, ProviderText,
	}
	for _, pt := range types {
		p := c.createProviderOfType(pt)
		switch pt {
		case ProviderHTTP:
			if _, ok := p.(*HttpProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderCLI:
			if _, ok := p.(*CliProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderSSE:
			if _, ok := p.(*SSEProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderHTTPStream:
			if _, ok := p.(*StreamableHttpProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderWebSocket:
			if _, ok := p.(*WebSocketProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderGRPC:
			if _, ok := p.(*GRPCProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderGraphQL:
			if _, ok := p.(*GraphQLProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderTCP:
			if _, ok := p.(*TCPProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderUDP:
			if _, ok := p.(*UDPProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderWebRTC:
			if _, ok := p.(*WebRTCProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderMCP:
			if _, ok := p.(*MCPProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		case ProviderText:
			if _, ok := p.(*TextProvider); !ok {
				t.Fatalf("type %s", pt)
			}
		}
	}
}

func TestDefaultTransportsKeys(t *testing.T) {
	tr := defaultTransports()
	keys := []string{"http", "cli", "sse", "http_stream", "mcp", "udp", "tcp", "websocket", "text", "graphql", "grpc"}
	for _, k := range keys {
		if _, ok := tr[k]; !ok {
			t.Fatalf("missing transport %s", k)
		}
	}
}

func TestLoadProviders(t *testing.T) {
	repo := NewInMemoryToolRepository()
	st := &stubTransport{}
	client := &UtcpClient{
		config:         NewClientConfig(),
		transports:     map[string]ClientTransport{"cli": st},
		toolRepository: repo,
		searchStrategy: NewTagSearchStrategy(repo, 1.0),
	}
	data := `[{"provider_type":"cli","name":"lp","command_name":"echo"}]`
	f, err := os.CreateTemp("", "prov.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := client.loadProviders(context.Background(), f.Name()); err != nil {
		t.Fatalf("loadProviders err: %v", err)
	}
	if !st.registerCalled {
		t.Fatalf("transport not used")
	}
	if _, err := repo.GetProvider(context.Background(), "lp"); err != nil {
		t.Fatalf("provider not saved: %v", err)
	}
}

func TestNewUTCPClientBasic(t *testing.T) {
	c, err := NewUTCPClient(context.Background(), nil, nil, nil)
	if err != nil || c == nil {
		t.Fatalf("creation failed: %v", err)
	}
	if len(c.transports) == 0 {
		t.Fatalf("expected transports")
	}
}
