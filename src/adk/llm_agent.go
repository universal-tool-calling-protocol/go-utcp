package adk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// LLM represents the minimal surface required by the LLMAgent to coordinate
// calls to sub-agents. Implementations can wrap network clients or provide
// deterministic, rule-based behaviour for testing.
type LLM interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// PlannerFunc describes the function signature used to select which sub-agent
// and tool should be executed to satisfy a user prompt.
type PlannerFunc func(ctx context.Context, llm LLM, userPrompt string, subAgents []*SubAgent) (*PlanResult, error)

// SummaryFunc turns the raw tool output into a final LLM response.
type SummaryFunc func(ctx context.Context, llm LLM, userPrompt string, invocation ToolInvocation, toolResult map[string]any) (string, error)

// SubAgent represents an agent that can be delegated to by an LLMAgent.
type SubAgent struct {
	Name        string
	Description string
	Agent       *Agent
}

// ToolInvocation captures the decision produced by the planner.
type ToolInvocation struct {
	SubAgent  string         `json:"sub_agent"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
	Reason    string         `json:"reason"`
}

// PlanResult is the structured output of the planner along with the raw LLM
// response for debugging purposes.
type PlanResult struct {
	Invocation ToolInvocation
	Raw        string
}

type llmAgentConfig struct {
	agentOptions   []AgentOption
	planner        PlannerFunc
	summarizer     SummaryFunc
	toolName       string
	toolDesc       string
	toolTags       []string
	averageToolRes *int
}

// LLMAgentOption mutates the configuration used to construct a new LLMAgent.
type LLMAgentOption func(*llmAgentConfig)

// WithAgentOptions forwards options to the underlying Agent constructed for the
// LLMAgent.
func WithAgentOptions(opts ...AgentOption) LLMAgentOption {
	return func(cfg *llmAgentConfig) {
		cfg.agentOptions = append(cfg.agentOptions, opts...)
	}
}

// WithPlanner overrides the planner function used by the LLMAgent.
func WithPlanner(planner PlannerFunc) LLMAgentOption {
	return func(cfg *llmAgentConfig) {
		if planner != nil {
			cfg.planner = planner
		}
	}
}

// WithSummarizer overrides the function used to produce the final response from
// the LLMAgent after a tool has been executed.
func WithSummarizer(summary SummaryFunc) LLMAgentOption {
	return func(cfg *llmAgentConfig) {
		if summary != nil {
			cfg.summarizer = summary
		}
	}
}

// WithLLMToolName customises the name registered for the orchestration tool.
func WithLLMToolName(name string) LLMAgentOption {
	return func(cfg *llmAgentConfig) {
		if strings.TrimSpace(name) != "" {
			cfg.toolName = name
		}
	}
}

// WithLLMToolDescription customises the description advertised for the LLM
// orchestration tool.
func WithLLMToolDescription(desc string) LLMAgentOption {
	return func(cfg *llmAgentConfig) {
		if strings.TrimSpace(desc) != "" {
			cfg.toolDesc = desc
		}
	}
}

// WithLLMToolTags overrides the tags registered on the orchestration tool.
func WithLLMToolTags(tags ...string) LLMAgentOption {
	return func(cfg *llmAgentConfig) {
		cfg.toolTags = append([]string(nil), tags...)
	}
}

// WithLLMToolAverageResponse sets the average response size metadata for the
// orchestration tool.
func WithLLMToolAverageResponse(size *int) LLMAgentOption {
	return func(cfg *llmAgentConfig) {
		cfg.averageToolRes = size
	}
}

// LLMAgent coordinates a group of sub-agents using an LLM to plan and summarise
// tool usage.
type LLMAgent struct {
	*Agent

	llm        LLM
	planner    PlannerFunc
	summarizer SummaryFunc

	mu            sync.RWMutex
	subAgents     map[string]*SubAgent
	subAgentOrder []string
	toolName      string
}

// NewLLMAgent creates a new LLM powered agent capable of delegating work to
// sub-agents registered via RegisterSubAgent. The LLMAgent itself exposes a
// single UTCP tool that accepts a prompt and returns the LLM crafted response
// together with metadata about the underlying tool invocation.
func NewLLMAgent(name string, llm LLM, opts ...LLMAgentOption) *LLMAgent {
	if llm == nil {
		panic("llm must not be nil")
	}

	cfg := &llmAgentConfig{
		planner:    defaultPlanner,
		summarizer: defaultSummarizer,
		toolName:   "chat",
		toolDesc:   "Plan a tool call across sub-agents and provide an LLM crafted reply.",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	underlying := NewAgent(name, cfg.agentOptions...)
	llmAgent := &LLMAgent{
		Agent:      underlying,
		llm:        llm,
		planner:    cfg.planner,
		summarizer: cfg.summarizer,
		subAgents:  make(map[string]*SubAgent),
		toolName:   cfg.toolName,
	}

	llmAgent.MustRegisterTool(ToolDefinition{
		Name:                llmAgent.toolName,
		Description:         cfg.toolDesc,
		Tags:                cfg.toolTags,
		AverageResponseSize: cfg.averageToolRes,
		Inputs: &tools.ToolInputOutputSchema{
			Type: "object",
			Properties: map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "The user request the LLM agent should satisfy.",
				},
				"context": map[string]any{
					"type":        "object",
					"description": "Optional contextual information to share with the planner.",
				},
			},
			Required: []string{"prompt"},
		},
		Outputs: &tools.ToolInputOutputSchema{
			Type: "object",
			Properties: map[string]any{
				"response": map[string]any{
					"type":        "string",
					"description": "Natural language response crafted by the LLM.",
				},
				"invocation": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"sub_agent": map[string]any{"type": "string"},
						"tool":      map[string]any{"type": "string"},
						"arguments": map[string]any{"type": "object"},
						"reason":    map[string]any{"type": "string"},
					},
				},
				"tool_result": map[string]any{
					"type":        "object",
					"description": "Raw result returned by the delegated tool.",
				},
				"planner_response": map[string]any{
					"type":        "string",
					"description": "Raw planner output before JSON decoding.",
				},
			},
			Required: []string{"response", "invocation"},
		},
		Handler: llmAgent.handleChat,
	})

	return llmAgent
}

// RegisterSubAgent registers a new sub-agent that the LLMAgent can delegate to.
func (a *LLMAgent) RegisterSubAgent(sub *SubAgent) error {
	if sub == nil {
		return errors.New("sub-agent cannot be nil")
	}
	if sub.Agent == nil {
		return errors.New("sub-agent must reference an Agent")
	}
	if strings.TrimSpace(sub.Name) == "" {
		return errors.New("sub-agent must have a name")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.subAgents[sub.Name]; exists {
		return fmt.Errorf("sub-agent %q already registered", sub.Name)
	}
	copy := *sub
	a.subAgents[sub.Name] = &copy
	a.subAgentOrder = append(a.subAgentOrder, sub.Name)
	return nil
}

// SubAgents returns a snapshot of registered sub-agents.
func (a *LLMAgent) SubAgents() []*SubAgent {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*SubAgent, 0, len(a.subAgents))
	for _, name := range a.subAgentOrder {
		if sub, ok := a.subAgents[name]; ok {
			copy := *sub
			out = append(out, &copy)
		}
	}
	return out
}

func (a *LLMAgent) handleChat(ctx context.Context, input map[string]any) (map[string]any, error) {
	prompt, _ := input["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("prompt is required")
	}

	plan, err := a.plan(ctx, prompt)
	if err != nil {
		return nil, err
	}

	sub, err := a.lookupSubAgent(plan.Invocation.SubAgent)
	if err != nil {
		return nil, err
	}

	arguments := plan.Invocation.Arguments
	if arguments == nil {
		arguments = map[string]any{}
	}

	result, err := sub.Agent.Call(ctx, plan.Invocation.Tool, arguments)
	if err != nil {
		return nil, fmt.Errorf("sub-agent %q tool %q failed: %w", sub.Name, plan.Invocation.Tool, err)
	}

	response, err := a.summarizer(ctx, a.llm, prompt, plan.Invocation, result)
	if err != nil {
		return nil, fmt.Errorf("failed to synthesise response: %w", err)
	}

	invocation := map[string]any{
		"sub_agent": sub.Name,
		"tool":      plan.Invocation.Tool,
		"arguments": arguments,
		"reason":    plan.Invocation.Reason,
	}

	return map[string]any{
		"response":         response,
		"invocation":       invocation,
		"tool_result":      result,
		"planner_response": plan.Raw,
	}, nil
}

func (a *LLMAgent) plan(ctx context.Context, prompt string) (*PlanResult, error) {
	subAgents := a.SubAgents()
	if len(subAgents) == 0 {
		return nil, errors.New("no sub-agents registered")
	}

	plan, err := a.planner(ctx, a.llm, prompt, subAgents)
	if err != nil {
		return nil, fmt.Errorf("planner failed: %w", err)
	}
	if plan == nil {
		return nil, errors.New("planner returned nil plan")
	}
	plan.Invocation.SubAgent = strings.TrimSpace(plan.Invocation.SubAgent)
	plan.Invocation.Tool = strings.TrimSpace(plan.Invocation.Tool)
	if plan.Invocation.SubAgent == "" {
		return nil, errors.New("planner did not specify a sub-agent")
	}
	if plan.Invocation.Tool == "" {
		return nil, errors.New("planner did not specify a tool")
	}
	return plan, nil
}

func (a *LLMAgent) lookupSubAgent(name string) (*SubAgent, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	sub, ok := a.subAgents[name]
	if !ok {
		return nil, fmt.Errorf("unknown sub-agent %q", name)
	}
	return sub, nil
}

func defaultPlanner(ctx context.Context, llm LLM, userPrompt string, subAgents []*SubAgent) (*PlanResult, error) {
	var b strings.Builder
	b.WriteString("You are an orchestration agent responsible for selecting the best UTCP tool to satisfy a request.\n")
	b.WriteString("Consider the following sub-agents and tools:\n")

	sort.SliceStable(subAgents, func(i, j int) bool { return subAgents[i].Name < subAgents[j].Name })

	for _, sub := range subAgents {
		b.WriteString(fmt.Sprintf("Sub-agent %q: %s\n", sub.Name, sub.Description))
		tools := sub.Agent.Tools()
		sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
		for _, tool := range tools {
			b.WriteString(fmt.Sprintf("  - Tool %q: %s\n", tool.Name, tool.Description))
		}
	}

	b.WriteString("\n<<USER_REQUEST>>\n")
	b.WriteString(userPrompt)
	b.WriteString("\n<<END_USER_REQUEST>>\n")
	b.WriteString("Respond with a JSON object containing keys \"sub_agent\", \"tool\", \"arguments\" (an object), and \"reason\" explaining your choice.\n")

	raw, err := llm.Generate(ctx, b.String())
	if err != nil {
		return nil, err
	}

	plan, err := decodePlannerResponse(raw)
	if err != nil {
		return nil, err
	}
	return &PlanResult{Invocation: *plan, Raw: raw}, nil
}

func decodePlannerResponse(raw string) (*ToolInvocation, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.Trim(trimmed, "`\n")
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end >= start {
		trimmed = trimmed[start : end+1]
	}

	var invocation ToolInvocation
	if err := json.Unmarshal([]byte(trimmed), &invocation); err != nil {
		return nil, fmt.Errorf("failed to decode planner response: %w", err)
	}
	if invocation.Arguments == nil {
		invocation.Arguments = map[string]any{}
	}
	return &invocation, nil
}

func defaultSummarizer(ctx context.Context, llm LLM, userPrompt string, invocation ToolInvocation, toolResult map[string]any) (string, error) {
	payload := map[string]any{
		"user_prompt":    userPrompt,
		"sub_agent":      invocation.SubAgent,
		"tool":           invocation.Tool,
		"arguments":      invocation.Arguments,
		"tool_result":    toolResult,
		"planner_reason": invocation.Reason,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("You are assisting with a final response summarisation. Provide a concise, helpful message for the user based on the executed tool.\n")
	b.WriteString("Use the following JSON context to craft the reply:\n")
	b.WriteString("<<TOOL_CONTEXT>>\n")
	b.WriteString(string(encoded))
	b.WriteString("\n<<END_TOOL_CONTEXT>>\n")

	summary, err := llm.Generate(ctx, b.String())
	if err != nil {
		return "", err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = fmt.Sprintf("Tool %s from sub-agent %s returned: %v", invocation.Tool, invocation.SubAgent, toolResult)
	}
	return summary, nil
}
