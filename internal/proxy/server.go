package proxy

import (
	"bufio"
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
	port          int
	router        Router
	server        *http.Server
	defaultMode   router.RouteMode
	manualModelID string
	apiKey        string
	httpClient    *http.Client
	OnRequestLog  func(entry *RequestLogEntry) // optional stats callback
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
	return NewWithManualModel(port, r, mode, "", apiKey)
}

func NewWithManualModel(port int, r Router, mode router.RouteMode, manualModelID, apiKey string) *Server {
	return &Server{
		port:          port,
		router:        r,
		defaultMode:   mode,
		manualModelID: manualModelID,
		apiKey:        apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Minute,
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
	mux.HandleFunc("/chat/completions", s.handleChatCompletion)
	mux.HandleFunc("/v1/messages", s.handleChatCompletion)
	mux.HandleFunc("/messages", s.handleChatCompletion)

	// Model listing endpoints
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/models/", s.handleModels)
	mux.HandleFunc("/models", s.handleModels)
	mux.HandleFunc("/models/", s.handleModels)

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
		modelID := modelField
		if mode == router.RouteManual && s.manualModelID != "" {
			modelID = s.manualModelID
		}
		if mode == router.RouteManual && modelID == "" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "manual mode requires a selected model"})
			return
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
			ModelID:  modelID,
		})
		if modelCfg == nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": errMsg})
			return
		}

		s.forwardRawRequest(w, r, body, modelCfg, complexity, mode, isAnthropic)
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
	modelID := modelField
	if mode == router.RouteManual && s.manualModelID != "" {
		modelID = s.manualModelID
	}
	if mode == router.RouteManual && modelID == "" {
		if isAnthropic {
			writeJSON(w, http.StatusBadGateway, map[string]interface{}{
				"type":  "error",
				"error": map[string]string{"type": "api_error", "message": "manual mode requires a selected model"},
			})
		} else {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "manual mode requires a selected model"})
		}
		return
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
		ModelID:  modelID,
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
	convertMessagesForUpstream(reqMap, isAnthropic, isTargetAnthropic)
	convertRequestParametersForUpstream(reqMap, isAnthropic, isTargetAnthropic)
	preserveCompletionTokenLimit(reqMap, isTargetAnthropic)
	sanitizeForProvider(reqMap, modelCfg.Provider)
	sanitizeThinkingForProtocol(reqMap, isAnthropic, isTargetAnthropic)
	stripThinkingBlocks(reqMap, isTargetAnthropic)
	sanitizeNullContent(reqMap)
	ensureMaxTokens(reqMap, isTargetAnthropic)
	upstreamBody, _ := json.Marshal(reqMap)

	s.forwardUpstream(w, r, upstreamBody, modelCfg, complexity, mode, decisionSummary, decisionJSON, isAnthropic)
}

// forwardRawRequest forwards the raw body bytes without any JSON manipulation.
func (s *Server) forwardRawRequest(w http.ResponseWriter, r *http.Request, body []byte, modelCfg *router.ModelConfig, complexity int64, mode router.RouteMode, isSourceAnthropic bool) {
	s.forwardUpstream(w, r, body, modelCfg, complexity, mode, "", "", isSourceAnthropic)
}

