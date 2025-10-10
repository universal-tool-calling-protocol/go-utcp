package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/universal-tool-calling-protocol/go-utcp/src/adk"
)

func main() {
	mathAgent := adk.NewAgent(
		"math",
		adk.WithDescription("Performs arithmetic operations"),
	)
	mathAgent.MustRegisterTool(adk.ToolDefinition{
		Name:        "add",
		Description: "Adds two numbers together",
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			a, _ := input["a"].(float64)
			b, _ := input["b"].(float64)
			return map[string]any{"sum": a + b}, nil
		},
	})

	knowledgeAgent := adk.NewAgent(
		"knowledge",
		adk.WithDescription("Shares helpful trivia"),
	)
	knowledgeAgent.MustRegisterTool(adk.ToolDefinition{
		Name:        "fact",
		Description: "Returns a fun fact for a topic",
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			topic := strings.TrimSpace(fmt.Sprint(input["topic"]))
			if topic == "" {
				topic = "the world"
			}
			return map[string]any{"fact": fmt.Sprintf("Did you know? %s has fascinating details!", topic)}, nil
		},
	})

	coordinator := adk.NewLLMAgent(
		"llm-coordinator",
		&ruleBasedLLM{},
		adk.WithAgentOptions(adk.WithDescription("LLM coordinator that delegates to specialist agents")),
		adk.WithLLMToolName("respond"),
		adk.WithLLMToolDescription("Uses an LLM planner to pick the right sub-agent tool and craft a response."),
		adk.WithLLMToolTags("llm", "coordinator"),
	)

	if err := coordinator.RegisterSubAgent(&adk.SubAgent{Name: "math", Description: "Solves basic math problems", Agent: mathAgent}); err != nil {
		log.Fatalf("register math agent: %v", err)
	}
	if err := coordinator.RegisterSubAgent(&adk.SubAgent{Name: "knowledge", Description: "Provides short facts", Agent: knowledgeAgent}); err != nil {
		log.Fatalf("register knowledge agent: %v", err)
	}

	prompts := []string{
		"What is 12 + 30?",
		"Share a fact about penguins",
	}

	for _, prompt := range prompts {
		result, err := coordinator.Call(context.Background(), "respond", map[string]any{"prompt": prompt})
		if err != nil {
			log.Fatalf("call failed: %v", err)
		}
		fmt.Printf("Prompt: %s\nResponse: %s\n\n", prompt, result["response"])
	}
}

type ruleBasedLLM struct{}

func (r *ruleBasedLLM) Generate(ctx context.Context, prompt string) (string, error) {
	if strings.Contains(prompt, "<<USER_REQUEST>>") {
		request := between(prompt, "<<USER_REQUEST>>", "<<END_USER_REQUEST>>")
		lower := strings.ToLower(request)

		if strings.Contains(lower, "fact") || strings.Contains(lower, "penguin") {
			plan := map[string]any{
				"sub_agent": "knowledge",
				"tool":      "fact",
				"arguments": map[string]any{"topic": request},
				"reason":    "The knowledge agent specialises in trivia.",
			}
			encoded, _ := json.Marshal(plan)
			return string(encoded), nil
		}

		numbers := numberPattern.FindAllString(lower, -1)
		if len(numbers) >= 2 {
			a, _ := strconv.ParseFloat(numbers[0], 64)
			b, _ := strconv.ParseFloat(numbers[1], 64)
			plan := map[string]any{
				"sub_agent": "math",
				"tool":      "add",
				"arguments": map[string]any{"a": a, "b": b},
				"reason":    "Math agent can sum the provided numbers.",
			}
			encoded, _ := json.Marshal(plan)
			return string(encoded), nil
		}

		fallback := map[string]any{
			"sub_agent": "knowledge",
			"tool":      "fact",
			"arguments": map[string]any{"topic": request},
			"reason":    "Defaulting to a knowledge lookup for general prompts.",
		}
		encoded, _ := json.Marshal(fallback)
		return string(encoded), nil
	}

	if strings.Contains(prompt, "<<TOOL_CONTEXT>>") {
		payload := between(prompt, "<<TOOL_CONTEXT>>", "<<END_TOOL_CONTEXT>>")
		var data map[string]any
		if err := json.Unmarshal([]byte(payload), &data); err != nil {
			return "", err
		}
		toolResult, _ := data["tool_result"].(map[string]any)
		if fact, ok := toolResult["fact"].(string); ok && fact != "" {
			return fact, nil
		}
		if sum, ok := toolResult["sum"].(float64); ok {
			return fmt.Sprintf("The result is %.0f.", sum), nil
		}
		return fmt.Sprintf("Tool output: %v", toolResult), nil
	}

	return "I'm ready to help whenever you need me!", nil
}

var numberPattern = regexp.MustCompile(`[-+]?[0-9]*\.?[0-9]+`)

func between(s, start, end string) string {
	startIdx := strings.Index(s, start)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(start)
	endIdx := strings.Index(s[startIdx:], end)
	if endIdx == -1 {
		return strings.TrimSpace(s[startIdx:])
	}
	return strings.TrimSpace(s[startIdx : startIdx+endIdx])
}
