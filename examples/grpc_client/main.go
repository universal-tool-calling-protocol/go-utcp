package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/grpcpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type server struct {
	grpcpb.UnimplementedUTCPServiceServer
}

// GetManual advertises available tools and their descriptions
func (s *server) GetManual(ctx context.Context, e *grpcpb.Empty) (*grpcpb.Manual, error) {
	return &grpcpb.Manual{
		Version: "1.2",
		Tools: []*grpcpb.Tool{
			{Name: "echo", Description: "Echoes back the provided message"},
			{Name: "reverse", Description: "Reverses the provided string"},
			{Name: "sum", Description: "Calculates the sum of a list of numbers"},
			{Name: "timestamp", Description: "Returns the current server timestamp in RFC3339 format"},
			{Name: "uppercase", Description: "Converts the provided string to uppercase"},
			{Name: "wordcount", Description: "Counts the number of words in the provided string"},
		},
	}, nil
}

// CallTool dispatches calls to the appropriate tool implementation
func (s *server) CallTool(ctx context.Context, req *grpcpb.ToolCallRequest) (*grpcpb.ToolCallResponse, error) {
	// Parse arguments
	var args map[string]any
	if err := json.Unmarshal([]byte(req.ArgsJson), &args); err != nil {
		return nil, err
	}

	var result any
	switch req.Tool {
	case "grpc.echo":
		if msg, ok := args["msg"].(string); ok {
			result = msg
		}

	case "grpc.reverse":
		if msg, ok := args["msg"].(string); ok {
			runes := []rune(msg)
			for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
				runes[i], runes[j] = runes[j], runes[i]
			}
			result = string(runes)
		}

	case "grpc.sum":
		if list, ok := args["numbers"].([]any); ok {
			sum := 0.0
			for _, n := range list {
				switch v := n.(type) {
				case float64:
					sum += v
				case string:
					if f, err := strconv.ParseFloat(v, 64); err == nil {
						sum += f
					}
				}
			}
			result = sum
		}

	case "grpc.timestamp":
		result = time.Now().Format(time.RFC3339)

	case "grpc.uppercase":
		if msg, ok := args["msg"].(string); ok {
			result = strings.ToUpper(msg)
		}

	case "grpc.wordcount":
		if msg, ok := args["msg"].(string); ok {
			words := strings.Fields(msg)
			result = len(words)
		}

	default:
		result = map[string]string{"error": "unknown tool " + req.Tool}
	}

	// Marshal and return
	out, err := json.Marshal(map[string]any{"result": result})
	if err != nil {
		return nil, err
	}
	return &grpcpb.ToolCallResponse{ResultJson: string(out)}, nil
}

func startServer(addr string) *grpc.Server {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	grpcpb.RegisterUTCPServiceServer(s, &server{})
	reflection.Register(s)
	go s.Serve(lis)
	return s
}

func main() {
	srv := startServer("127.0.0.1:9090")
	defer srv.Stop()
	// wait for server to start
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	// Discover tools
	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	// Example calls
	for _, call := range []struct {
		method string
		args   map[string]any
	}{
		{"grpc.echo", map[string]any{"msg": "Hello, world!"}},
		{"grpc.reverse", map[string]any{"msg": "Hello"}},
		{"grpc.sum", map[string]any{"numbers": []any{1, 2, 3.5, "4.5"}}},
		{"grpc.timestamp", map[string]any{}},
		{"grpc.uppercase", map[string]any{"msg": "hello"}},
		{"grpc.wordcount", map[string]any{"msg": "Hello world from Go server"}},
	} {
		res, err := client.CallTool(ctx, call.method, call.args)
		if err != nil {
			log.Fatalf("call %s error: %v", call.method, err)
		}
		log.Printf("%s -> %v", call.method, res.(map[string]any)["result"])
	}
}