// forwardUpstream sends the prepared body to the upstream provider and streams
// the response back to the client while extracting token usage.
func (s *Server) forwardUpstream(w http.ResponseWriter, r *http.Request, body []byte, modelCfg *router.ModelConfig, complexity int64, mode router.RouteMode, decisionSummary, decisionJSON string, isSourceAnthropic bool) {
	w.Header().Set("X-Router-Model", modelCfg.Name)
	w.Header().Set("X-Router-Complexity", fmt.Sprintf("%d", complexity))
	if decisionSummary != "" {
		w.Header().Set("X-Router-Decision", decisionSummary)
	}

	isTargetAnthropic := modelCfg.Provider == "anthropic"

	// Build upstream URL based on provider type
	var upstreamPath string
	if isTargetAnthropic {
		upstreamPath = "/messages"
	} else {
		upstreamPath = "/chat/completions"
		body = injectStreamOptions(body)
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

	contentType := resp.Header.Get("Content-Type")
	isStreaming := strings.Contains(contentType, "text/event-stream")

	// Read first chunk to detect empty responses before committing status code
	buf := make([]byte, 32*1024)
	firstN, firstErr := resp.Body.Read(buf)

	if firstN == 0 && firstErr != nil && resp.StatusCode < 400 {
		log.Printf("upstream returned %d with empty body url=%s", resp.StatusCode, targetURL)
		rs.status = "error"
		rs.errMsg = "upstream returned empty response"
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream returned empty response"})
		s.logRequest(modelCfg, complexity, mode, &rs, time.Since(start).Milliseconds())
		return
	}

	flusher, canFlush := w.(http.Flusher)
	var totalBytes int

	if resp.StatusCode < 400 && isSourceAnthropic != isTargetAnthropic {
		if isStreaming {
			stream := parseUpstreamStream(bytes.NewReader(buf[:firstN]), firstErr, resp.Body, isTargetAnthropic, modelCfg.ModelID)
			result := &router.RouteResult{ModelName: modelCfg.Name, Provider: modelCfg.Provider, Status: "success", Stream: stream}
			if isSourceAnthropic {
				s.writeAnthropicStream(w, result, &rs)
			} else {
				s.writeOpenAIStream(w, result, &rs)
			}
			s.logRequest(modelCfg, complexity, mode, &rs, time.Since(start).Milliseconds())
			return
		}

		var responseBody []byte
		if firstN > 0 {
			responseBody = append(responseBody, buf[:firstN]...)
		}
		remaining, _ := io.ReadAll(resp.Body)
		responseBody = append(responseBody, remaining...)
		convertedBody, err := convertNonStreamingResponse(responseBody, isTargetAnthropic, modelCfg.Name)
		if err != nil {
			log.Printf("failed to convert upstream response from %s for %s: %v", modelCfg.Provider, r.URL.Path, err)
			rs.status = "error"
			rs.errMsg = err.Error()
			writeJSON(w, http.StatusBadGateway, protocolErrorBody(isSourceAnthropic, err.Error()))
			s.logRequest(modelCfg, complexity, mode, &rs, time.Since(start).Milliseconds())
			return
		}
		s.extractUsageFromJSON(convertedBody, isSourceAnthropic, &rs)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(convertedBody)
		if canFlush {
			flusher.Flush()
		}
		totalBytes = len(convertedBody)
	} else {
		// Copy response headers from upstream (Content-Length stripped to avoid mismatch)
		copyResponseHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)

		if isStreaming {
			// For streaming responses, parse SSE lines for token usage while forwarding
			if isTargetAnthropic {
				totalBytes = s.streamAndTrackAnthropic(w, flusher, canFlush, buf, firstN, firstErr, resp.Body, &rs)
			} else {
				totalBytes = s.streamAndTrackOpenAI(w, flusher, canFlush, buf, firstN, firstErr, resp.Body, &rs)
			}
		} else {
			// For non-streaming responses, read full body and extract usage
			var responseBody []byte
			if firstN > 0 {
				responseBody = append(responseBody, buf[:firstN]...)
			}
			remaining, _ := io.ReadAll(resp.Body)
			responseBody = append(responseBody, remaining...)
			totalBytes = len(responseBody)

			// Extract usage from non-streaming response
			s.extractUsageFromJSON(responseBody, isTargetAnthropic, &rs)

			w.Write(responseBody)
			if canFlush {
				flusher.Flush()
			}
		}
	}

	log.Printf("upstream response: status=%d bytes=%d tokens_in=%d tokens_out=%d url=%s",
		resp.StatusCode, totalBytes, rs.tokensIn, rs.tokensOut, targetURL)

	if resp.StatusCode >= 400 {
		rs.status = "error"
	} else {
		rs.status = "success"
	}
	s.logRequest(modelCfg, complexity, mode, &rs, time.Since(start).Milliseconds())
}

// injectStreamOptions adds stream_options with include_usage to OpenAI requests.
func injectStreamOptions(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	var stream bool
	if raw, ok := m["stream"]; !ok || json.Unmarshal(raw, &stream) != nil || !stream {
		return body
	}
	if _, ok := m["stream_options"]; ok {
		return body // already set
	}
	m["stream_options"], _ = json.Marshal(map[string]bool{"include_usage": true})
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

// streamAndTrackOpenAI reads an OpenAI SSE stream, forwards it to the client,
// and extracts token usage from chunks that contain it.
func (s *Server) streamAndTrackOpenAI(w http.ResponseWriter, flusher http.Flusher, canFlush bool, buf []byte, firstN int, firstErr error, body io.Reader, rs *requestStats) int {
	totalBytes := 0
	reader := io.MultiReader(
		bytes.NewReader(buf[:firstN]),
		body,
	)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		totalBytes += len(line) + 1 // +1 for newline
		fmt.Fprintln(w, line)
		if canFlush {
			flusher.Flush()
		}

		// Parse SSE data line for usage
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				continue
			}
			var chunk struct {
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err == nil && chunk.Usage != nil {
				rs.tokensIn = chunk.Usage.PromptTokens
				rs.tokensOut = chunk.Usage.CompletionTokens
			}
		}
	}

	_ = firstErr // scanner already consumed via MultiReader
	return totalBytes
}

// streamAndTrackAnthropic reads an Anthropic SSE stream, forwards it to the client,
// and extracts input/output tokens from message_start and message_delta events.
func (s *Server) streamAndTrackAnthropic(w http.ResponseWriter, flusher http.Flusher, canFlush bool, buf []byte, firstN int, firstErr error, body io.Reader, rs *requestStats) int {
	totalBytes := 0
	reader := io.MultiReader(
		bytes.NewReader(buf[:firstN]),
		body,
	)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		totalBytes += len(line) + 1
		fmt.Fprintln(w, line)
		if canFlush {
			flusher.Flush()
		}

		// Parse SSE data line for Anthropic events
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var event struct {
				Type    string `json:"type"`
				Message *struct {
					Usage *struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
				Usage *struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				switch event.Type {
				case "message_start":
					if event.Message != nil && event.Message.Usage != nil {
						rs.tokensIn = event.Message.Usage.InputTokens
					}
				case "message_delta":
					if event.Usage != nil {
						rs.tokensOut = event.Usage.OutputTokens
					}
				}
			}
		}
	}

	_ = firstErr
	return totalBytes
}

