package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/universal-tool-calling-protocol/go-utcp/src/adk"
)

func main() {
	researchAgent := adk.NewAgent(
		"research",
		adk.WithDescription("Collects curated insights for the swarm"),
	)
	researchAgent.MustRegisterTool(adk.ToolDefinition{
		Name:        "gather_insights",
		Description: "Returns a short list of insights for a goal",
		Handler:     gatherInsights,
	})

	plannerAgent := adk.NewAgent(
		"planner",
		adk.WithDescription("Designs actionable plans from shared context"),
	)
	plannerAgent.MustRegisterTool(adk.ToolDefinition{
		Name:        "draft_plan",
		Description: "Produces a coordinated action plan",
		Handler:     draftPlan,
	})

	reviewerAgent := adk.NewAgent(
		"reviewer",
		adk.WithDescription("Polishes plans into user-ready guidance"),
	)
	reviewerAgent.MustRegisterTool(adk.ToolDefinition{
		Name:        "finalise",
		Description: "Transforms the draft into a polished recommendation",
		Handler:     finalisePlan,
	})

	coordinator := adk.NewLLMAgent(
		"swarm-coordinator",
		&swarmLLM{},
		adk.WithLLMToolName("coordinate"),
		adk.WithLLMToolDescription("Coordinates a swarm of specialist agents"),
		adk.WithLLMToolTags("swarm", "multi-agent"),
		adk.WithAgentOptions(adk.WithDescription("Rule-based swarm coordinator")),
	)

	must(coordinator.RegisterSubAgent(&adk.SubAgent{Name: "research", Description: "Looks up background insights", Agent: researchAgent}))
	must(coordinator.RegisterSubAgent(&adk.SubAgent{Name: "planner", Description: "Turns insights into plans", Agent: plannerAgent}))
	must(coordinator.RegisterSubAgent(&adk.SubAgent{Name: "reviewer", Description: "Crafts polished recommendations", Agent: reviewerAgent}))

	goal := "Design a remote-friendly winter offsite that emphasises knowledge sharing."

	steps := []struct {
		phase  string
		prompt func(previous map[string]any) string
	}{
		{
			phase: "research",
			prompt: func(_ map[string]any) string {
				return fmt.Sprintf("[phase:research]\nGoal: %s", goal)
			},
		},
		{
			phase: "planning",
			prompt: func(previous map[string]any) string {
				insights := extractInsights(previous)
				var ctx string
				if len(insights) > 0 {
					ctx = fmt.Sprintf("Research findings: %s", strings.Join(insights, "; "))
				}
				return fmt.Sprintf("[phase:planning]\nGoal: %s\n%s", goal, ctx)
			},
		},
		{
			phase: "review",
			prompt: func(previous map[string]any) string {
				draft := extractPlan(previous)
				return fmt.Sprintf("[phase:review]\nGoal: %s\nDraft plan: %s", goal, draft)
			},
		},
	}

	ctx := context.Background()
	var last map[string]any
	for idx, step := range steps {
		prompt := strings.TrimSpace(step.prompt(last))
		result, err := coordinator.Call(ctx, "coordinate", map[string]any{"prompt": prompt})
		if err != nil {
			log.Fatalf("step %d (%s) failed: %v", idx+1, step.phase, err)
		}

		fmt.Printf("=== Step %d: %s ===\n", idx+1, strings.Title(step.phase))
		fmt.Printf("Prompt:\n%s\n", prompt)

		invocation := asMap(result["invocation"])
		fmt.Printf("Delegated to: %s/%s\n", invocationString(invocation, "sub_agent"), invocationString(invocation, "tool"))
		fmt.Printf("Reason: %s\n", invocationString(invocation, "reason"))

		fmt.Printf("LLM summary: %s\n", stringOrDefault(result["response"], "(no summary)"))

		toolResult, _ := json.MarshalIndent(result["tool_result"], "", "  ")
		fmt.Printf("Tool result:%s\n\n", formatJSON(toolResult))

		last = result
	}
}

