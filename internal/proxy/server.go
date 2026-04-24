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

type SelectionExplainer interface {
	ExplainSelection(ctx context.Context, req *router.RouteRequest) *router.SelectionResult
}

// RequestLogEntry is the data passed to the OnRequestLog callback after each request.
type RequestLogEntry struct {
	ModelName       string
	Source          string
	Complexity      int64
	RouteMode       string
	Status          string
	TokensIn        int
	TokensOut       int
	LatencyMs       int64
	ErrorMsg        string
	Diagnostics     string
	DiagnosticsJSON string
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
	tokensIn        int
	tokensOut       int
	status          string // "success" or "error"
	errMsg          string
	diagnostics     string
	diagnosticsJSON string
}

// New creates a new proxy Server listening on the given port with a default route mode.
func New(port int, r Router, mode router.RouteMode, apiKey string) *Server {
	return &Server{
		port:        port,
		router:      r,
		defaultMode: mode,
		apiKey:      apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
			Transport: &http.Transport{
				DisableCompression: true,
			},
		},
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

// requestHeadersToSkip are headers that must NOT be copied when forwarding REQUEST headers.
// Go's http client manages these automatically; copying them causes mismatches
// (e.g. wrong Content-Length after body modification → empty upstream response).
var requestHeadersToSkip = map[string]bool{
	"Content-Length":    true,
	"Content-Encoding":  true,
	"Accept-Encoding":   true,
	"Transfer-Encoding": true,
	"Connection":        true,
	"Keep-Alive":        true,
	"Upgrade":           true,
	"Host":              true,
}

// responseHeadersToSkip are headers to skip when copying RESPONSE headers.
// Content-Length is stripped because Go's Transport may decode the upstream
// body (e.g. unchunk) causing the forwarded Content-Length to mismatch the
// actual bytes written. Let Go's ResponseWriter handle framing instead.
// Content-Encoding is kept so clients can correctly decompress responses.
var responseHeadersToSkip = map[string]bool{
	"Content-Length":    true,
	"Transfer-Encoding": true,
	"Connection":        true,
	"Keep-Alive":        true,
	"Upgrade":           true,
}

// copyRequestHeaders copies headers from src to dst, skipping auto-managed request headers.
func copyRequestHeaders(dst http.Header, src http.Header) {
	for k, vv := range src {
		if requestHeadersToSkip[k] {
			continue
		}
		dst[k] = vv
	}
}

// copyResponseHeaders copies headers from src to dst, skipping only hop-by-hop headers.
// Preserves Content-Length and Content-Encoding for correct client-side parsing.
func copyResponseHeaders(dst http.Header, src http.Header) {
	for k, vv := range src {
		if responseHeadersToSkip[k] {
			continue
		}
		dst[k] = vv
	}
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

	bodyBytes = rewritePassthroughModel(bodyBytes, info.ModelID)

	// Build upstream URL
	targetURL := buildUpstreamURL(info.BaseURL, r.URL.Path, r.URL.RawQuery)

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

	// Copy original headers (skip Content-Length/Host to avoid body mismatch)
	copyRequestHeaders(proxyReq.Header, r.Header)

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

	// Copy response headers (skip Content-Length/Encoding to avoid mismatch)
	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// Stream response body back
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	var totalBytes int
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			totalBytes += n
			w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}

	log.Printf("passthrough response: status=%d bytes=%d path=%s", resp.StatusCode, totalBytes, r.URL.Path)
}

func buildUpstreamURL(baseURL, requestPath, rawQuery string) string {
	targetURL := strings.TrimSuffix(baseURL, "/")
	upstreamPath := strings.TrimPrefix(requestPath, "/v1")
	if upstreamPath == "" {
		upstreamPath = "/"
	}
	if !strings.HasPrefix(upstreamPath, "/") {
		upstreamPath = "/" + upstreamPath
	}
	targetURL += upstreamPath
	if rawQuery != "" {
		targetURL += "?" + rawQuery
	}
	return targetURL
}