func parseUpstreamStream(first io.Reader, firstErr error, body io.ReadCloser, isAnthropic bool, model string) <-chan provider.StreamChunk {
	out := make(chan provider.StreamChunk, 64)
	go func() {
		defer close(out)
		defer body.Close()

		reader := io.MultiReader(first, body)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var inputTokens int
		var contentReceived bool
		var blockType string
		var thinking strings.Builder
		var signature string
		var preserveContentBlocks bool

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if !isAnthropic && data == "[DONE]" {
				out <- provider.StreamChunk{Done: true, Model: model}
				return
			}

			if isAnthropic {
				var event struct {
					Type    string `json:"type"`
					Index   int    `json:"index"`
					Message *struct {
						Model string `json:"model"`
						Usage *struct {
							InputTokens int `json:"input_tokens"`
						} `json:"usage"`
					} `json:"message"`
					ContentBlock map[string]interface{} `json:"content_block"`
					Delta        *struct {
						Type      string `json:"type"`
						Text      string `json:"text"`
						Thinking  string `json:"thinking"`
						Signature string `json:"signature"`
					} `json:"delta"`
					Usage *struct {
						OutputTokens int `json:"output_tokens"`
					} `json:"usage"`
				}
				if json.Unmarshal([]byte(data), &event) != nil {
					continue
				}
				switch event.Type {
				case "message_start":
					if event.Message != nil {
						if event.Message.Model != "" {
							model = event.Message.Model
						}
						if event.Message.Usage != nil {
							inputTokens = event.Message.Usage.InputTokens
						}
					}
				case "content_block_start":
					blockType = ""
					thinking.Reset()
					signature = ""
					if event.ContentBlock != nil {
						if t, ok := event.ContentBlock["type"].(string); ok {
							blockType = t
						}
						if blockType == "redacted_thinking" {
							preserveContentBlocks = true
							out <- provider.StreamChunk{ContentBlock: event.ContentBlock, Model: model}
							contentReceived = true
							blockType = ""
						}
						if blockType == "thinking" {
							preserveContentBlocks = true
						}
					}
				case "content_block_delta":
					if event.Delta != nil && event.Delta.Text != "" {
						if preserveContentBlocks {
							out <- provider.StreamChunk{ContentBlock: map[string]interface{}{"type": "text", "text": event.Delta.Text}, Model: model}
						} else {
							out <- provider.StreamChunk{Content: event.Delta.Text, Model: model}
						}
						contentReceived = true
					}
					if event.Delta != nil && event.Delta.Thinking != "" {
						thinking.WriteString(event.Delta.Thinking)
					}
					if event.Delta != nil && event.Delta.Signature != "" {
						signature = event.Delta.Signature
					}
				case "content_block_stop":
					if blockType == "thinking" && thinking.Len() > 0 {
						block := map[string]interface{}{
							"type":     "thinking",
							"thinking": thinking.String(),
						}
						if signature != "" {
							block["signature"] = signature
						}
						out <- provider.StreamChunk{ContentBlock: block, Model: model}
						contentReceived = true
					}
					blockType = ""
					thinking.Reset()
					signature = ""
				case "message_delta":
					usage := &provider.Usage{InputTokens: inputTokens}
					if event.Usage != nil {
						usage.OutputTokens = event.Usage.OutputTokens
					}
					out <- provider.StreamChunk{Usage: usage, Model: model}
				case "message_stop":
					out <- provider.StreamChunk{Done: true, Model: model}
					return
				}
				continue
			}

			var resp struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
				Model string `json:"model"`
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(data), &resp) != nil {
				continue
			}
			if resp.Model != "" {
				model = resp.Model
			}
			if resp.Usage != nil {
				out <- provider.StreamChunk{Usage: &provider.Usage{InputTokens: resp.Usage.PromptTokens, OutputTokens: resp.Usage.CompletionTokens}, Model: model}
			}
			for _, choice := range resp.Choices {
				if choice.Delta.Content != "" {
					out <- provider.StreamChunk{Content: choice.Delta.Content, Model: model}
					contentReceived = true
				}
				if choice.FinishReason != nil {
					out <- provider.StreamChunk{Done: true, Model: model}
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			out <- provider.StreamChunk{Error: fmt.Errorf("stream read error: %w", err), Done: true, Model: model}
			return
		}
		if firstErr != nil && firstErr != io.EOF {
			out <- provider.StreamChunk{Error: fmt.Errorf("stream read error: %w", firstErr), Done: true, Model: model}
			return
		}
		if !contentReceived {
			out <- provider.StreamChunk{Error: fmt.Errorf("stream ended without any content"), Done: true, Model: model}
			return
		}
		out <- provider.StreamChunk{Done: true, Model: model}
	}()
	return out
}

