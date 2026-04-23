package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	port        int
	router      Router
	server      *http.Server
	defaultMode router.RouteMode
	apiKey      string
}

// New creates a new proxy Server listening on the given port with a default route mode.
func New(port int, r Router, mode router.RouteMode, apiKey string) *Server {
	return &Server{
		port:        port,
		router:      r,
		defaultMode: mode,
		apiKey:      apiKey,
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

	// API key authentication
	if s.apiKey != "" {
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" || token == authHeader {
			// No Bearer token — also check x-api-key header
			token = r.Header.Get("x-api-key")
		}
		if token != s.apiKey {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"error": map[string]string{
					"message": "invalid or missing API key",
					"type":    "authentication_error",
				},
			})
			return
		}
	}

	body, err := provider.ReadBody(r.Body, 1<<20) // 1MB limit
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string      `json:"role"`
			Content interface{} `json:"content"`
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

	// Determine route mode based on model field:
	// - "auto" or empty → use default mode
	// - specific model name → manual mode (route to that model)
	mode := s.defaultMode
	modelID := ""
	switch {
	case req.Model == "" || req.Model == "auto":
		// Use default mode
	case req.Model == "race":
		mode = router.RouteRace
	default:
		// Specific model name → manual mode
		mode = router.RouteManual
		modelID = req.Model
	}

	// Header override still takes priority
	if h := r.Header.Get("X-Router-Mode"); h != "" {
		mode = router.RouteModeFromString(h)
	}

	routeReq := &router.RouteRequest{
		Messages: msgs,
		Mode:     mode,
		ModelID:  modelID,
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

	var hasSentContent bool
	var gotError bool

	for chunk := range result.Stream {
		if chunk.Error != nil {
			log.Printf("stream error: %v", chunk.Error)
			// Send error as SSE to client
			errResp := map[string]interface{}{
				"error": map[string]string{
					"message": chunk.Error.Error(),
					"type":    "stream_error",
				},
			}
			if b, err := json.Marshal(errResp); err == nil {
				fmt.Fprintf(w, "data: %s\n\n", b)
				if canFlush {
					flusher.Flush()
				}
			}
			gotError = true
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
		b, err := json.Marshal(data)
		if err != nil {
			log.Printf("json marshaling error on response chunk: %v", err)
			// Send error to client
			errResp := map[string]interface{}{
				"error": map[string]string{
					"message": fmt.Sprintf("internal error: failed to encode response: %v", err),
					"type":    "encoding_error",
				},
			}
			if bErr, errMarshal := json.Marshal(errResp); errMarshal == nil {
				fmt.Fprintf(w, "data: %s\n\n", bErr)
				if canFlush {
					flusher.Flush()
				}
			}
			gotError = true
			break
		}

		fmt.Fprintf(w, "data: %s\n\n", b)
		hasSentContent = true

		if canFlush {
			flusher.Flush()
		}

		if chunk.Done {
			break
		}
	}

	if !gotError {
		// If no error and no content was received, send an explicit error
		if !hasSentContent {
			errResp := map[string]interface{}{
				"error": map[string]string{
					"message": "no content was received from the model",
					"type":    "empty_response",
				},
			}
			if b, err := json.Marshal(errResp); err == nil {
				fmt.Fprintf(w, "data: %s\n\n", b)
				if canFlush {
					flusher.Flush()
				}
			}
		}
		// Always send [DONE] in OpenAI protocol when no error
		fmt.Fprint(w, "data: [DONE]\n\n")
		if canFlush {
			flusher.Flush()
		}
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