func rewritePassthroughModel(body []byte, modelID string) []byte {
	if len(body) == 0 || modelID == "" {
		return body
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	if _, ok := payload["model"]; !ok {
		return body
	}

	payload["model"], _ = json.Marshal(modelID)
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return rewritten
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

	// Minimal parse: only extract model and messages for routing
	var modelField string
	var providerMsgs []provider.Message

	var reqMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &reqMap); err != nil {
		// Can't parse JSON at all — try blind passthrough using default route
		mode := s.defaultMode
		if h := r.Header.Get("X-Router-Mode"); h != "" {
			mode = router.RouteModeFromString(h)
		}

		type modelSelector interface {
			SelectModel(ctx context.Context, req *router.RouteRequest) (*router.ModelConfig, int64, string)
		}
		selector, ok := s.router.(modelSelector)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "router does not support model selection"})
			return
		}

		modelCfg, complexity, errMsg := selector.SelectModel(r.Context(), &router.RouteRequest{
			Messages: nil,
			Mode:     mode,
			ModelID:  "",
		})
		if modelCfg == nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": errMsg})
			return
		}

		s.forwardRawRequest(w, r, body, modelCfg, complexity, mode)
		return
	}

	// Extract model
	if raw, ok := reqMap["model"]; ok {
		json.Unmarshal(raw, &modelField)
	}

	// Extract messages for complexity classification
	var msgs []proxyMessage
	if raw, ok := reqMap["messages"]; ok {
		json.Unmarshal(raw, &msgs)
	}
	var systemText string
	if raw, ok := reqMap["system"]; ok {
		systemText = extractSystemText(raw)
	}

	providerMsgs = make([]provider.Message, 0, len(msgs)+1)
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

	type modelSelector interface {
		SelectModel(ctx context.Context, req *router.RouteRequest) (*router.ModelConfig, int64, string)
	}

	selector, ok := s.router.(modelSelector)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "router does not support model selection"})
		return
	}

	routeReq := &router.RouteRequest{
		Messages: providerMsgs,
		Mode:     mode,
		ModelID:  modelField,
	}
	modelCfg, complexity, errMsg := selector.SelectModel(r.Context(), routeReq)
	decisionSummary := ""
	decisionJSON := ""
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

	if explainer, ok := s.router.(SelectionExplainer); ok {
		if selection := explainer.ExplainSelection(r.Context(), routeReq); selection != nil && selection.Diagnostics != nil {
			decisionSummary = selection.Diagnostics.HeaderSummary()
			decisionJSON = selection.Diagnostics.ToJSON()
			w.Header().Set("X-Router-Decision", decisionSummary)
		}
	}

	// Replace model, then sanitize/cross-convert for the target provider
	reqMap["model"], _ = json.Marshal(modelCfg.ModelID)
	isTargetAnthropic := modelCfg.Provider == "anthropic"
	convertToolsForUpstream(reqMap, isAnthropic, isTargetAnthropic)
	convertToolChoiceForUpstream(reqMap, isAnthropic, isTargetAnthropic)
	convertSystemForUpstream(reqMap, isAnthropic, isTargetAnthropic)
	sanitizeForProvider(reqMap, modelCfg.Provider)
	stripThinkingBlocks(reqMap, isTargetAnthropic)
	ensureMaxTokens(reqMap, isTargetAnthropic)
	upstreamBody, _ := json.Marshal(reqMap)

	s.forwardUpstream(w, r, upstreamBody, modelCfg, complexity, mode, decisionSummary, decisionJSON)
}

// forwardRawRequest forwards the raw body bytes without any JSON manipulation.
func (s *Server) forwardRawRequest(w http.ResponseWriter, r *http.Request, body []byte, modelCfg *router.ModelConfig, complexity int64, mode router.RouteMode) {
	s.forwardUpstream(w, r, body, modelCfg, complexity, mode, "", "")
}