func convertNonStreamingResponse(body []byte, isUpstreamAnthropic bool, modelName string) ([]byte, error) {
	if isUpstreamAnthropic {
		message, usage, finishReason, err := parseAnthropicNonStreamingForOpenAI(body)
		if err != nil {
			return nil, err
		}
		resp := map[string]interface{}{
			"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   modelName,
			"choices": []map[string]interface{}{{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			}},
		}
		if usage != nil {
			resp["usage"] = map[string]int{
				"prompt_tokens":     usage.InputTokens,
				"completion_tokens": usage.OutputTokens,
				"total_tokens":      usage.InputTokens + usage.OutputTokens,
			}
		}
		return json.Marshal(resp)
	}

	content, usage, stopReason, err := parseOpenAINonStreamingForAnthropic(body)
	if err != nil {
		return nil, err
	}
	resp := map[string]interface{}{
		"id":            fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		"type":          "message",
		"role":          "assistant",
		"model":         modelName,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"content":       content,
		"usage":         map[string]int{"input_tokens": 0, "output_tokens": 0},
	}
	if usage != nil {
		resp["usage"] = map[string]int{"input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens}
	}
	return json.Marshal(resp)
}

func parseAnthropicNonStreamingForOpenAI(body []byte) (map[string]interface{}, *provider.Usage, string, error) {
	var resp struct {
		Content    []map[string]interface{} `json:"content"`
		StopReason string                   `json:"stop_reason"`
		Usage      *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil, "", fmt.Errorf("decode Anthropic response: %w", err)
	}

	var textParts []string
	var toolCalls []map[string]interface{}
	for _, block := range resp.Content {
		blockType, _ := block["type"].(string)
		switch blockType {
		case "text":
			if text, _ := block["text"].(string); text != "" {
				textParts = append(textParts, text)
			}
		case "thinking", "redacted_thinking":
			// OpenAI-compatible message.content must remain a string; thinking
			// blocks are intentionally omitted from this compatibility response.
		case "tool_use":
			if call, ok := anthropicToolUseBlockToOpenAIInterface(block); ok {
				toolCalls = append(toolCalls, call)
			}
		}
	}

	message := map[string]interface{}{"role": "assistant"}
	// OpenAI Chat Completions requires message.content to be a string.
	// Do not preserve Anthropic thinking/redacted_thinking blocks in the
	// OpenAI-compatible content field; clients may stringify object parts as
	// "[object Object]". Text blocks are the only portable content here.
	message["content"] = strings.Join(textParts, "")
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
		if message["content"] == nil {
			message["content"] = ""
		}
	}

	usage := (*provider.Usage)(nil)
	if resp.Usage != nil {
		usage = &provider.Usage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}
	}
	return message, usage, anthropicStopReasonToOpenAI(resp.StopReason), nil
}

func anthropicToolUseBlockToOpenAIInterface(block map[string]interface{}) (map[string]interface{}, bool) {
	id, _ := block["id"].(string)
	name, _ := block["name"].(string)
	if id == "" || name == "" {
		return nil, false
	}
	args := "{}"
	if input, ok := block["input"]; ok && input != nil {
		if b, err := json.Marshal(input); err == nil {
			args = string(b)
		}
	}
	return map[string]interface{}{
		"id":   id,
		"type": "function",
		"function": map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	}, true
}

func parseOpenAINonStreamingForAnthropic(body []byte) ([]map[string]interface{}, *provider.Usage, string, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content   interface{}              `json:"content"`
				ToolCalls []map[string]interface{} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil, "", fmt.Errorf("decode OpenAI response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, nil, "", fmt.Errorf("OpenAI response missing choices")
	}
	choice := resp.Choices[0]
	blocks := openAIContentToAnthropicBlocks(choice.Message.Content)
	for _, call := range choice.Message.ToolCalls {
		if block, ok := openAIToolCallToAnthropicInterface(call); ok {
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		blocks = []map[string]interface{}{{"type": "text", "text": ""}}
	}
	usage := (*provider.Usage)(nil)
	if resp.Usage != nil {
		usage = &provider.Usage{InputTokens: resp.Usage.PromptTokens, OutputTokens: resp.Usage.CompletionTokens}
	}
	return blocks, usage, openAIFinishReasonToAnthropic(choice.FinishReason), nil
}

func openAIContentToAnthropicBlocks(content interface{}) []map[string]interface{} {
	switch v := content.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []map[string]interface{}{{"type": "text", "text": v}}
	case []interface{}:
		blocks := make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			t, _ := m["type"].(string)
			switch t {
			case "text", "thinking", "redacted_thinking", "tool_use", "tool_result", "image", "document":
				blocks = append(blocks, m)
			case "image_url", "input_image":
				if block, ok := openAIImageInterfaceToAnthropic(m); ok {
					blocks = append(blocks, block)
				}
			}
		}
		return blocks
	default:
		return nil
	}
}

func openAIImageInterfaceToAnthropic(m map[string]interface{}) (map[string]interface{}, bool) {
	var url string
	if imageURL, ok := m["image_url"].(map[string]interface{}); ok {
		url, _ = imageURL["url"].(string)
	}
	if url == "" {
		url, _ = m["image"].(string)
	}
	if url == "" {
		return nil, false
	}
	source := map[string]interface{}{}
	if mediaType, data, ok := parseDataURL(url); ok {
		source["type"] = "base64"
		source["media_type"] = mediaType
		source["data"] = data
	} else {
		source["type"] = "url"
		source["url"] = url
	}
	return map[string]interface{}{"type": "image", "source": source}, true
}

func openAIToolCallToAnthropicInterface(call map[string]interface{}) (map[string]interface{}, bool) {
	id, _ := call["id"].(string)
	fn, _ := call["function"].(map[string]interface{})
	name, _ := fn["name"].(string)
	if id == "" || name == "" {
		return nil, false
	}
	input := map[string]interface{}{}
	if args, _ := fn["arguments"].(string); args != "" {
		_ = json.Unmarshal([]byte(args), &input)
	}
	return map[string]interface{}{"type": "tool_use", "id": id, "name": name, "input": input}, true
}

