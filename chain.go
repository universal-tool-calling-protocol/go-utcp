package utcp

import (
	"context"
	"fmt"
	"time"
)

// CallToolChain executes a chain of tool calls sequentially in Go (no JavaScript VM).
// It takes a slice of ChainStep definitions and passes each step's output into the next.
func (c *UtcpClient) CallToolChain(
	ctx context.Context,
	steps []ChainStep,
	timeout time.Duration,
) (map[string]any, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	results := make(map[string]any, len(steps))

	for i, step := range steps {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		// Merge previous outputs into current step inputs if requested
		inputs := make(map[string]any, len(step.Inputs))
		for k, v := range step.Inputs {
			inputs[k] = v
		}
		if step.UsePrevious {
			for k, v := range results {
				if _, exists := inputs[k]; !exists {
					inputs[k] = v
				}
			}
		}

		start := time.Now()
		result, err := c.CallTool(ctx, step.ToolName, inputs)
		elapsed := time.Since(start)

		if err != nil {
			return results, fmt.Errorf("step %d (%s) failed after %s: %w",
				i+1, step.ToolName, elapsed, err)
		}

		results[step.ToolName] = result
	}

	return results, nil
}

// ChainStep defines one step in a Go-native UTCP tool chain.
type ChainStep struct {
	ToolName    string         `json:"tool_name"`
	Inputs      map[string]any `json:"inputs,omitempty"`
	UsePrevious bool           `json:"use_previous,omitempty"`
}
