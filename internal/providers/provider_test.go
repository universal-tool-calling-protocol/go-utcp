package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseProviderType(t *testing.T) {
	bp := &BaseProvider{ProviderType: ProviderCLI}
	assert.Equal(t, ProviderCLI, bp.Type(), "BaseProvider.Type should return the ProviderType field")
}

func TestUnmarshalProvider_HTTP(t *testing.T) {
	jsonData := []byte(`{
		"name": "TestHTTP",
		"provider_type": "http",
		"http_method": "POST",
		"url": "https://example.com/api",
		"content_type": "application/json"
	}`)
	p, err := UnmarshalProvider(jsonData)
	assert.NoError(t, err)
	hp, ok := p.(*HttpProvider)
	assert.True(t, ok, "Expected HttpProvider type")
	assert.Equal(t, "TestHTTP", hp.Name)
	assert.Equal(t, "POST", hp.HTTPMethod)
	assert.Equal(t, "https://example.com/api", hp.URL)
	assert.Equal(t, "application/json", hp.ContentType)
}

func TestUnmarshalProvider_SSE(t *testing.T) {
	jsonData := []byte(`{
		"name": "TestSSE",
		"provider_type": "sse",
		"url": "https://example.com/events",
		"reconnect": false,
		"retry_timeout": 10000
	}`)
	p, err := UnmarshalProvider(jsonData)
	assert.NoError(t, err)
	sp, ok := p.(*SSEProvider)
	assert.True(t, ok, "Expected SSEProvider type")
	assert.Equal(t, "TestSSE", sp.Name)
	assert.Equal(t, "https://example.com/events", sp.URL)
	assert.False(t, sp.Reconnect)
	assert.Equal(t, 10000, sp.RetryTimeout)
}

func TestUnmarshalProvider_CLI(t *testing.T) {
	jsonData := []byte(`{
		"name": "TestCLI",
		"provider_type": "cli",
		"command_name": "echo",
		"env_vars": {"FOO": "bar"}
	}`)
	p, err := UnmarshalProvider(jsonData)
	assert.NoError(t, err)
	cp, ok := p.(*CliProvider)
	assert.True(t, ok, "Expected CliProvider type")
	assert.Equal(t, "TestCLI", cp.Name)
	assert.Equal(t, "echo", cp.CommandName)
	assert.Equal(t, map[string]string{"FOO": "bar"}, cp.EnvVars)
}

func TestUnmarshalProvider_TCPAndUDP(t *testing.T) {
	for _, tc := range []struct {
		jsonData []byte
		typeName string
	}{
		{[]byte(`{"name":"TCP","provider_type":"tcp","host":"localhost","port":8080,"timeout":20000}`), "TCPProvider"},
		{[]byte(`{"name":"UDP","provider_type":"udp","host":"127.0.0.1","port":9090,"timeout":15000}`), "UDPProvider"},
	} {
		p, err := UnmarshalProvider(tc.jsonData)
		assert.NoError(t, err)
		switch prov := p.(type) {
		case *TCPProvider:
			assert.Equal(t, "TCP", prov.Name)
			assert.Equal(t, 8080, prov.Port)
			assert.Equal(t, 20000, prov.Timeout)
		case *UDPProvider:
			assert.Equal(t, "UDP", prov.Name)
			assert.Equal(t, 9090, prov.Port)
			assert.Equal(t, 15000, prov.Timeout)
		default:
			t.Errorf("Expected %s, got %T", tc.typeName, p)
		}
	}
}

func TestUnmarshalProvider_Unsupported(t *testing.T) {
	jsonData := []byte(`{"provider_type":"unknown"}`)
	_, err := UnmarshalProvider(jsonData)
	assert.Error(t, err)
}