// forwardUpstream sends the prepared body to the upstream provider and streams
// the response back to the client byte-for-byte.
func (s *Server) forwardUpstream(w http.ResponseWriter, r *http.Request, body []byte, modelCfg *router.ModelConfig, complexity int64, mode router.RouteMode, decisionSummary, decisionJSON string) {
	w.Header().Set("X-Router-Model", modelCfg.Name)
	w.Header().Set("X-Router-Complexity", fmt.Sprintf("%d", complexity))
	if decisionSummary != "" {
		w.Header().Set("X-Router-Decision", decisionSummary)
	}

	// Build upstream URL based on provider type
	var upstreamPath string
	if modelCfg.Provider == "anthropic" {
		upstreamPath = "/messages"
	} else {
		upstreamPath = "/chat/completions"
	}
	targetURL := strings.TrimSuffix(modelCfg.BaseURL, "/") + upstreamPath

	proxyReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	// Copy client headers as-is
	copyRequestHeaders(proxyReq.Header, r.Header)
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

	start := time.Now()
	rs := requestStats{
		diagnostics:     decisionSummary,
		diagnosticsJSON: decisionJSON,
	}

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

	if resp.StatusCode >= 400 {
		bodyPreview := string(body)
		if len(bodyPreview) > 500 {
			bodyPreview = bodyPreview[:500] + "..."
		}
		log.Printf("upstream %d from %s (model=%s): %s", resp.StatusCode, targetURL, modelCfg.ModelID, bodyPreview)
	}

	// Read first chunk to detect empty responses before committing status code
	buf := make([]byte, 32*1024)
	firstN, firstErr := resp.Body.Read(buf)

	if firstN == 0 && firstErr != nil && resp.StatusCode < 400 {
		// Upstream returned success status but empty body — return 502 so client retries
		log.Printf("upstream returned %d with empty body url=%s", resp.StatusCode, targetURL)
		rs.status = "error"
		rs.errMsg = "upstream returned empty response"
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream returned empty response"})
		s.logRequest(modelCfg, complexity, mode, &rs, time.Since(start).Milliseconds())
		return
	}

	// Copy response headers from upstream (Content-Length stripped to avoid mismatch)
	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// Write first chunk and stream remaining body
	flusher, canFlush := w.(http.Flusher)
	var totalBytes int
	if firstN > 0 {
		totalBytes += firstN
		w.Write(buf[:firstN])
		if canFlush {
			flusher.Flush()
		}
	}

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			totalBytes += n
			w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}

	log.Printf("upstream response: status=%d bytes=%d content-type=%s url=%s",
		resp.StatusCode, totalBytes, resp.Header.Get("Content-Type"), targetURL)

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
			ModelName:       modelCfg.Name,
			Source:          "proxy",
			Complexity:      complexity,
			RouteMode:       mode.String(),
			Status:          rs.status,
			TokensIn:        rs.tokensIn,
			TokensOut:       rs.tokensOut,
			LatencyMs:       latencyMs,
			ErrorMsg:        rs.errMsg,
			Diagnostics:     rs.diagnostics,
			DiagnosticsJSON: rs.diagnosticsJSON,
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

// ---- Tools Format Conversion ----

// convertToolsForUpstream converts the tools array between Anthropic and OpenAI
// formats when the request crosses provider types.
//
//	Anthropic: [{"name":"x","description":"...","input_schema":{...}}]
//	OpenAI:    [{"type":"function","function":{"name":"x","description":"...","parameters":{...}}}]
func convertToolsForUpstream(reqMap map[string]json.RawMessage, isSourceAnthropic, isTargetAnthropic bool) {
	if isSourceAnthropic == isTargetAnthropic {
		return
	}

	toolsRaw, ok := reqMap["tools"]
	if !ok {
		return
	}

	var tools []json.RawMessage
	if err := json.Unmarshal(toolsRaw, &tools); err != nil || len(tools) == 0 {
		return
	}

	var converted []json.RawMessage

	for _, raw := range tools {
		var tool map[string]json.RawMessage
		if json.Unmarshal(raw, &tool) != nil {
			continue
		}

		if isSourceAnthropic {
			// Anthropic → OpenAI: wrap fields under "function"
			fn := make(map[string]json.RawMessage)
			for _, k := range []string{"name", "description"} {
				if v, ok := tool[k]; ok {
					fn[k] = v
				}
			}
			if v, ok := tool["input_schema"]; ok {
				fn["parameters"] = v
			}
			fnJSON, _ := json.Marshal(fn)
			out := map[string]json.RawMessage{
				"type":     json.RawMessage(`"function"`),
				"function": json.RawMessage(fnJSON),
			}
			outJSON, _ := json.Marshal(out)
			converted = append(converted, json.RawMessage(outJSON))
		} else {
			// OpenAI → Anthropic: unwrap "function"
			fnRaw, ok := tool["function"]
			if !ok {
				continue
			}
			var fn map[string]json.RawMessage
			if json.Unmarshal(fnRaw, &fn) != nil {
				continue
			}
			out := make(map[string]json.RawMessage)
			for _, k := range []string{"name", "description"} {
				if v, ok := fn[k]; ok {
					out[k] = v
				}
			}
			if v, ok := fn["parameters"]; ok {
				out["input_schema"] = v
			}
			outJSON, _ := json.Marshal(out)
			converted = append(converted, json.RawMessage(outJSON))
		}
	}

	if len(converted) > 0 {
		arrJSON, _ := json.Marshal(converted)
		reqMap["tools"] = json.RawMessage(arrJSON)
	}
}

