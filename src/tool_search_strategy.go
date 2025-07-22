package src

import (
	"context"
)

// ToolSearchStrategy is an interface for any component
// that knows how to search for tools based on a query.
type ToolSearchStrategy interface {
	// SearchTools returns up to `limit` tools matching `query`.
	// A limit of 0 means “no limit” (return all matches).
	//
	// ctx carries deadlines, cancellation signals, and other request-scoped values.
	SearchTools(ctx context.Context, query string, limit int) ([]Tool, error)
}