func anthropicStopReasonToOpenAI(reason string) string {
	switch reason {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}

func openAIFinishReasonToAnthropic(reason string) string {
	switch reason {
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	default:
		return "end_turn"
	}
}

func parseOpenAINonStreaming(body []byte) (string, *provider.Usage, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content interface{} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", nil, fmt.Errorf("decode OpenAI response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", nil, fmt.Errorf("OpenAI response missing choices")
	}
	content := textFromContent(resp.Choices[0].Message.Content)
	usage := (*provider.Usage)(nil)
	if resp.Usage != nil {
		usage = &provider.Usage{InputTokens: resp.Usage.PromptTokens, OutputTokens: resp.Usage.CompletionTokens}
	}
	return content, usage, nil
}

func parseAnthropicNonStreaming(body []byte) (interface{}, *provider.Usage, error) {
	var resp struct {
		Content []map[string]interface{} `json:"content"`
		Usage   *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", nil, fmt.Errorf("decode Anthropic response: %w", err)
	}
	var parts []string
	preserveBlocks := false
	for _, block := range resp.Content {
		blockType, _ := block["type"].(string)
		switch blockType {
		case "text":
			if text, _ := block["text"].(string); text != "" {
				parts = append(parts, text)
			}
		case "thinking", "redacted_thinking":
			preserveBlocks = true
		}
	}
	usage := (*provider.Usage)(nil)
	if resp.Usage != nil {
		usage = &provider.Usage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}
	}
	if preserveBlocks {
		return resp.Content, usage, nil
	}
	return strings.Join(parts, ""), usage, nil
}

func textFromContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, part := range v {
			m, ok := part.(map[string]interface{})
			if !ok {
				continue
			}
			if m["type"] == "text" {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func textFromContentBlock(block map[string]interface{}) string {
	if block == nil {
		return ""
	}
	blockType, _ := block["type"].(string)
	switch blockType {
	case "text":
		if text, ok := block["text"].(string); ok {
			return text
		}
	}
	return ""
}

func protocolErrorBody(isAnthropic bool, message string) map[string]interface{} {
	if isAnthropic {
		return map[string]interface{}{"type": "error", "error": map[string]string{"type": "api_error", "message": message}}
	}
	return map[string]interface{}{"error": map[string]string{"type": "api_error", "message": message}}
}

// extractUsageFromJSON parses usage from a non-streaming response body.
func (s *Server) extractUsageFromJSON(body []byte, isAnthropic bool, rs *requestStats) {
	if isAnthropic {
		var resp struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(body, &resp) == nil {
			rs.tokensIn = resp.Usage.InputTokens
			rs.tokensOut = resp.Usage.OutputTokens
		}
	} else {
		var resp struct {
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(body, &resp) == nil {
			rs.tokensIn = resp.Usage.PromptTokens
			rs.tokensOut = resp.Usage.CompletionTokens
		}
	}
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
		if chunk.ContentBlock != nil {
			if text := textFromContentBlock(chunk.ContentBlock); text != "" {
				writeSSE(w, flusher, canFlush, map[string]interface{}{
					"id":      completionID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   chunk.Model,
					"choices": []map[string]interface{}{
						{"index": 0, "delta": map[string]string{"content": text}, "finish_reason": nil},
					},
				})
				hasSentContent = true
			}
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
		if chunk.ContentBlock != nil {
			content += textFromContentBlock(chunk.ContentBlock)
		}
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
	messageContent := content

	resp := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   result.ModelName,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]interface{}{"role": "assistant", "content": messageContent},
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
	if !ok && !isSourceAnthropic && isTargetAnthropic {
		toolsRaw, ok = reqMap["functions"]
	}
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
				fnJSON, _ := json.Marshal(tool)
				fnRaw = fnJSON
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
	if !ok && !isSourceAnthropic && isTargetAnthropic {
		tcRaw, ok = reqMap["function_call"]
	}
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
			} else if name := tc["name"]; name != nil {
				reqMap["tool_choice"] = json.RawMessage(
					`{"type":"tool","name":` + string(name) + `}`,
				)
			}
		}
	}
}

func convertRequestParametersForUpstream(reqMap map[string]json.RawMessage, isSourceAnthropic, isTargetAnthropic bool) {
	if isSourceAnthropic == isTargetAnthropic {
		return
	}

	if isTargetAnthropic {
		if raw, ok := reqMap["stop"]; ok {
			reqMap["stop_sequences"] = raw
			delete(reqMap, "stop")
		}
		if raw, ok := reqMap["user"]; ok {
			mergeMetadataUserID(reqMap, raw)
			delete(reqMap, "user")
		}
		return
	}

	if raw, ok := reqMap["stop_sequences"]; ok {
		reqMap["stop"] = raw
		delete(reqMap, "stop_sequences")
	}
	if raw, ok := reqMap["metadata"]; ok {
		if userID := metadataUserID(raw); userID != nil {
			reqMap["user"] = userID
		}
	}
}

