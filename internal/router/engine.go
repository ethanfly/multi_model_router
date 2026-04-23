package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"multi_model_router/internal/provider"

	"github.com/google/uuid"
)

// ModelConfig holds the configuration for a single model.
type ModelConfig struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Provider       string `json:"provider"`
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	ModelID        string `json:"model_id"`
	Reasoning      int    `json:"reasoning"`
	Coding         int    `json:"coding"`
	Creativity     int    `json:"creativity"`
	Speed          int    `json:"speed"`
	CostEfficiency int    `json:"cost_efficiency"`
	MaxRPM         int    `json:"max_rpm"`
	MaxTPM         int    `json:"max_tpm"`
	IsActive       bool   `json:"is_active"`

	// ProviderInstance is the actual provider client, set at load time.
	ProviderInstance provider.Provider `json:"-"`
}

// RouteMode represents the routing mode.
type RouteMode int

const (
	RouteAuto   RouteMode = iota
	RouteManual
	RouteRace
)

// RouteModeFromString converts a string to a RouteMode.
func RouteModeFromString(s string) RouteMode {
	switch s {
	case "manual":
		return RouteManual
	case "race":
		return RouteRace
	default:
		return RouteAuto
	}
}

// String returns the string representation of a RouteMode.
func (r RouteMode) String() string {
	switch r {
	case RouteManual:
		return "manual"
	case RouteRace:
		return "race"
	default:
		return "auto"
	}
}

// complexityWeights maps complexity levels to weighted scoring dimensions.
var complexityWeights = map[Complexity]struct {
	Reasoning     float64
	Coding        float64
	Creativity    float64
	Speed         float64
	CostEfficiency float64
}{
	Simple: {
		Reasoning:     0.1,
		Coding:        0.1,
		Creativity:    0.15,
		Speed:         0.35,
		CostEfficiency: 0.3,
	},
	Medium: {
		Reasoning:     0.2,
		Coding:        0.2,
		Creativity:    0.2,
		Speed:         0.2,
		CostEfficiency: 0.2,
	},
	Complex: {
		Reasoning:     0.35,
		Coding:        0.3,
		Creativity:    0.1,
		Speed:         0.1,
		CostEfficiency: 0.15,
	},
}

// RouteRequest is the input to the router engine.
type RouteRequest struct {
	Messages    []provider.Message
	Mode        RouteMode
	ModelID     string
	Source      string
	MaxTokens   int
	Temperature float64
}

// RouteResult is the output from the router engine.
type RouteResult struct {
	ModelID     int64
	ModelName   string
	Provider    string
	Complexity  int64
	RouteMode   int64
	TokensIn    int64
	TokensOut   int64
	LatencyMs   int64
	Status      string
	ErrorMsg    string
	Stream      <-chan provider.StreamChunk
}

// Engine is the main router engine that selects models and routes requests.
type Engine struct {
	classifier  *Classifier
	models      map[string]*ModelConfig
	rateLimits  map[string]time.Time
	mu          sync.RWMutex
}

// NewEngine creates a new router Engine with the given classifier.
func NewEngine(classifier *Classifier) *Engine {
	return &Engine{
		classifier: classifier,
		models:     make(map[string]*ModelConfig),
		rateLimits: make(map[string]time.Time),
	}
}

// AddProvider is kept for backward compatibility but is a no-op.
// Each model now carries its own provider instance.
func (e *Engine) AddProvider(name string, p provider.Provider) {
	// no-op: providers are per-model now
}

// SetModels replaces the current model configurations.
func (e *Engine) SetModels(models []*ModelConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.models = make(map[string]*ModelConfig)
	for _, m := range models {
		e.models[m.ID] = m
	}
}

// Route dispatches the request based on the route mode.
func (e *Engine) Route(ctx context.Context, req *RouteRequest) *RouteResult {
	switch req.Mode {
	case RouteManual:
		return e.routeManual(ctx, req)
	case RouteRace:
		return e.routeRace(ctx, req)
	default:
		return e.routeAuto(ctx, req)
	}
}

