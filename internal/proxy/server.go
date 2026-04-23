package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// ModelLister provides model listing capability.
type ModelLister interface {
	ListActiveModels() []*router.ModelConfig
}

// ProviderResolver resolves a model reference to an upstream provider config.
type ProviderResolver interface {
	ResolveProvider(modelRef string) (router.ProviderInfo, bool)
}

// RequestLogEntry is the data passed to the OnRequestLog callback after each request.
type RequestLogEntry struct {
	ModelName  string
	Source     string
	Complexity int64
	RouteMode  string
	Status     string
	TokensIn   int
	TokensOut  int
	LatencyMs  int64
	ErrorMsg   string
}

// Server is a local HTTP proxy that accepts OpenAI-compatible and Anthropic-compatible
// requests and routes them through the router engine.
type Server struct {
	port         int
	router       Router
	server       *http.Server
	defaultMode  router.RouteMode
	apiKey       string
	httpClient   *http.Client
	OnRequestLog func(entry *RequestLogEntry) // optional stats callback
}

// requestStats tracks metrics during response writing.
type requestStats struct {
	tokensIn  int
	tokensOut int
	status    string // "success" or "error"
	errMsg    string
}

// New creates a new proxy Server listening on the given port with a default route mode.
func New(port int, r Router, mode router.RouteMode, apiKey string) *Server {
	return &Server{
		port:        port,
		router:      r,
		defaultMode: mode,
		apiKey:      apiKey,
		httpClient:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// Start creates the HTTP mux and starts listening in a goroutine.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Chat endpoints (full protocol support)
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletion)
	mux.HandleFunc("/v1/messages", s.handleChatCompletion)

	// Model listing endpoints
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/models/", s.handleModels)

	// Generic passthrough for all other /v1/* endpoints
	// Covers: embeddings, completions, images, audio, moderations, etc.
	mux.HandleFunc("/v1/", s.handlePassthrough)

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/", s.handleHealth)

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

// ---- Models Endpoint ----

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	ml, ok := s.router.(ModelLister)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model listing not supported"})
		return
	}

	models := ml.ListActiveModels()
	path := r.URL.Path

	// Single model: /v1/models/{id}
	if path != "/v1/models" && path != "/v1/models/" {
		modelID := strings.TrimPrefix(path, "/v1/models/")
		for _, m := range models {
			if m.ModelID == modelID || m.ID == modelID || m.Name == modelID {
				writeJSON(w, http.StatusOK, formatOpenAIModel(m))
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": map[string]string{
				"message": fmt.Sprintf("model %s not found", modelID),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// List all models
	data := make([]interface{}, 0, len(models))
	for _, m := range models {
		data = append(data, formatOpenAIModel(m))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object": "list",
		"data":   data,
	})
}

func formatOpenAIModel(m *router.ModelConfig) map[string]interface{} {
	return map[string]interface{}{
		"id":       m.ModelID,
		"object":   "model",
		"created":  time.Now().Unix(),
		"owned_by": m.Provider,
	}
}

// ---- Generic Passthrough ----

func (s *Server) handlePassthrough(w http.ResponseWriter, r *http.Request) {
	pr, ok := s.router.(ProviderResolver)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "endpoint not supported"})
		return
	}

	// Read body and try to extract model name
	var bodyBytes []byte
	var modelRef string

	if r.Body != nil && r.ContentLength != 0 {
		bodyBytes, _ = io.ReadAll(io.LimitReader(r.Body, 1<<20))
		r.Body.Close()

		var partial struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(bodyBytes, &partial) == nil {
			modelRef = partial.Model
		}
	}

	info, found := pr.ResolveProvider(modelRef)
	if !found {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "no upstream provider available"})
		return
	}

	// Build upstream URL
	targetURL := strings.TrimSuffix(info.BaseURL, "/") + r.URL.Path

	// Create forwarded request
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bodyReader)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create proxy request"})
		return
	}

	// Copy original headers
	for k, vv := range r.Header {
		proxyReq.Header[k] = vv
	}

	// Set auth based on provider type
	switch info.Provider {
	case "anthropic":
		proxyReq.Header.Set("x-api-key", info.APIKey)
		proxyReq.Header.Set("anthropic-version", "2023-06-01")
		proxyReq.Header.Del("Authorization")
	default:
		proxyReq.Header.Set("Authorization", "Bearer "+info.APIKey)
		proxyReq.Header.Del("x-api-key")
	}

	// Forward
	resp, err := s.httpClient.Do(proxyReq)
	if err != nil {
		log.Printf("passthrough error for %s: %v", r.URL.Path, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("upstream request failed: %v", err)})
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream response body back
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}
}

