package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	graphqltransport "github.com/universal-tool-calling-protocol/go-utcp/src/transports/graphql"
)

var gSchema graphql.Schema

func startServer(addr string) {
	// Define Query type
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"echo": &graphql.Field{
				Type: graphql.String,
				Args: graphql.FieldConfigArgument{
					"msg": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					msg, _ := p.Args["msg"].(string)
					return msg, nil
				},
			},
		},
	})

	// Define Subscription type
	subscriptionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Subscription",
		Fields: graphql.Fields{
			"updates": &graphql.Field{
				Type: graphql.Int,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return p.Source, nil
				},
				Subscribe: func(p graphql.ResolveParams) (interface{}, error) {
					ch := make(chan interface{})
					go func() {
						defer close(ch)
						for i := 1; i <= 2; i++ {
							select {
							case <-p.Context.Done():
								return
							case ch <- i:
							}
							time.Sleep(100 * time.Millisecond)
						}
					}()
					return ch, nil
				},
			},
		},
	})

	// Build the schema
	var err error
	gSchema, err = graphql.NewSchema(graphql.SchemaConfig{
		Query:        queryType,
		Subscription: subscriptionType,
	})
	if err != nil {
		log.Fatalf("failed to create schema: %v", err)
	}

	// Setup HTTP handler with body rewrite
	gqlHTTP := handler.New(&handler.Config{
		Schema:     &gSchema,
		Pretty:     true,
		GraphiQL:   false,
		Playground: false,
	})
	http.Handle("/graphql", withBodyRewrite(gqlHTTP, func(body []byte) []byte {
		var req struct {
			Query     string          `json:"query"`
			Variables json.RawMessage `json:"variables,omitempty"`
			OpName    string          `json:"operationName,omitempty"`
		}
		if err := json.Unmarshal(body, &req); err == nil && req.Query != "" {
			req.Query = canonGQL(req.Query)
			if b, err := json.Marshal(req); err == nil {
				return b
			}
		}
		return body
	}))

	// Setup WebSocket for subscriptions
	upgrader := websocket.Upgrader{
		Subprotocols: []string{"graphql-ws"},
		CheckOrigin:  func(r *http.Request) bool { return true },
	}
	http.HandleFunc("/sub", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WS upgrade error: %v", err)
			return
		}
		defer conn.Close()

		type wsMsg struct {
			ID      string `json:"id,omitempty"`
			Type    string `json:"type"`
			Payload struct {
				Query string `json:"query"`
			} `json:"payload,omitempty"`
		}

		for {
			_, msgData, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg wsMsg
			if err := json.Unmarshal(msgData, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "connection_init":
				_ = conn.WriteJSON(map[string]any{"type": "connection_ack"})
			case "start":
				go runSubscription(conn, msg.ID, msg.Payload.Query)
			case "stop", "complete":
				return
			}
		}
	})

	log.Printf("GraphQL server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func runSubscription(conn *websocket.Conn, opID, query string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Apply prefix stripping
	query = canonGQL(query)

	ch := graphql.Subscribe(graphql.Params{
		Schema:        gSchema,
		RequestString: query,
		Context:       ctx,
	})

	for res := range ch {
		payload := map[string]any{"data": res.Data}
		if len(res.Errors) > 0 {
			payload["errors"] = res.Errors
		}
		_ = conn.WriteJSON(map[string]any{"type": "data", "id": opID, "payload": payload})
	}
	_ = conn.WriteJSON(map[string]any{"type": "complete", "id": opID})
}

// canonGQL removes provider prefixes from GraphQL queries
func canonGQL(q string) string {
	q = strings.ReplaceAll(q, "graphqlsub.", "")
	q = strings.ReplaceAll(q, "graphql.", "")
	return q
}

// withBodyRewrite wraps an HTTP handler to rewrite the request body
func withBodyRewrite(next http.Handler, rewrite func([]byte) []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			body = rewrite(body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	go startServer(":8080")
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("UTCP client init error: %v", err)
	}

	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("tool search error: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	// Example query
	res, err := client.CallTool(ctx, "graphql.echo", map[string]any{"msg": "hi"})
	if err != nil {
		log.Fatalf("query call error: %v", err)
	}
	log.Printf("Query result: %#v", res)

	// Example subscription
	subRes, err := client.CallTool(ctx, "graphqlsub.updates", nil)
	if err != nil {
		log.Fatalf("subscription call error: %v", err)
	}
	sub, ok := subRes.(*graphqltransport.SubscriptionResult)
	if !ok {
		log.Fatalf("unexpected subscription type: %T", subRes)
	}
	for {
		val, err := sub.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("subscription next error: %v", err)
		}
		log.Printf("Subscription update: %#v", val)
	}
	sub.Close()
}