// convertToolChoiceForUpstream converts tool_choice between Anthropic and OpenAI formats.
//
//	OpenAI:    "auto" | "none" | "required" | {"type":"function","function":{"name":"x"}}
//	Anthropic: {"type":"auto"} | {"type":"none"} | {"type":"any"} | {"type":"tool","name":"x"}
func convertToolChoiceForUpstream(reqMap map[string]json.RawMessage, isSourceAnthropic, isTargetAnthropic bool) {
	if isSourceAnthropic == isTargetAnthropic {
		return
	}

	tcRaw, ok := reqMap["tool_choice"]
	if !ok {
		return
	}

	if isSourceAnthropic {
		// Anthropic object → OpenAI string or object
		var tc map[string]json.RawMessage
		if json.Unmarshal(tcRaw, &tc) != nil {
			return
		}
		var tcType string
		json.Unmarshal(tc["type"], &tcType)
		switch tcType {
		case "auto":
			reqMap["tool_choice"] = json.RawMessage(`"auto"`)
		case "none":
			reqMap["tool_choice"] = json.RawMessage(`"none"`)
		case "any":
			reqMap["tool_choice"] = json.RawMessage(`"required"`)
		case "tool":
			if name := tc["name"]; name != nil {
				reqMap["tool_choice"] = json.RawMessage(
					`{"type":"function","function":{"name":` + string(name) + `}}`,
				)
			}
		}
	} else {
		// OpenAI string or object → Anthropic object
		var s string
		if json.Unmarshal(tcRaw, &s) == nil {
			switch s {
			case "auto":
				reqMap["tool_choice"] = json.RawMessage(`{"type":"auto"}`)
			case "none":
				reqMap["tool_choice"] = json.RawMessage(`{"type":"none"}`)
			case "required":
				reqMap["tool_choice"] = json.RawMessage(`{"type":"any"}`)
			}
			return
		}
		var tc map[string]json.RawMessage
		if json.Unmarshal(tcRaw, &tc) == nil {
			var tcType string
			json.Unmarshal(tc["type"], &tcType)
			if tcType == "function" {
				if fnRaw := tc["function"]; fnRaw != nil {
					var fn map[string]json.RawMessage
					if json.Unmarshal(fnRaw, &fn) == nil {
						if name := fn["name"]; name != nil {
							reqMap["tool_choice"] = json.RawMessage(
								`{"type":"tool","name":` + string(name) + `}`,
							)
						}
					}
				}
			}
		}
	}
}

// ---- Provider Field Sanitization ----

// openAIFields are fields accepted by OpenAI-compatible chat completion APIs.
var openAIFields = map[string]bool{
	"model": true, "messages": true, "max_tokens": true, "max_completion_tokens": true,
	"temperature": true, "top_p": true, "n": true, "stream": true, "stream_options": true,
	"stop": true, "presence_penalty": true, "frequency_penalty": true,
	"logit_bias": true, "user": true, "response_format": true, "seed": true,
	"tools": true, "tool_choice": true, "parallel_tool_calls": true,
	"logprobs": true, "top_logprobs": true, "reasoning_effort": true,
}

// anthropicFields are fields accepted by the Anthropic messages API.
var anthropicFields = map[string]bool{
	"model": true, "messages": true, "max_tokens": true,
	"temperature": true, "top_p": true, "top_k": true, "stream": true,
	"stop_sequences": true, "system": true, "tools": true,
	"tool_choice": true, "thinking": true, "metadata": true,
}

// sanitizeForProvider removes fields that the target provider doesn't support.
func sanitizeForProvider(reqMap map[string]json.RawMessage, provider string) {
	allowed := openAIFields
	if provider == "anthropic" {
		allowed = anthropicFields
	}
	for k := range reqMap {
		if !allowed[k] {
			delete(reqMap, k)
		}
	}
}

