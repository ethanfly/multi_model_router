package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"multi_model_router/internal/provider"
	"multi_model_router/internal/router"
)

// Router matches the router.Engine interface for routing requests.
type Router interface {
	Route(ctx context.Context, req *router.RouteRequest) *router.RouteResult
}

// Server is a local HTTP proxy that accepts OpenAI-compatible requests
// and routes them through the router engine.
type Server struct {
	port   int
	router Router
	server *http.Server
}

// New creates a new proxy Server listening on the given port.
func New(port int, r Router) *Server {
	return &Server{
		port:   port,
		router: r,
	}
}

// Start creates the HTTP mux and starts listening in a goroutine.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletion)
	mux.HandleFunc("/v1/messages", s.handleChatCompletion)
	mux.HandleFunc("/", s.handleNotFound)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("proxy server error: %v", err)
		}
	}()

	return nil
}

// Stop performs a graceful shutdown of the HTTP server.
func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// Port returns the port the server is configured to listen on.
func (s *Server) Port() int {
	return s.port
}

// handleChatCompletion handles POST /v1/chat/completions and /v1/messages.
func (s *Server) handleChatCompletion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := provider.ReadBody(r.Body, 1<<20) // 1MB limit
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	// Convert to provider.Message slice.
	msgs := make([]provider.Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = provider.Message{Role: m.Role, Content: m.Content}
	}

	// Determine route mode from header.
	mode := router.RouteAuto
	if r.Header.Get("X-Router-Mode") == "race" {
		mode = router.RouteRace
	}

	routeReq := &router.RouteRequest{
		Messages: msgs,
		Mode:     mode,
		ModelID:  req.Model,
	}

	result := s.router.Route(r.Context(), routeReq)
	if result == nil || result.Status != "success" {
		errMsg := "routing failed"
		if result != nil && result.ErrorMsg != "" {
			errMsg = result.ErrorMsg
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": errMsg})
		return
	}

	// Set response headers.
	w.Header().Set("X-Router-Model", result.ModelName)
	w.Header().Set("X-Router-Complexity", fmt.Sprintf("%d", result.Complexity))

	// Stream the response as SSE.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, canFlush := w.(http.Flusher)

	for chunk := range result.Stream {
		if chunk.Error != nil {
			log.Printf("stream error: %v", chunk.Error)
			break
		}

		data := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta": map[string]string{
						"content": chunk.Content,
					},
				},
			},
			"model": chunk.Model,
		}
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", b)

		if canFlush {
			flusher.Flush()
		}

		if chunk.Done {
			break
		}
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
	}
}

// handleNotFound returns a helpful JSON message for unmatched routes.
func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{
		"message": "Multi-Model Router proxy running. Use /v1/chat/completions",
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