func mergeMetadataUserID(reqMap map[string]json.RawMessage, userRaw json.RawMessage) {
	if len(userRaw) == 0 || string(userRaw) == "null" {
		return
	}
	metadata := map[string]json.RawMessage{}
	if raw, ok := reqMap["metadata"]; ok {
		_ = json.Unmarshal(raw, &metadata)
	}
	if _, exists := metadata["user_id"]; !exists {
		metadata["user_id"] = userRaw
		reqMap["metadata"], _ = json.Marshal(metadata)
	}
}

func metadataUserID(raw json.RawMessage) json.RawMessage {
	var metadata map[string]json.RawMessage
	if json.Unmarshal(raw, &metadata) != nil {
		return nil
	}
	if userID, ok := metadata["user_id"]; ok {
		return userID
	}
	return nil
}

func preserveCompletionTokenLimit(reqMap map[string]json.RawMessage, isTargetAnthropic bool) {
	if !isTargetAnthropic {
		return
	}
	if _, hasMaxTokens := reqMap["max_tokens"]; hasMaxTokens {
		return
	}
	raw, ok := reqMap["max_completion_tokens"]
	if !ok {
		return
	}
	var mt int
	if json.Unmarshal(raw, &mt) == nil && mt > 0 {
		reqMap["max_tokens"] = raw
	}
}

func convertMessagesForUpstream(reqMap map[string]json.RawMessage, isSourceAnthropic, isTargetAnthropic bool) {
	msgsRaw, ok := reqMap["messages"]
	if !ok {
		return
	}

	var msgs []map[string]json.RawMessage
	if json.Unmarshal(msgsRaw, &msgs) != nil {
		return
	}

	modified := false
	converted := make([]map[string]json.RawMessage, 0, len(msgs))
	for _, msg := range msgs {
		var role string
		json.Unmarshal(msg["role"], &role)
		switch {
		case isTargetAnthropic:
			if normalizeOpenAIContentForAnthropic(msg) {
				modified = true
			}
			if role == "developer" {
				msg["role"] = json.RawMessage(`"user"`)
				modified = true
			}
		case isTargetAnthropic && role == "assistant":
			modified = convertOpenAIToolCallsForAnthropic(msg) || modified
		case isTargetAnthropic && role == "tool":
			msg["role"] = json.RawMessage(`"user"`)
			msg["content"] = anthropicToolResultContent(msg)
			delete(msg, "tool_call_id")
			delete(msg, "name")
			modified = true
		case !isTargetAnthropic:
			openAIMsgs, changed := convertAnthropicMessageForOpenAI(msg)
			if changed {
				modified = true
			}
			converted = append(converted, openAIMsgs...)
			continue
		case role != "system" && role != "user" && role != "assistant" && role != "tool":
			msg["role"] = json.RawMessage(`"user"`)
			modified = true
		}

		if isTargetAnthropic && role == "assistant" {
			modified = convertOpenAIToolCallsForAnthropic(msg) || modified
		}
		if isTargetAnthropic && role == "tool" {
			msg["role"] = json.RawMessage(`"user"`)
			msg["content"] = anthropicToolResultContent(msg)
			delete(msg, "tool_call_id")
			delete(msg, "name")
			modified = true
		}
		converted = append(converted, msg)
	}

	if modified {
		reqMap["messages"], _ = json.Marshal(converted)
	}
}

func convertOpenAIToolCallsForAnthropic(msg map[string]json.RawMessage) bool {
	toolCallsRaw, ok := msg["tool_calls"]
	if !ok || len(toolCallsRaw) == 0 {
		return false
	}

	var toolCalls []map[string]json.RawMessage
	if json.Unmarshal(toolCallsRaw, &toolCalls) != nil || len(toolCalls) == 0 {
		return false
	}

	blocks := contentBlocksFromRaw(msg["content"])
	if len(blocks) == 0 {
		if contentText := textFromRawMessage(msg["content"]); contentText != "" {
			textBlock := map[string]json.RawMessage{"type": json.RawMessage(`"text"`)}
			textBlock["text"], _ = json.Marshal(contentText)
			blocks = append(blocks, textBlock)
		}
	}

	for _, call := range toolCalls {
		fnRaw := call["function"]
		if len(fnRaw) == 0 {
			continue
		}
		var fn map[string]json.RawMessage
		if json.Unmarshal(fnRaw, &fn) != nil {
			continue
		}
		block := map[string]json.RawMessage{"type": json.RawMessage(`"tool_use"`)}
		if id := call["id"]; len(id) > 0 {
			block["id"] = id
		}
		if name := fn["name"]; len(name) > 0 {
			block["name"] = name
		}
		block["input"] = json.RawMessage(`{}`)
		if args := fn["arguments"]; len(args) > 0 {
			var argsText string
			if json.Unmarshal(args, &argsText) == nil && argsText != "" {
				var input json.RawMessage
				if json.Unmarshal([]byte(argsText), &input) == nil {
					block["input"] = input
				}
			} else {
				block["input"] = args
			}
		}
		blocks = append(blocks, block)
	}

	if len(blocks) == 0 {
		return false
	}
	msg["content"], _ = json.Marshal(blocks)
	delete(msg, "tool_calls")
	return true
}

func textFromRawMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return ""
}

func contentBlocksFromRaw(raw json.RawMessage) []map[string]json.RawMessage {
	var blocks []map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	return blocks
}

