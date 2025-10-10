package adk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type callRequest struct {
	Tool  string         `json:"tool"`
	Input map[string]any `json:"input"`
}

type callResponse struct {
	Output map[string]any `json:"output,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// HTTPHandler returns an http.Handler that exposes the agent's manual and call endpoints.
func (a *Agent) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/manual", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(a.Manual()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	mux.HandleFunc("/call", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		var req callRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request payload", http.StatusBadRequest)
			return
		}
		if req.Tool == "" {
			http.Error(w, "tool field is required", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		output, err := a.Call(ctx, req.Tool, req.Input)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(callResponse{Error: err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(callResponse{Output: output})
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return mux
}

// ServeHTTP starts an HTTP server bound to the provided address and shuts down when the context is canceled.
func (a *Agent) ServeHTTP(ctx context.Context, addr string) error {
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:    addr,
		Handler: a.HTTPHandler(),
	}

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		close(done)
	}()

	err := server.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-done
	return nil
}