// SelectModel performs routing and returns the selected ModelConfig without sending the request.
// This enables raw passthrough proxying where the caller forwards the original request body directly.
func (e *Engine) SelectModel(ctx context.Context, req *RouteRequest) (*ModelConfig, int64, string) {
	switch req.Mode {
	case RouteManual:
		model := e.findModel(req.ModelID)
		if model == nil {
			return nil, 0, fmt.Sprintf("model %s not found", req.ModelID)
		}
		return model, 0, ""
	default: // RouteAuto and RouteRace both select the best model
		question := messagesToString(req.Messages)
		classResult, err := e.classifier.Classify(ctx, question)
		if err != nil {
			return nil, 0, fmt.Sprintf("classification failed: %v", err)
		}
		model := e.selectModel(classResult.Complexity)
		if model == nil {
			return nil, int64(classResult.Complexity), "no available model"
		}
		return model, int64(classResult.Complexity), ""
	}
}

// routeAuto classifies the question, selects the best model, and sends the request.
func (e *Engine) routeAuto(ctx context.Context, req *RouteRequest) *RouteResult {
	question := messagesToString(req.Messages)
	classResult, err := e.classifier.Classify(ctx, question)
	if err != nil {
		return &RouteResult{
			Status:   "error",
			ErrorMsg: fmt.Sprintf("classification failed: %v", err),
		}
	}

	model := e.selectModel(classResult.Complexity)
	if model == nil {
		return &RouteResult{
			Status:      "error",
			ErrorMsg:    "no available model",
			Complexity:  int64(classResult.Complexity),
		}
	}

	result := e.sendToModel(ctx, model, req)
	if result == nil || result.Status != "success" {
		// Try fallback
		fallback := e.selectModelFallback(classResult.Complexity, model.ID)
		if fallback != nil {
			result = e.sendToModel(ctx, fallback, req)
		}
	}

	if result != nil {
		result.Complexity = int64(classResult.Complexity)
		result.RouteMode = int64(RouteAuto)
	}
	return result
}

// routeManual routes the request to a specific model by ID or model name.
func (e *Engine) routeManual(ctx context.Context, req *RouteRequest) *RouteResult {
	model := e.findModel(req.ModelID)
	if model == nil {
		return &RouteResult{
			Status:   "error",
			ErrorMsg: fmt.Sprintf("model %s not found", req.ModelID),
		}
	}

	result := e.sendToModel(ctx, model, req)
	if result != nil {
		result.RouteMode = int64(RouteManual)
	}
	return result
}

// findModel looks up a model by internal UUID (ID) or API model name (ModelID).
func (e *Engine) findModel(idOrName string) *ModelConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Try exact UUID match first
	if m, ok := e.models[idOrName]; ok {
		return m
	}

	// Fallback: search by model name (ModelID field)
	for _, m := range e.models {
		if m.ModelID == idOrName {
			return m
		}
	}

	return nil
}

// routeRace sends the request to all active, non-rate-limited models concurrently
// and returns the first successful result.
func (e *Engine) routeRace(ctx context.Context, req *RouteRequest) *RouteResult {
	e.mu.RLock()
	var candidates []*ModelConfig
	for _, m := range e.models {
		if m.IsActive && !e.isRateLimited(m.ID) {
			candidates = append(candidates, m)
		}
	}
	e.mu.RUnlock()

	if len(candidates) == 0 {
		return &RouteResult{
			Status:   "error",
			ErrorMsg: "no available models for race",
		}
	}

	type raceResult struct {
		result *RouteResult
	}

	ch := make(chan *raceResult, len(candidates))
	for _, m := range candidates {
		go func(cfg *ModelConfig) {
			r := e.sendToModel(ctx, cfg, req)
			ch <- &raceResult{result: r}
		}(m)
	}

	for range candidates {
		rr := <-ch
		if rr.result != nil && rr.result.Status == "success" {
			rr.result.RouteMode = int64(RouteRace)
			return rr.result
		}
	}

	return &RouteResult{
		Status:   "error",
		ErrorMsg: "all models failed in race",
	}
}

// selectModel selects the best model for the given complexity using weighted scoring.
func (e *Engine) selectModel(complexity Complexity) *ModelConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()

	weights, ok := complexityWeights[complexity]
	if !ok {
		weights = complexityWeights[Medium]
	}

	var best *ModelConfig
	bestScore := -1.0

	for _, m := range e.models {
		if !m.IsActive || e.isRateLimited(m.ID) {
			continue
		}
		score := weights.Reasoning*float64(m.Reasoning) +
			weights.Coding*float64(m.Coding) +
			weights.Creativity*float64(m.Creativity) +
			weights.Speed*float64(m.Speed) +
			weights.CostEfficiency*float64(m.CostEfficiency)
		if score > bestScore {
			bestScore = score
			best = m
		}
	}

	return best
}