func gatherInsights(_ context.Context, input map[string]any) (map[string]any, error) {
	topic := strings.ToLower(strings.TrimSpace(fmt.Sprint(input["topic"])))
	if topic == "" {
		topic = "team collaboration"
	}

	knowledgeBase := map[string][]string{
		"remote-friendly winter offsite": {
			"Choose a location with hybrid broadcast support",
			"Blend short talks with interactive workshops",
			"Include optional outdoor activities with warm-up stations",
		},
		"team collaboration": {
			"Schedule time for pair rotations",
			"Highlight recent wins to build momentum",
			"Use asynchronous boards to collect ideas ahead of time",
		},
	}

	insights := knowledgeBase["team collaboration"]
	for key, values := range knowledgeBase {
		if strings.Contains(topic, key) {
			insights = values
			break
		}
	}

	return map[string]any{"insights": insights}, nil
}

func draftPlan(_ context.Context, input map[string]any) (map[string]any, error) {
	goal := strings.TrimSpace(fmt.Sprint(input["goal"]))
	if goal == "" {
		goal = "Deliver value to the team"
	}

	insights := normaliseList(input["insights"])
	if len(insights) == 0 {
		insights = []string{
			"Keep sessions short for remote engagement",
			"Mix presentation and collaboration formats",
			"Celebrate success stories to motivate attendees",
		}
	}

	agenda := []string{
		fmt.Sprintf("Kickoff: share the goal — %s", goal),
		fmt.Sprintf("Insight spotlight: %s", insights[0]),
	}
	if len(insights) > 1 {
		agenda = append(agenda, fmt.Sprintf("Collaboration lab: %s", insights[1]))
	}
	if len(insights) > 2 {
		agenda = append(agenda, fmt.Sprintf("Recharge & reflect: %s", insights[2]))
	}

	return map[string]any{
		"plan":       strings.Join(agenda, " | "),
		"highlights": insights,
	}, nil
}

func finalisePlan(_ context.Context, input map[string]any) (map[string]any, error) {
	plan := strings.TrimSpace(fmt.Sprint(input["plan"]))
	goal := strings.TrimSpace(fmt.Sprint(input["goal"]))
	if goal == "" {
		goal = "Deliver value to the team"
	}

	summary := fmt.Sprintf("Goal: %s. Recommended flow: %s. Close with a retro to capture feedback and next steps.", goal, plan)
	return map[string]any{"summary": summary}, nil
}

type swarmLLM struct{}

func (s *swarmLLM) Generate(_ context.Context, prompt string) (string, error) {
	if strings.Contains(prompt, "<<USER_REQUEST>>") {
		request := between(prompt, "<<USER_REQUEST>>", "<<END_USER_REQUEST>>")
		phase := parsePhase(request)
		goal := parseLine(request, "goal:")

		switch phase {
		case "planning":
			insights := parseList(request, "research findings:")
			plan := map[string]any{
				"sub_agent": "planner",
				"tool":      "draft_plan",
				"arguments": map[string]any{"goal": goal, "insights": insights},
				"reason":    "Translate the research into a concrete plan.",
			}
			return encodePlan(plan)
		case "review":
			draft := parseLine(request, "draft plan:")
			plan := map[string]any{
				"sub_agent": "reviewer",
				"tool":      "finalise",
				"arguments": map[string]any{"goal": goal, "plan": draft},
				"reason":    "Polish the draft into a user-facing recommendation.",
			}
			return encodePlan(plan)
		default:
			plan := map[string]any{
				"sub_agent": "research",
				"tool":      "gather_insights",
				"arguments": map[string]any{"topic": goal},
				"reason":    "Start by gathering insights relevant to the goal.",
			}
			return encodePlan(plan)
		}
	}

	if strings.Contains(prompt, "<<TOOL_CONTEXT>>") {
		payload := between(prompt, "<<TOOL_CONTEXT>>", "<<END_TOOL_CONTEXT>>")
		var data map[string]any
		if err := json.Unmarshal([]byte(payload), &data); err != nil {
			return "", err
		}
		sub := strings.TrimSpace(fmt.Sprint(data["sub_agent"]))
		toolResult := asMap(data["tool_result"])

		switch sub {
		case "research":
			insights := normaliseList(toolResult["insights"])
			if len(insights) == 0 {
				return "Research agent had no findings, so continue exploring questions with the team.", nil
			}
			return fmt.Sprintf("Research gathered: %s.", strings.Join(insights, "; ")), nil
		case "planner":
			plan := strings.TrimSpace(fmt.Sprint(toolResult["plan"]))
			if plan == "" {
				return "Planner produced an empty plan; request more detail.", nil
			}
			return fmt.Sprintf("Draft plan ready: %s", plan), nil
		case "reviewer":
			summary := strings.TrimSpace(fmt.Sprint(toolResult["summary"]))
			if summary == "" {
				return "Here is the refined recommendation ready to share.", nil
			}
			return summary, nil
		default:
			return fmt.Sprintf("Tool output: %v", toolResult), nil
		}
	}

	return "Ready to coordinate the swarm of agents!", nil
}