func normalizeOpenAIContentForAnthropic(msg map[string]json.RawMessage) bool {
	raw := msg["content"]
	blocks := contentBlocksFromRaw(raw)
	if len(blocks) == 0 {
		return false
	}

	modified := false
	converted := make([]map[string]json.RawMessage, 0, len(blocks))
	for _, block := range blocks {
		var blockType string
		_ = json.Unmarshal(block["type"], &blockType)
		switch blockType {
		case "image_url", "input_image":
			if imageBlock, ok := openAIImageBlockToAnthropic(block); ok {
				converted = append(converted, imageBlock)
				modified = true
			} else {
				converted = append(converted, block)
			}
		default:
			converted = append(converted, block)
		}
	}
	if modified {
		msg["content"], _ = json.Marshal(converted)
	}
	return modified
}

func openAIImageBlockToAnthropic(block map[string]json.RawMessage) (map[string]json.RawMessage, bool) {
	url := rawString(block["image_url"])
	if url == "" {
		var imageURL map[string]json.RawMessage
		if json.Unmarshal(block["image_url"], &imageURL) == nil {
			url = rawString(imageURL["url"])
		}
	}
	if url == "" {
		url = rawString(block["image"])
	}
	if url == "" {
		return nil, false
	}

	source := map[string]json.RawMessage{}
	if mediaType, data, ok := parseDataURL(url); ok {
		source["type"] = json.RawMessage(`"base64"`)
		source["media_type"], _ = json.Marshal(mediaType)
		source["data"], _ = json.Marshal(data)
	} else {
		source["type"] = json.RawMessage(`"url"`)
		source["url"], _ = json.Marshal(url)
	}
	sourceJSON, _ := json.Marshal(source)
	return map[string]json.RawMessage{
		"type":   json.RawMessage(`"image"`),
		"source": sourceJSON,
	}, true
}

func parseDataURL(url string) (string, string, bool) {
	if !strings.HasPrefix(url, "data:") {
		return "", "", false
	}
	header, data, ok := strings.Cut(strings.TrimPrefix(url, "data:"), ",")
	if !ok || !strings.HasSuffix(header, ";base64") {
		return "", "", false
	}
	return strings.TrimSuffix(header, ";base64"), data, true
}

func convertAnthropicMessageForOpenAI(msg map[string]json.RawMessage) ([]map[string]json.RawMessage, bool) {
	var role string
	_ = json.Unmarshal(msg["role"], &role)
	if role != "system" && role != "user" && role != "assistant" && role != "tool" {
		msg["role"] = json.RawMessage(`"user"`)
		role = "user"
	}

	blocks := contentBlocksFromRaw(msg["content"])
	if len(blocks) == 0 {
		return []map[string]json.RawMessage{msg}, role != rawString(msg["role"])
	}

	var textParts []string
	var contentParts []map[string]json.RawMessage
	var toolCalls []map[string]json.RawMessage
	var toolMessages []map[string]json.RawMessage
	modified := false

	for _, block := range blocks {
		var blockType string
		_ = json.Unmarshal(block["type"], &blockType)
		switch blockType {
		case "text":
			text := rawString(block["text"])
			if text == "" {
				continue
			}
			textParts = append(textParts, text)
			contentParts = append(contentParts, map[string]json.RawMessage{
				"type": json.RawMessage(`"text"`),
				"text": block["text"],
			})
		case "image":
			if part, ok := anthropicImageBlockToOpenAI(block); ok {
				contentParts = append(contentParts, part)
				modified = true
			}
		case "tool_use":
			if role == "assistant" {
				if call, ok := anthropicToolUseToOpenAI(block); ok {
					toolCalls = append(toolCalls, call)
					modified = true
				}
			}
		case "tool_result":
			if toolMsg, ok := anthropicToolResultToOpenAI(block); ok {
				toolMessages = append(toolMessages, toolMsg)
				modified = true
			}
		case "thinking", "redacted_thinking":
			modified = true
		default:
			modified = true
		}
	}

	if len(toolMessages) > 0 {
		return toolMessages, true
	}

	if len(contentParts) > 0 && len(contentParts) != len(textParts) {
		msg["content"], _ = json.Marshal(contentParts)
		modified = true
	} else {
		msg["content"], _ = json.Marshal(strings.Join(textParts, ""))
		modified = true
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"], _ = json.Marshal(toolCalls)
	}
	return []map[string]json.RawMessage{msg}, modified
}

func anthropicImageBlockToOpenAI(block map[string]json.RawMessage) (map[string]json.RawMessage, bool) {
	var source map[string]json.RawMessage
	if json.Unmarshal(block["source"], &source) != nil {
		return nil, false
	}
	sourceType := rawString(source["type"])
	url := rawString(source["url"])
	if sourceType == "base64" {
		mediaType := rawString(source["media_type"])
		data := rawString(source["data"])
		if mediaType != "" && data != "" {
			url = "data:" + mediaType + ";base64," + data
		}
	}
	if url == "" {
		return nil, false
	}
	imageURL, _ := json.Marshal(map[string]string{"url": url})
	return map[string]json.RawMessage{
		"type":      json.RawMessage(`"image_url"`),
		"image_url": imageURL,
	}, true
}