// selectModelFallback selects a fallback model, excluding the given model ID.
func (e *Engine) selectModelFallback(complexity Complexity, excludeID string) *ModelConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()

	weights, ok := complexityWeights[complexity]
	if !ok {
		weights = complexityWeights[Medium]
	}

	var best *ModelConfig
	bestScore := -1.0

	for _, m := range e.models {
		if !m.IsActive || m.ID == excludeID || e.isRateLimited(m.ID) {
			continue
		}
		score := weights.Reasoning*float64(m.Reasoning) +
			weights.Coding*float64(m.Coding) +
			weights.Creativity*float64(m.Creativity) +
			weights.Speed*float64(m.Speed) +
			weights.CostEfficiency*float64(m.CostEfficiency)
		if score > bestScore {
			bestScore = score
			best = m
		}
	}

	return best
}

// sendToModel sends a request to the specified model via its provider.
func (e *Engine) sendToModel(ctx context.Context, cfg *ModelConfig, req *RouteRequest) *RouteResult {
	if cfg.ProviderInstance == nil {
		return &RouteResult{
			Status:   "error",
			ErrorMsg: fmt.Sprintf("provider not initialized for model %s", cfg.Name),
		}
	}

	start := time.Now()

	chatReq := &provider.ChatRequest{
		Model:       cfg.ModelID,
		Messages:    req.Messages,
		Stream:      true,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	stream, err := cfg.ProviderInstance.ChatCompletion(ctx, chatReq)
	if err != nil {
		return &RouteResult{
			Status:   "error",
			ErrorMsg: err.Error(),
		}
	}

	latency := time.Since(start).Milliseconds()

	return &RouteResult{
		ModelID:   int64(len(cfg.ID)), // use length as placeholder ID
		ModelName: cfg.Name,
		Provider:  cfg.Provider,
		LatencyMs: latency,
		Status:    "success",
		Stream:    stream,
	}
}

// isRateLimited checks if a model is currently rate-limited.
func (e *Engine) isRateLimited(modelID string) bool {
	if t, ok := e.rateLimits[modelID]; ok {
		return time.Now().Before(t)
	}
	return false
}

// messagesToString concatenates all message contents into a single string.
// Handles both string content and array (multi-modal) content by extracting text parts.
func messagesToString(msgs []provider.Message) string {
	var sb string
	for i := range msgs {
		sb += msgs[i].ExtractText()
	}
	return sb
}

// ListActiveModels returns all active model configurations.
func (e *Engine) ListActiveModels() []*ModelConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var active []*ModelConfig
	for _, m := range e.models {
		if m.IsActive {
			active = append(active, m)
		}
	}
	return active
}

// ProviderInfo holds the information needed to forward a request to an upstream provider.
type ProviderInfo struct {
	BaseURL  string
	APIKey   string
	ModelID  string
	Provider string // "openai" or "anthropic"
}

// ResolveProvider finds the provider configuration for the given model reference.
// It matches by ModelID, ID, or Name. Falls back to the first active model if no match.
func (e *Engine) ResolveProvider(modelRef string) (ProviderInfo, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Exact match by ModelID, ID, or Name
	for _, m := range e.models {
		if !m.IsActive {
			continue
		}
		if m.ModelID == modelRef || m.ID == modelRef || m.Name == modelRef {
			return ProviderInfo{
				BaseURL:  m.BaseURL,
				APIKey:   m.APIKey,
				ModelID:  m.ModelID,
				Provider: m.Provider,
			}, true
		}
	}

	// Fallback: first active OpenAI-compatible provider
	for _, m := range e.models {
		if m.IsActive && m.Provider == "openai" {
			return ProviderInfo{
				BaseURL:  m.BaseURL,
				APIKey:   m.APIKey,
				ModelID:  m.ModelID,
				Provider: m.Provider,
			}, true
		}
	}

	// Last resort: any active provider
	for _, m := range e.models {
		if m.IsActive {
			return ProviderInfo{
				BaseURL:  m.BaseURL,
				APIKey:   m.APIKey,
				ModelID:  m.ModelID,
				Provider: m.Provider,
			}, true
		}
	}

	return ProviderInfo{}, false
}

// NewUUID generates a new UUID string.
func NewUUID() string {
	return uuid.New().String()
}