func encodePlan(plan map[string]any) (string, error) {
	encoded, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func between(s, start, end string) string {
	startIdx := strings.Index(strings.ToLower(s), strings.ToLower(start))
	if startIdx == -1 {
		return ""
	}
	startIdx += len(start)
	endIdx := strings.Index(strings.ToLower(s[startIdx:]), strings.ToLower(end))
	if endIdx == -1 {
		return strings.TrimSpace(s[startIdx:])
	}
	return strings.TrimSpace(s[startIdx : startIdx+endIdx])
}

func parsePhase(request string) string {
	lower := strings.ToLower(request)
	start := strings.Index(lower, "[phase:")
	if start == -1 {
		return ""
	}
	start += len("[phase:")
	end := strings.Index(lower[start:], "]")
	if end == -1 {
		return strings.TrimSpace(lower[start:])
	}
	return strings.TrimSpace(lower[start : start+end])
}

func parseLine(request, prefix string) string {
	lowerPrefix := strings.ToLower(prefix)
	for _, line := range strings.Split(request, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), lowerPrefix) {
			value := strings.TrimSpace(trimmed[len(prefix):])
			if value == "" {
				continue
			}
			return value
		}
	}
	return ""
}

func parseList(request, prefix string) []string {
	raw := parseLine(request, prefix)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ";")
	var out []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func normaliseList(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			str := strings.TrimSpace(fmt.Sprint(item))
			if str != "" {
				out = append(out, str)
			}
		}
		return out
	case string:
		return parseList(v, "")
	default:
		return nil
	}
}

func asMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if existing, ok := value.(map[string]any); ok {
		return existing
	}
	return map[string]any{}
}

func invocationString(invocation map[string]any, key string) string {
	return strings.TrimSpace(fmt.Sprint(invocation[key]))
}

func stringOrDefault(value any, fallback string) string {
	if str := strings.TrimSpace(fmt.Sprint(value)); str != "" {
		return str
	}
	return fallback
}

func extractInsights(result map[string]any) []string {
	if result == nil {
		return nil
	}
	toolResult := asMap(result["tool_result"])
	return normaliseList(toolResult["insights"])
}

func extractPlan(result map[string]any) string {
	if result == nil {
		return ""
	}
	toolResult := asMap(result["tool_result"])
	plan := strings.TrimSpace(fmt.Sprint(toolResult["plan"]))
	if plan == "" {
		plan = strings.TrimSpace(fmt.Sprint(toolResult["summary"]))
	}
	return plan
}

func formatJSON(raw []byte) string {
	if len(raw) == 0 {
		return " {}"
	}
	return "\n" + string(raw)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