func anthropicToolUseToOpenAI(block map[string]json.RawMessage) (map[string]json.RawMessage, bool) {
	id := block["id"]
	name := block["name"]
	if len(id) == 0 || len(name) == 0 {
		return nil, false
	}
	args := "{}"
	if input := block["input"]; len(input) > 0 {
		args = string(input)
	}
	fn, _ := json.Marshal(map[string]json.RawMessage{
		"name":      name,
		"arguments": mustJSONMarshal(args),
	})
	return map[string]json.RawMessage{
		"id":       id,
		"type":     json.RawMessage(`"function"`),
		"function": fn,
	}, true
}

func anthropicToolResultToOpenAI(block map[string]json.RawMessage) (map[string]json.RawMessage, bool) {
	toolUseID := block["tool_use_id"]
	if len(toolUseID) == 0 {
		return nil, false
	}
	content := textFromContentRaw(block["content"])
	msg := map[string]json.RawMessage{
		"role":         json.RawMessage(`"tool"`),
		"tool_call_id": toolUseID,
	}
	msg["content"], _ = json.Marshal(content)
	return msg, true
}

func textFromContentRaw(raw json.RawMessage) string {
	if text := rawString(raw); text != "" {
		return text
	}
	var blocks []map[string]json.RawMessage
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, block := range blocks {
			if rawString(block["type"]) == "text" {
				if text := rawString(block["text"]); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	}
	return string(raw)
}

func rawString(raw json.RawMessage) string {
	var s string
	if len(raw) > 0 && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

func mustJSONMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func anthropicToolResultContent(msg map[string]json.RawMessage) json.RawMessage {
	toolUseID := msg["tool_call_id"]
	if len(toolUseID) == 0 {
		toolUseID = msg["name"]
	}

	content := msg["content"]
	if len(content) == 0 || string(content) == "null" {
		content = json.RawMessage(`""`)
	}

	block := map[string]json.RawMessage{
		"type":    json.RawMessage(`"tool_result"`),
		"content": content,
	}
	if len(toolUseID) > 0 {
		block["tool_use_id"] = toolUseID
	}
	blocks := []map[string]json.RawMessage{block}
	out, _ := json.Marshal(blocks)
	return out
}

func dropAnthropicOnlyContentBlocks(msg map[string]json.RawMessage) bool {
	contentRaw := msg["content"]
	if len(contentRaw) == 0 {
		return false
	}

	var blocks []map[string]json.RawMessage
	if json.Unmarshal(contentRaw, &blocks) != nil {
		return false
	}

	var textParts []string
	for _, block := range blocks {
		var blockType string
		json.Unmarshal(block["type"], &blockType)
		if blockType == "text" {
			var text string
			json.Unmarshal(block["text"], &text)
			if text != "" {
				textParts = append(textParts, text)
			}
		}
	}

	text := strings.Join(textParts, "")
	msg["content"], _ = json.Marshal(text)
	return true
}

// openAIFields are fields accepted by OpenAI-compatible chat completion APIs.
var openAIFields = map[string]bool{
	"model": true, "messages": true, "max_tokens": true, "max_completion_tokens": true,
	"temperature": true, "top_p": true, "n": true, "stream": true, "stream_options": true,
	"stop": true, "presence_penalty": true, "frequency_penalty": true,
	"logit_bias": true, "user": true, "response_format": true, "seed": true,
	"tools": true, "tool_choice": true, "parallel_tool_calls": true,
	"logprobs": true, "top_logprobs": true, "reasoning_effort": true,
	"functions": true, "function_call": true, "store": true, "metadata": true,
	"service_tier": true, "modalities": true, "audio": true, "prediction": true,
	"web_search_options": true,
}

// anthropicFields are fields accepted by the Anthropic messages API.
var anthropicFields = map[string]bool{
	"model": true, "messages": true, "max_tokens": true,
	"temperature": true, "top_p": true, "top_k": true, "stream": true,
	"stop_sequences": true, "system": true, "tools": true,
	"tool_choice": true, "thinking": true, "metadata": true,
	"container": true, "context_management": true, "mcp_servers": true,
	"service_tier": true,
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

// sanitizeThinkingForProtocol intentionally keeps Anthropic thinking untouched.
// The response converter bridges thinking blocks back to OpenAI-compatible clients.
func sanitizeThinkingForProtocol(reqMap map[string]json.RawMessage, isSourceAnthropic, isTargetAnthropic bool) {
	// When the target is Anthropic, thinking must be forwarded exactly as the
	// client provided it. Non-Anthropic cleanup is handled by stripThinkingBlocks.
	_ = reqMap
	_ = isSourceAnthropic
	_ = isTargetAnthropic
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
			if blockType != "thinking" && blockType != "redacted_thinking" {
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

// sanitizeNullContent replaces null message content with empty strings.
// Some upstream providers reject "content": null in messages.
func sanitizeNullContent(reqMap map[string]json.RawMessage) {
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
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(msgRaw, &msg) != nil {
			continue
		}

		if len(msg.Content) == 0 || string(msg.Content) == "null" {
			var msgMap map[string]json.RawMessage
			if json.Unmarshal(msgRaw, &msgMap) != nil {
				continue
			}
			msgMap["content"] = json.RawMessage(`""`)
			msgs[i], _ = json.Marshal(msgMap)
			modified = true
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
			if role == "system" || role == "developer" {
				content := textFromContentRaw(m["content"])
				if content != "" {
					systemTexts = append(systemTexts, content)
				}
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
			"POST /chat/completions",
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