// ---- Chat Completion Endpoint ----

type proxyRequest struct {
	Model       string          `json:"model"`
	Messages    []proxyMessage  `json:"messages"`
	Stream      bool            `json:"stream"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
	System      json.RawMessage `json:"system"` // Anthropic-specific
}

type proxyMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

func (s *Server) handleChatCompletion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	isAnthropic := strings.HasSuffix(r.URL.Path, "/messages")

	// API key authentication
	if s.apiKey != "" {
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" || token == authHeader {
			token = r.Header.Get("x-api-key")
		}
		if token != s.apiKey {
			if isAnthropic {
				writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
					"type":  "error",
					"error": map[string]string{"type": "authentication_error", "message": "invalid or missing API key"},
				})
			} else {
				writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
					"error": map[string]string{"message": "invalid or missing API key", "type": "authentication_error"},
				})
			}
			return
		}
	}

	// Read raw body (10MB limit for tool-heavy requests)
	body, err := provider.ReadBody(r.Body, 10<<20)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	// Parse into raw map to preserve ALL fields (tools, tool_choice, thinking, etc.)
	var reqMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &reqMap); err != nil {
		if isAnthropic {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"type":  "error",
				"error": map[string]string{"type": "invalid_request_error", "message": "invalid JSON"},
			})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		}
		return
	}

	// Extract fields needed for routing only
	var modelField string
	if raw, ok := reqMap["model"]; ok {
		json.Unmarshal(raw, &modelField)
	}

	var msgs []proxyMessage
	if raw, ok := reqMap["messages"]; ok {
		json.Unmarshal(raw, &msgs)
	}

	var systemText string
	if raw, ok := reqMap["system"]; ok {
		systemText = extractSystemText(raw)
	}

	// Build messages for classification
	providerMsgs := make([]provider.Message, 0, len(msgs)+1)
	if isAnthropic && systemText != "" {
		providerMsgs = append(providerMsgs, provider.Message{Role: "system", Content: systemText})
	}
	for _, m := range msgs {
		providerMsgs = append(providerMsgs, provider.Message{Role: m.Role, Content: m.Content})
	}

	// Determine route mode
	mode := s.defaultMode
	switch {
	case modelField == "" || modelField == "auto":
	case modelField == "race":
		mode = router.RouteRace
	default:
		mode = router.RouteManual
	}
	if h := r.Header.Get("X-Router-Mode"); h != "" {
		mode = router.RouteModeFromString(h)
	}

	// Select model via router (raw passthrough — no intermediate serialization)
	type modelSelector interface {
		SelectModel(ctx context.Context, req *router.RouteRequest) (*router.ModelConfig, int64, string)
	}

	selector, ok := s.router.(modelSelector)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "router does not support model selection"})
		return
	}

	modelCfg, complexity, errMsg := selector.SelectModel(r.Context(), &router.RouteRequest{
		Messages: providerMsgs,
		Mode:     mode,
		ModelID:  modelField,
	})
	if modelCfg == nil {
		if isAnthropic {
			writeJSON(w, http.StatusBadGateway, map[string]interface{}{
				"type":  "error",
				"error": map[string]string{"type": "api_error", "message": errMsg},
			})
		} else {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": errMsg})
		}
		return
	}

	w.Header().Set("X-Router-Model", modelCfg.Name)
	w.Header().Set("X-Router-Complexity", fmt.Sprintf("%d", complexity))

	// Replace model in raw body with upstream model ID, preserving all other fields
	reqMap["model"], _ = json.Marshal(modelCfg.ModelID)
	upstreamBody, _ := json.Marshal(reqMap)

	// Build upstream URL based on provider type
	var upstreamPath string
	if modelCfg.Provider == "anthropic" {
		upstreamPath = "/messages"
	} else {
		upstreamPath = "/chat/completions"
	}
	targetURL := strings.TrimSuffix(modelCfg.BaseURL, "/") + upstreamPath

	// Create upstream request
	proxyReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(upstreamBody))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	// Copy all client headers (preserves anthropic-beta, content-type, etc.)
	for k, vv := range r.Header {
		proxyReq.Header[k] = vv
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	// Set auth based on provider type
	switch modelCfg.Provider {
	case "anthropic":
		proxyReq.Header.Set("x-api-key", modelCfg.APIKey)
		if proxyReq.Header.Get("anthropic-version") == "" {
			proxyReq.Header.Set("anthropic-version", "2023-06-01")
		}
		proxyReq.Header.Del("Authorization")
	default:
		proxyReq.Header.Set("Authorization", "Bearer "+modelCfg.APIKey)
		proxyReq.Header.Del("x-api-key")
	}

	// Forward to upstream
	start := time.Now()
	var rs requestStats

	resp, err := s.httpClient.Do(proxyReq)
	if err != nil {
		log.Printf("upstream error for %s: %v", modelCfg.Name, err)
		rs.status = "error"
		rs.errMsg = err.Error()
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("upstream request failed: %v", err)})
		s.logRequest(modelCfg, complexity, mode, &rs, time.Since(start).Milliseconds())
		return
	}
	defer resp.Body.Close()

	// Copy response headers from upstream
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream response body back to client
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}

	// Stats
	if resp.StatusCode >= 400 {
		rs.status = "error"
	} else {
		rs.status = "success"
	}
	s.logRequest(modelCfg, complexity, mode, &rs, time.Since(start).Milliseconds())
}

func (s *Server) logRequest(modelCfg *router.ModelConfig, complexity int64, mode router.RouteMode, rs *requestStats, latencyMs int64) {
	if s.OnRequestLog != nil && rs.status != "" {
		s.OnRequestLog(&RequestLogEntry{
			ModelName:  modelCfg.Name,
			Source:     "proxy",
			Complexity: complexity,
			RouteMode:  mode.String(),
			Status:     rs.status,
			TokensIn:   rs.tokensIn,
			TokensOut:  rs.tokensOut,
			LatencyMs:  latencyMs,
			ErrorMsg:   rs.errMsg,
		})
	}
}

// extractSystemText extracts text from Anthropic's system field.
func extractSystemText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var texts []string
		for _, b := range blocks {
			if b.Type == "text" {
				texts = append(texts, b.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

// ---- OpenAI Response Writers ----

func (s *Server) writeOpenAIStream(w http.ResponseWriter, result *router.RouteResult, rs *requestStats) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, canFlush := w.(http.Flusher)
	completionID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	// Initial role chunk
	writeSSE(w, flusher, canFlush, map[string]interface{}{
		"id":      completionID,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   result.ModelName,
		"choices": []map[string]interface{}{
			{"index": 0, "delta": map[string]string{"role": "assistant"}, "finish_reason": nil},
		},
	})

	var hasSentContent bool
	for chunk := range result.Stream {
		if chunk.Error != nil {
			log.Printf("stream error: %v", chunk.Error)
			rs.status = "error"
			rs.errMsg = chunk.Error.Error()
			writeSSE(w, flusher, canFlush, map[string]interface{}{
				"error": map[string]string{"message": chunk.Error.Error(), "type": "stream_error"},
			})
			fmt.Fprint(w, "data: [DONE]\n\n")
			if canFlush {
				flusher.Flush()
			}
			return
		}

		if chunk.Done {
			doneData := map[string]interface{}{
				"id":      completionID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   chunk.Model,
				"choices": []map[string]interface{}{
					{"index": 0, "delta": map[string]interface{}{}, "finish_reason": "stop"},
				},
			}
			if chunk.Usage != nil {
				doneData["usage"] = map[string]interface{}{
					"prompt_tokens":     chunk.Usage.InputTokens,
					"completion_tokens": chunk.Usage.OutputTokens,
					"total_tokens":      chunk.Usage.InputTokens + chunk.Usage.OutputTokens,
				}
				rs.tokensIn = chunk.Usage.InputTokens
				rs.tokensOut = chunk.Usage.OutputTokens
			}
			writeSSE(w, flusher, canFlush, doneData)
			hasSentContent = true
			rs.status = "success"
			break
		}

		if chunk.Content != "" {
			writeSSE(w, flusher, canFlush, map[string]interface{}{
				"id":      completionID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   chunk.Model,
				"choices": []map[string]interface{}{
					{"index": 0, "delta": map[string]string{"content": chunk.Content}, "finish_reason": nil},
				},
			})
			hasSentContent = true
		}
	}

	if !hasSentContent {
		log.Printf("warning: no content from upstream model %s", result.ModelName)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
	}
}

func (s *Server) writeOpenAINonStream(w http.ResponseWriter, result *router.RouteResult, rs *requestStats) {
	w.Header().Set("Content-Type", "application/json")

	var content string
	var usage *provider.Usage
	for chunk := range result.Stream {
		if chunk.Error != nil {
			rs.status = "error"
			rs.errMsg = chunk.Error.Error()
			writeJSON(w, http.StatusBadGateway, map[string]interface{}{
				"error": map[string]string{"message": chunk.Error.Error(), "type": "upstream_error"},
			})
			return
		}
		content += chunk.Content
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.Done {
			break
		}
	}

	rs.status = "success"
	if usage != nil {
		rs.tokensIn = usage.InputTokens
		rs.tokensOut = usage.OutputTokens
	}

	resp := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   result.ModelName,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]interface{}{"role": "assistant", "content": content},
				"finish_reason": "stop",
			},
		},
	}
	if usage != nil {
		resp["usage"] = map[string]interface{}{
			"prompt_tokens":     usage.InputTokens,
			"completion_tokens": usage.OutputTokens,
			"total_tokens":      usage.InputTokens + usage.OutputTokens,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- Anthropic Response Writers ----

func (s *Server) writeAnthropicStream(w http.ResponseWriter, result *router.RouteResult, rs *requestStats) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, canFlush := w.(http.Flusher)
	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())

	// message_start
	writeAnthropicSSE(w, flusher, canFlush, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         result.ModelName,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]int{"input_tokens": 0, "output_tokens": 0},
		},
	})

	// content_block_start
	writeAnthropicSSE(w, flusher, canFlush, "content_block_start", map[string]interface{}{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]string{
			"type": "text",
			"text": "",
		},
	})

	var outputTokens int

	for chunk := range result.Stream {
		if chunk.Error != nil {
			log.Printf("stream error (anthropic): %v", chunk.Error)
			rs.status = "error"
			rs.errMsg = chunk.Error.Error()
			writeAnthropicSSE(w, flusher, canFlush, "error", map[string]interface{}{
				"type":  "error",
				"error": map[string]string{"type": "stream_error", "message": chunk.Error.Error()},
			})
			return
		}

		if chunk.Usage != nil {
			outputTokens = chunk.Usage.OutputTokens
			rs.tokensIn = chunk.Usage.InputTokens
			rs.tokensOut = chunk.Usage.OutputTokens
		}

		if chunk.Content != "" {
			writeAnthropicSSE(w, flusher, canFlush, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]string{"type": "text_delta", "text": chunk.Content},
			})
		}

		if chunk.Done {
			break
		}
	}

	// content_block_stop
	rs.status = "success"
	writeAnthropicSSE(w, flusher, canFlush, "content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	})

	// message_delta
	writeAnthropicSSE(w, flusher, canFlush, "message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
		"usage": map[string]int{"output_tokens": outputTokens},
	})

	// message_stop
	writeAnthropicSSE(w, flusher, canFlush, "message_stop", map[string]interface{}{
		"type": "message_stop",
	})
}

func (s *Server) writeAnthropicNonStream(w http.ResponseWriter, result *router.RouteResult, rs *requestStats) {
	w.Header().Set("Content-Type", "application/json")

	var content string
	var usage *provider.Usage
	for chunk := range result.Stream {
		if chunk.Error != nil {
			rs.status = "error"
			rs.errMsg = chunk.Error.Error()
			writeJSON(w, http.StatusBadGateway, map[string]interface{}{
				"type":  "error",
				"error": map[string]string{"type": "upstream_error", "message": chunk.Error.Error()},
			})
			return
		}
		content += chunk.Content
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.Done {
			break
		}
	}

	rs.status = "success"
	if usage != nil {
		rs.tokensIn = usage.InputTokens
		rs.tokensOut = usage.OutputTokens
	}

	resp := map[string]interface{}{
		"id":            fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		"type":          "message",
		"role":          "assistant",
		"model":         result.ModelName,
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"content": []map[string]string{
			{"type": "text", "text": content},
		},
	}
	if usage != nil {
		resp["usage"] = map[string]interface{}{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
		}
	} else {
		resp["usage"] = map[string]interface{}{
			"input_tokens":  0,
			"output_tokens": 0,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- SSE Helpers ----

func writeSSE(w http.ResponseWriter, flusher http.Flusher, canFlush bool, data interface{}) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
	if canFlush {
		flusher.Flush()
	}
}

func writeAnthropicSSE(w http.ResponseWriter, flusher http.Flusher, canFlush bool, event string, data interface{}) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	if canFlush {
		flusher.Flush()
	}
}

// ---- Helpers ----

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	activeModels := 0
	if ml, ok := s.router.(ModelLister); ok {
		activeModels = len(ml.ListActiveModels())
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "ok",
		"service":       "multi-model-router",
		"active_models": activeModels,
		"endpoints": []string{
			"POST /v1/chat/completions",
			"POST /v1/messages",
			"GET  /v1/models",
			"GET  /health",
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
