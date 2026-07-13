package streamable

import (
	"context"
	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/streamable"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

func TestStreamableHTTPClientTransport_RegisterAndCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/tools":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"version":"1.0","tools":[{"name":"echo","description":"Echo"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/echo":
			var in map[string]interface{}
			json.NewDecoder(r.Body).Decode(&in)
			out, _ := json.Marshal(map[string]interface{}{"result": in["msg"]})
			w.Header().Set("Content-Type", "application/json")
			w.Write(out)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov := &StreamableHttpProvider{
		BaseProvider: BaseProvider{Name: "stream", ProviderType: ProviderHTTPStream},
		URL:          server.URL + "/tools",
		HTTPMethod:   http.MethodGet,
	}

	tr := NewStreamableHTTPTransport(nil)
	ctx := context.Background()

	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	prov.URL = server.URL
	res, err := tr.CallTool(ctx, "echo", map[string]interface{}{"msg": "hi"}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}

	// Handle the StreamResult returned by CallTool
	streamResult, ok := res.(transports.StreamResult)
	if !ok {
		t.Fatalf("expected StreamResult, got: %T", res)
	}
	defer streamResult.Close()

	// Get the first (and only) result from the stream
	result, err := streamResult.Next()
	if err != nil {
		t.Fatalf("stream next error: %v", err)
	}

	// Verify there are no more results
	_, err = streamResult.Next()
	if err != io.EOF {
		t.Fatalf("expected EOF after first result, got: %v", err)
	}

	// Check the actual result
	m, ok := result.(map[string]interface{})
	if !ok || m["result"] != "hi" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type closeCountingBody struct {
	io.Reader
	closed chan struct{}
}

func (b *closeCountingBody) Close() error {
	b.closed <- struct{}{}
	return nil
}

func TestStreamableCloseUnblocksFullBuffer(t *testing.T) {
	closed := make(chan struct{}, 2)
	body := &closeCountingBody{
		Reader: strings.NewReader(strings.Repeat("{\"n\":1}\n", 32)),
		closed: closed,
	}
	transport := NewStreamableHTTPTransport(nil)
	transport.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       body,
			Request:    request,
		}, nil
	})}
	provider := &StreamableHttpProvider{
		BaseProvider: BaseProvider{Name: "stream", ProviderType: ProviderHTTPStream},
		URL:          "http://example.test",
	}
	result, err := transport.CallTool(context.Background(), "many", nil, provider, nil)
	if err != nil {
		t.Fatal(err)
	}
	stream := result.(transports.StreamResult)
	time.Sleep(20 * time.Millisecond)
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("stream producer did not exit after Close")
		}
	}
}

func TestStreamableDeregister(t *testing.T) {
	stream := NewStreamableHTTPTransport(nil)
	sth := &StreamableHttpProvider{BaseProvider: BaseProvider{Name: "h", ProviderType: ProviderHTTPStream}}
	if err := stream.DeregisterToolProvider(context.Background(), sth); err != nil {
		t.Fatalf("stream deregister error: %v", err)
	}
}
