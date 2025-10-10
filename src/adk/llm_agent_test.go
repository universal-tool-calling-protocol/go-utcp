package adk

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

type scriptedLLM struct {
	mu        sync.Mutex
	responses []llmResponse
	idx       int
}

type llmResponse struct {
	content string
	err     error
}

func newScriptedLLM(responses ...llmResponse) *scriptedLLM {
	return &scriptedLLM{responses: responses}
}

func (s *scriptedLLM) Generate(ctx context.Context, prompt string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idx >= len(s.responses) {
		return "", errors.New("unexpected llm call")
	}
	resp := s.responses[s.idx]
	s.idx++
	return resp.content, resp.err
}

func TestLLMAgentEndToEnd(t *testing.T) {
	calc := NewAgent("calculator")
	calc.MustRegisterTool(ToolDefinition{
		Name:        "add",
		Description: "Adds two numbers",
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			a, _ := input["a"].(float64)
			b, _ := input["b"].(float64)
			return map[string]any{"sum": a + b}, nil
		},
	})

	info := NewAgent("facts")
	info.MustRegisterTool(ToolDefinition{
		Name:        "lookup",
		Description: "Provides canned facts",
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			topic, _ := input["topic"].(string)
			return map[string]any{"fact": "Fact about " + topic}, nil
		},
	})

	llm := newScriptedLLM(
		llmResponse{content: `{"sub_agent":"calculator","tool":"add","arguments":{"a":2,"b":3},"reason":"math"}`},
		llmResponse{content: "The sum of 2 and 3 is 5."},
	)

	agent := NewLLMAgent("orchestrator", llm, WithLLMToolName("respond"))
	if err := agent.RegisterSubAgent(&SubAgent{Name: "calculator", Description: "Performs arithmetic", Agent: calc}); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	if err := agent.RegisterSubAgent(&SubAgent{Name: "facts", Description: "Returns facts", Agent: info}); err != nil {
		t.Fatalf("register facts: %v", err)
	}

	result, err := agent.Call(context.Background(), "respond", map[string]any{"prompt": "What is 2+3?"})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	if got := result["response"].(string); got != "The sum of 2 and 3 is 5." {
		t.Fatalf("unexpected response: %q", got)
	}

	invocation, ok := result["invocation"].(map[string]any)
	if !ok {
		t.Fatalf("invocation not map: %#v", result["invocation"])
	}
	if invocation["sub_agent"].(string) != "calculator" {
		t.Fatalf("unexpected sub agent: %#v", invocation["sub_agent"])
	}
	if invocation["tool"].(string) != "add" {
		t.Fatalf("unexpected tool: %#v", invocation["tool"])
	}

	toolRes, ok := result["tool_result"].(map[string]any)
	if !ok {
		t.Fatalf("tool_result not map: %#v", result["tool_result"])
	}
	if toolRes["sum"].(float64) != 5 {
		t.Fatalf("unexpected sum: %#v", toolRes["sum"])
	}
}

func TestLLMAgentPlannerErrors(t *testing.T) {
	calc := NewAgent("calculator")
	calc.MustRegisterTool(ToolDefinition{
		Name: "add",
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			return map[string]any{"sum": 0}, nil
		},
	})

	cases := []struct {
		name     string
		llm      *scriptedLLM
		wantErr  string
		register func(*LLMAgent)
	}{
		{
			name:    "invalid json",
			llm:     newScriptedLLM(llmResponse{content: "not json"}),
			wantErr: "failed to decode planner response",
		},
		{
			name:    "missing sub agent",
			llm:     newScriptedLLM(llmResponse{content: `{"tool":"add"}`}),
			wantErr: "planner did not specify a sub-agent",
		},
		{
			name:    "unknown sub agent",
			llm:     newScriptedLLM(llmResponse{content: `{"sub_agent":"unknown","tool":"add"}`}),
			wantErr: "unknown sub-agent",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := NewLLMAgent("coordinator", tc.llm)
			if tc.register != nil {
				tc.register(agent)
			} else {
				if err := agent.RegisterSubAgent(&SubAgent{Name: "calculator", Agent: calc}); err != nil {
					t.Fatalf("register sub agent: %v", err)
				}
			}

			_, err := agent.Call(context.Background(), "chat", map[string]any{"prompt": "test"})
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLLMAgentSummarizerError(t *testing.T) {
	calc := NewAgent("calculator")
	calc.MustRegisterTool(ToolDefinition{
		Name: "add",
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			return map[string]any{"sum": 10}, nil
		},
	})

	llm := newScriptedLLM(
		llmResponse{content: `{"sub_agent":"calculator","tool":"add"}`},
		llmResponse{content: "", err: errors.New("llm failure")},
	)

	agent := NewLLMAgent("coordinator", llm)
	if err := agent.RegisterSubAgent(&SubAgent{Name: "calculator", Agent: calc}); err != nil {
		t.Fatalf("register sub agent: %v", err)
	}

	_, err := agent.Call(context.Background(), "chat", map[string]any{"prompt": "add"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "failed to synthesise response") {
		t.Fatalf("unexpected error: %v", err)
	}
}