// stripThinkingBlocks removes Anthropic "thinking" content blocks from messages
// when forwarding to providers that don't support them (e.g. OpenAI-compatible APIs).
func stripThinkingBlocks(reqMap map[string]json.RawMessage, isTargetAnthropic bool) {
	if isTargetAnthropic {
		return
	}

	msgsRaw, ok := reqMap["messages"]
	if !ok {
		return
	}

	var msgs []json.RawMessage
	if json.Unmarshal(msgsRaw, &msgs) != nil {
		return
	}

	modified := false
	for i, msgRaw := range msgs {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(msgRaw, &msg) != nil {
			continue
		}

		// Content might be a string (no blocks to strip) or an array of blocks
		var blocks []map[string]json.RawMessage
		if json.Unmarshal(msg.Content, &blocks) != nil {
			continue // string content, nothing to strip
		}

		var filtered []map[string]json.RawMessage
		for _, block := range blocks {
			var blockType string
			json.Unmarshal(block["type"], &blockType)
			if blockType != "thinking" {
				filtered = append(filtered, block)
			}
		}

		if len(filtered) < len(blocks) {
			modified = true
			filteredRaw, _ := json.Marshal(filtered)
			// Rebuild the message with filtered content
			var msgMap map[string]json.RawMessage
			json.Unmarshal(msgRaw, &msgMap)
			msgMap["content"] = filteredRaw
			msgs[i], _ = json.Marshal(msgMap)
		}
	}

	if modified {
		reqMap["messages"], _ = json.Marshal(msgs)
	}
}

// convertSystemForUpstream handles the system field when crossing provider types.
// Anthropic uses a top-level "system" field; OpenAI includes system messages in "messages".
func convertSystemForUpstream(reqMap map[string]json.RawMessage, isSourceAnthropic, isTargetAnthropic bool) {
	if isSourceAnthropic == isTargetAnthropic {
		return
	}

	if isSourceAnthropic && !isTargetAnthropic {
		// Anthropic → OpenAI: move top-level "system" into messages[0] as system role
		sysRaw, ok := reqMap["system"]
		if !ok || len(sysRaw) == 0 {
			return
		}
		sysText := extractSystemText(sysRaw)
		if sysText == "" {
			delete(reqMap, "system")
			return
		}

		// Prepend system message to messages array
		var msgs []json.RawMessage
		if raw, ok := reqMap["messages"]; ok {
			json.Unmarshal(raw, &msgs)
		}
		sysMsg, _ := json.Marshal(map[string]string{"role": "system", "content": sysText})
		msgs = append([]json.RawMessage{sysMsg}, msgs...)
		reqMap["messages"], _ = json.Marshal(msgs)
		delete(reqMap, "system")
	}

	if !isSourceAnthropic && isTargetAnthropic {
		// OpenAI → Anthropic: extract system messages into top-level "system"
		var msgs []map[string]json.RawMessage
		if raw, ok := reqMap["messages"]; ok {
			json.Unmarshal(raw, &msgs)
		}
		var systemTexts []string
		var remaining []map[string]json.RawMessage
		for _, m := range msgs {
			var role string
			json.Unmarshal(m["role"], &role)
			if role == "system" {
				var content string
				json.Unmarshal(m["content"], &content)
				systemTexts = append(systemTexts, content)
			} else {
				remaining = append(remaining, m)
			}
		}
		if len(systemTexts) > 0 {
			sysText := strings.Join(systemTexts, "\n")
			reqMap["system"], _ = json.Marshal(sysText)
			reqMap["messages"], _ = json.Marshal(remaining)
		}
	}
}

// ensureMaxTokens validates and fixes max_tokens for the target provider.
func ensureMaxTokens(reqMap map[string]json.RawMessage, isTargetAnthropic bool) {
	if raw, ok := reqMap["max_tokens"]; ok {
		var mt int
		if json.Unmarshal(raw, &mt) == nil && mt <= 0 {
			delete(reqMap, "max_tokens")
		}
	}
	// max_completion_tokens → max_tokens for non-OpenAI providers
	if !isTargetAnthropic {
		if raw, ok := reqMap["max_completion_tokens"]; ok {
			if _, hasMT := reqMap["max_tokens"]; !hasMT {
				var mt int
				if json.Unmarshal(raw, &mt) == nil && mt > 0 {
					reqMap["max_tokens"] = raw
				}
			}
			delete(reqMap, "max_completion_tokens")
		}
	}
	// Anthropic requires max_tokens > 0
	if isTargetAnthropic {
		if _, ok := reqMap["max_tokens"]; !ok {
			reqMap["max_tokens"], _ = json.Marshal(4096)
		}
	}
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
