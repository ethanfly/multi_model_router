package router

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	RouteAuto RouteMode = iota
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

type scoreWeights struct {
	Reasoning      float64
	Coding         float64
	Creativity     float64
	Speed          float64
	CostEfficiency float64
}

// complexityWeights maps complexity levels to weighted scoring dimensions.
var complexityWeights = map[Complexity]scoreWeights{
	Simple: {
		Reasoning:      0.1,
		Coding:         0.1,
		Creativity:     0.15,
		Speed:          0.2,
		CostEfficiency: 0.45,
	},
	Medium: {
		Reasoning:      0.2,
		Coding:         0.2,
		Creativity:     0.1,
		Speed:          0.15,
		CostEfficiency: 0.35,
	},
	Complex: {
		Reasoning:      0.4,
		Coding:         0.3,
		Creativity:     0.1,
		Speed:          0.1,
		CostEfficiency: 0.1,
	},
}

var taskWeights = map[TaskType]scoreWeights{
	TaskFast: {
		Reasoning:      0.05,
		Coding:         0.05,
		Creativity:     0.05,
		Speed:          0.3,
		CostEfficiency: 0.55,
	},
	TaskCoding: {
		Reasoning:      0.22,
		Coding:         0.48,
		Creativity:     0.05,
		Speed:          0.12,
		CostEfficiency: 0.13,
	},
	TaskDebugging: {
		Reasoning:      0.35,
		Coding:         0.4,
		Creativity:     0.03,
		Speed:          0.1,
		CostEfficiency: 0.12,
	},
	TaskAgentic: {
		Reasoning:      0.25,
		Coding:         0.35,
		Creativity:     0.1,
		Speed:          0.15,
		CostEfficiency: 0.15,
	},
	TaskArchitecture: {
		Reasoning:      0.45,
		Coding:         0.25,
		Creativity:     0.08,
		Speed:          0.08,
		CostEfficiency: 0.14,
	},
	TaskCreative: {
		Reasoning:      0.18,
		Coding:         0.05,
		Creativity:     0.45,
		Speed:          0.14,
		CostEfficiency: 0.18,
	},
}

func weightsForRoute(complexity Complexity, taskType TaskType) scoreWeights {
	if weights, ok := taskWeights[taskType]; ok {
		return weights
	}
	if weights, ok := complexityWeights[complexity]; ok {
		return weights
	}
	return complexityWeights[Medium]
}

type modelRoutingProfile struct {
	Reason        string
	FastBonus     float64
	CodingBonus   float64
	AgenticBonus  float64
	ReasonBonus   float64
	CreativeBonus float64
	GeneralBonus  float64
}

func (p modelRoutingProfile) Bonus(taskType TaskType) float64 {
	switch taskType {
	case TaskFast:
		return p.FastBonus + p.GeneralBonus
	case TaskCoding, TaskDebugging:
		return p.CodingBonus + p.ReasonBonus*0.4 + p.GeneralBonus
	case TaskAgentic:
		return p.AgenticBonus + p.CodingBonus*0.4 + p.ReasonBonus*0.2 + p.GeneralBonus
	case TaskArchitecture:
		return p.ReasonBonus + p.CodingBonus*0.25 + p.GeneralBonus
	case TaskCreative:
		return p.CreativeBonus + p.ReasonBonus*0.2 + p.GeneralBonus
	default:
		return p.GeneralBonus
	}
}

func routingProfileForModel(model *ModelConfig) modelRoutingProfile {
	if model == nil {
		return modelRoutingProfile{}
	}
	key := strings.ToLower(strings.Join([]string{model.Name, model.ModelID, model.Provider, model.BaseURL}, " "))

	switch {
	case strings.Contains(key, "kimi-k2.6") || strings.Contains(key, "kimi k2.6"):
		return modelRoutingProfile{
			Reason:       "benchmark profile: Kimi K2.6 is weighted toward coding and agentic tool workflows",
			CodingBonus:  0.75,
			AgenticBonus: 1.1,
			ReasonBonus:  0.35,
			GeneralBonus: 0.05,
		}
	case strings.Contains(key, "glm-5.1") || strings.Contains(key, "glm 5.1"):
		return modelRoutingProfile{
			Reason:       "benchmark profile: GLM 5.1 is weighted toward coding, reasoning, and efficient agent work",
			FastBonus:    0.2,
			CodingBonus:  0.65,
			AgenticBonus: 0.55,
			ReasonBonus:  0.55,
			GeneralBonus: 0.1,
		}
	case strings.Contains(key, "gpt-5.5") || strings.Contains(key, "gpt5.5"):
		return modelRoutingProfile{
			Reason:        "benchmark profile: GPT-5.5 is weighted as a frontier general reasoning and coding model",
			FastBonus:     0.1,
			CodingBonus:   0.55,
			AgenticBonus:  0.45,
			ReasonBonus:   0.75,
			CreativeBonus: 0.45,
			GeneralBonus:  0.15,
		}
	case strings.Contains(key, "deepseek-v4-pro") || strings.Contains(key, "deepseekv4") || strings.Contains(key, "deepseek v4"):
		return modelRoutingProfile{
			Reason:        "benchmark profile: DeepSeek V4 Pro is weighted toward strong reasoning, coding, and broad generation",
			CodingBonus:   0.6,
			AgenticBonus:  0.35,
			ReasonBonus:   0.65,
			CreativeBonus: 0.25,
			GeneralBonus:  0.05,
		}
	case strings.Contains(key, "minimax-m2.7") || strings.Contains(key, "m2.7-highspeed") || strings.Contains(key, "minimax"):
		return modelRoutingProfile{
			Reason:       "benchmark profile: MiniMax M2.7 highspeed is weighted toward speed, cost, and balanced agent work",
			FastBonus:    1.1,
			CodingBonus:  0.2,
			AgenticBonus: 0.35,
			ReasonBonus:  0.15,
			GeneralBonus: 0.15,
		}
	case strings.Contains(key, "gemma-4") || strings.Contains(key, "gemma4"):
		return modelRoutingProfile{
			Reason:       "benchmark profile: Gemma 4 26B is weighted toward low-cost local/private simple work",
			FastBonus:    1.0,
			CodingBonus:  -0.1,
			ReasonBonus:  -0.15,
			GeneralBonus: 0.05,
		}
	default:
		return modelRoutingProfile{}
	}
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
	Diagnostics *RouteDiagnostics
}

type RouteDiagnostics struct {
	Mode                 string                `json:"mode"`
	ClassificationInput  string                `json:"classificationInput,omitempty"`
	ClassificationMethod string                `json:"classificationMethod,omitempty"`
	Complexity           string                `json:"complexity,omitempty"`
	TaskType             string                `json:"taskType,omitempty"`
	EstimatedTokens      int                   `json:"estimatedTokens,omitempty"`
	SelectedModel        string                `json:"selectedModel,omitempty"`
	FallbackUsed         bool                  `json:"fallbackUsed"`
	Summary              string                `json:"summary"`
	Candidates           []CandidateDiagnostic `json:"candidates,omitempty"`
}

type CandidateDiagnostic struct {
	Name      string  `json:"name"`
	ModelID   string  `json:"modelId"`
	Provider  string  `json:"provider"`
	Eligible  bool    `json:"eligible"`
	Score     float64 `json:"score,omitempty"`
	Decision  string  `json:"decision,omitempty"`
	Reason    string  `json:"reason,omitempty"`
	RecentRPM int     `json:"recentRpm"`
	RecentTPM int     `json:"recentTpm"`
	MaxRPM    int     `json:"maxRpm"`
	MaxTPM    int     `json:"maxTpm"`
}

type SelectionResult struct {
	Model       *ModelConfig
	Complexity  int64
	ErrorMsg    string
	Diagnostics *RouteDiagnostics
}

// Engine is the main router engine that selects models and routes requests.
type Engine struct {
	classifier   *Classifier
	models       map[string]*ModelConfig
	rateLimits   map[string]time.Time
	usageRecords map[string][]usageRecord
	mu           sync.RWMutex
}

type usageRecord struct {
	requestID string
	timestamp time.Time
	tokens    int
}

type rankedCandidate struct {
	model *ModelConfig
	score float64
}

type candidateEvaluation struct {
	model     *ModelConfig
	score     float64
	eligible  bool
	reason    string
	profile   string
	recentRPM int
	recentTPM int
}

type tokenEstimate struct {
	promptTokens int
	totalTokens  int
}

const (
	usageWindow                   = time.Minute
	providerRateLimitBackoff      = 30 * time.Second
	defaultCompletionTokenReserve = 512
	tieScoreEpsilon               = 1e-9
)

// NewEngine creates a new router Engine with the given classifier.
func NewEngine(classifier *Classifier) *Engine {
	return &Engine{
		classifier:   classifier,
		models:       make(map[string]*ModelConfig),
		rateLimits:   make(map[string]time.Time),
		usageRecords: make(map[string][]usageRecord),
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
	selection := e.ExplainSelection(ctx, req)
	if selection == nil {
		return nil, 0, "selection unavailable"
	}
	return selection.Model, selection.Complexity, selection.ErrorMsg
}

func (e *Engine) ExplainSelection(ctx context.Context, req *RouteRequest) *SelectionResult {
	switch req.Mode {
	case RouteManual:
		model := e.findModel(req.ModelID)
		if model == nil {
			return &SelectionResult{
				ErrorMsg: fmt.Sprintf("model %s not found", req.ModelID),
				Diagnostics: &RouteDiagnostics{
					Mode:    RouteManual.String(),
					Summary: fmt.Sprintf("mode=manual; model=%s; error=model not found", req.ModelID),
				},
			}
		}
		diagnostics := &RouteDiagnostics{
			Mode:          RouteManual.String(),
			SelectedModel: model.Name,
			Summary:       fmt.Sprintf("mode=manual; selected=%s", model.Name),
			Candidates: []CandidateDiagnostic{{
				Name:     model.Name,
				ModelID:  model.ModelID,
				Provider: model.Provider,
				Eligible: true,
				Decision: "selected",
			}},
		}
		return &SelectionResult{
			Model:       model,
			Complexity:  0,
			Diagnostics: diagnostics,
		}
	default: // RouteAuto and RouteRace both select the best model
		question := messagesForClassification(req.Messages)
		classResult, err := e.classifier.Classify(ctx, question)
		if err != nil {
			return &SelectionResult{
				ErrorMsg: fmt.Sprintf("classification failed: %v", err),
				Diagnostics: &RouteDiagnostics{
					Mode:                req.Mode.String(),
					ClassificationInput: question,
					Summary:             fmt.Sprintf("mode=%s; error=classification failed", req.Mode.String()),
				},
			}
		}
		evaluations := e.evaluateCandidates(classResult.Complexity, classResult.TaskType, req, nil)
		models := eligibleModels(evaluations)
		diagnostics := diagnosticsFromEvaluations(req.Mode.String(), question, classResult, estimateRequestTokens(req).totalTokens, evaluations)
		if len(models) == 0 {
			diagnostics.Summary = diagnosticsSummary(diagnostics)
			return &SelectionResult{
				Complexity:  int64(classResult.Complexity),
				ErrorMsg:    unavailableModelError(evaluations),
				Diagnostics: diagnostics,
			}
		}
		diagnostics.SelectedModel = models[0].Name
		diagnostics.Candidates = markDiagnosticDecision(diagnostics.Candidates, models[0].Name, "selected", "")
		diagnostics.Summary = diagnosticsSummary(diagnostics)
		return &SelectionResult{
			Model:       models[0],
			Complexity:  int64(classResult.Complexity),
			Diagnostics: diagnostics,
		}
	}
}

// routeAuto classifies the question, selects the best model, and sends the request.
func (e *Engine) routeAuto(ctx context.Context, req *RouteRequest) *RouteResult {
	question := messagesForClassification(req.Messages)
	classResult, err := e.classifier.Classify(ctx, question)
	if err != nil {
		return &RouteResult{
			Status:   "error",
			ErrorMsg: fmt.Sprintf("classification failed: %v", err),
		}
	}

	estimate := estimateRequestTokens(req)
	evaluations := e.evaluateCandidates(classResult.Complexity, classResult.TaskType, req, nil)
	diagnostics := diagnosticsFromEvaluations(RouteAuto.String(), question, classResult, estimate.totalTokens, evaluations)
	candidates := eligibleModels(evaluations)
	if len(candidates) == 0 {
		diagnostics.Summary = diagnosticsSummary(diagnostics)
		return &RouteResult{
			Status:      "error",
			ErrorMsg:    unavailableModelError(evaluations),
			Complexity:  int64(classResult.Complexity),
			Diagnostics: diagnostics,
		}
	}

	var result *RouteResult
	var lastErr *RouteResult

	for i, model := range candidates {
		result = e.sendToModel(ctx, model, req)
		if result == nil || result.Status != "success" {
			lastErr = result
			diagnostics.Candidates = markDiagnosticDecision(diagnostics.Candidates, model.Name, "failed", errorMessage(result))
			continue
		}

		result = e.prepareAutoStreamResult(ctx, req, result, candidates[i+1:])
		if result != nil && result.Status == "success" {
			result.Complexity = int64(classResult.Complexity)
			result.RouteMode = int64(RouteAuto)
			result.Diagnostics = diagnostics
			result.Diagnostics.SelectedModel = result.ModelName
			result.Diagnostics.FallbackUsed = i > 0 || result.ModelName != candidates[0].Name
			result.Diagnostics.Candidates = markDiagnosticDecision(result.Diagnostics.Candidates, result.ModelName, "selected", "")
			result.Diagnostics.Summary = diagnosticsSummary(result.Diagnostics)
			return result
		}
		lastErr = result
		diagnostics.Candidates = markDiagnosticDecision(diagnostics.Candidates, model.Name, "failed", errorMessage(result))
	}

	if lastErr != nil {
		lastErr.Complexity = int64(classResult.Complexity)
		lastErr.RouteMode = int64(RouteAuto)
		lastErr.Diagnostics = diagnostics
		lastErr.Diagnostics.Summary = diagnosticsSummary(lastErr.Diagnostics)
		return lastErr
	}

	diagnostics.Summary = diagnosticsSummary(diagnostics)
	return &RouteResult{
		Status:      "error",
		ErrorMsg:    "all auto-route candidates failed",
		Complexity:  int64(classResult.Complexity),
		RouteMode:   int64(RouteAuto),
		Diagnostics: diagnostics,
	}
}

func (e *Engine) prepareAutoStreamResult(ctx context.Context, req *RouteRequest, result *RouteResult, fallbacks []*ModelConfig) *RouteResult {
	current := result
	remaining := append([]*ModelConfig(nil), fallbacks...)

	for {
		buffered, failedEarly := primeStream(current.Stream)
		if !failedEarly {
			current.Stream = prependStream(buffered, current.Stream)
			return current
		}

		if len(remaining) == 0 {
			current.Stream = bufferedStream(buffered)
			return current
		}

		switched := false
		for len(remaining) > 0 {
			nextModel := remaining[0]
			remaining = remaining[1:]

			nextResult := e.sendToModel(ctx, nextModel, req)
			if nextResult == nil || nextResult.Status != "success" {
				current = nextResult
				continue
			}

			current = nextResult
			switched = true
			break
		}

		if !switched {
			if current != nil && current.Status != "success" {
				return current
			}
			current.Stream = bufferedStream(buffered)
			return current
		}
	}
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
		result.Diagnostics = &RouteDiagnostics{
			Mode:          RouteManual.String(),
			SelectedModel: model.Name,
			Summary:       fmt.Sprintf("mode=manual; selected=%s", model.Name),
			Candidates: []CandidateDiagnostic{{
				Name:     model.Name,
				ModelID:  model.ModelID,
				Provider: model.Provider,
				Eligible: true,
				Decision: "selected",
			}},
		}
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
	candidates := e.raceCandidates(req)
	diagnostics := raceDiagnostics(req, candidates)

	if len(candidates) == 0 {
		return &RouteResult{
			Status:      "error",
			ErrorMsg:    "no available models for race",
			Diagnostics: diagnostics,
		}
	}

	type raceResult struct {
		modelID string
		result  *RouteResult
	}

	raceCtx, raceCancel := context.WithCancel(ctx)
	defer raceCancel()

	ch := make(chan *raceResult, len(candidates))
	cancelFns := make(map[string]context.CancelFunc, len(candidates))
	for _, m := range candidates {
		candidateCtx, cancel := context.WithCancel(raceCtx)
		cancelFns[m.ID] = cancel
		go func(cfg *ModelConfig) {
			r := e.sendToModel(candidateCtx, cfg, req)
			if r != nil && r.Status == "success" {
				buffered, failedEarly := primeStream(r.Stream)
				if failedEarly {
					r = &RouteResult{
						ModelName: cfg.Name,
						Provider:  cfg.Provider,
						Status:    "error",
						ErrorMsg:  chunkErrorMessage(buffered),
					}
				} else {
					r.Stream = prependStream(buffered, r.Stream)
				}
			}
			ch <- &raceResult{modelID: cfg.ID, result: r}
		}(m)
	}

	for range candidates {
		rr := <-ch
		if rr.result != nil && rr.result.Status == "success" {
			for modelID, cancel := range cancelFns {
				if modelID != rr.modelID {
					cancel()
				}
			}
			rr.result.RouteMode = int64(RouteRace)
			rr.result.Diagnostics = diagnostics
			rr.result.Diagnostics.SelectedModel = rr.result.ModelName
			rr.result.Diagnostics.Candidates = markDiagnosticDecision(rr.result.Diagnostics.Candidates, rr.result.ModelName, "selected", "")
			rr.result.Diagnostics.Summary = diagnosticsSummary(rr.result.Diagnostics)
			return rr.result
		}
		diagnostics.Candidates = markDiagnosticDecision(diagnostics.Candidates, candidateNameByID(candidates, rr.modelID), "failed", errorMessage(rr.result))
	}

	for _, cancel := range cancelFns {
		cancel()
	}

	diagnostics.Summary = diagnosticsSummary(diagnostics)
	return &RouteResult{
		Status:      "error",
		ErrorMsg:    "all models failed in race",
		Diagnostics: diagnostics,
	}
}

// selectModel selects the best model for the given complexity using weighted scoring.
func (e *Engine) selectModel(complexity Complexity, req *RouteRequest) *ModelConfig {
	candidates := e.rankedCandidates(complexity, TaskGeneral, req, nil)
	if len(candidates) == 0 {
		return nil
	}
	return candidates[0]
}

// selectModelFallback selects a fallback model, excluding the given model ID.
func (e *Engine) selectModelFallback(complexity Complexity, req *RouteRequest, excludeID string) *ModelConfig {
	candidates := e.rankedCandidates(complexity, TaskGeneral, req, map[string]bool{excludeID: true})
	if len(candidates) == 0 {
		return nil
	}
	return candidates[0]
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
	estimate := estimateRequestTokens(req)
	reservationID := e.reserveUsage(cfg.ID, estimate.totalTokens)

	chatReq := &provider.ChatRequest{
		Model:       cfg.ModelID,
		Messages:    req.Messages,
		Stream:      true,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	stream, err := cfg.ProviderInstance.ChatCompletion(ctx, chatReq)
	if err != nil {
		e.reconcileUsage(cfg.ID, reservationID, 0)
		if isRateLimitError(err) {
			e.markRateLimited(cfg.ID, time.Now().Add(providerRateLimitBackoff))
		}
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
		Stream:    e.observeStream(cfg.ID, reservationID, estimate, stream),
	}
}

// isRateLimited checks if a model is currently rate-limited.
func (e *Engine) isRateLimited(modelID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.isRateLimitedLocked(modelID, time.Now())
}

func (e *Engine) rankedCandidates(complexity Complexity, taskType TaskType, req *RouteRequest, exclude map[string]bool) []*ModelConfig {
	return eligibleModels(e.evaluateCandidates(complexity, taskType, req, exclude))
}

func (e *Engine) raceCandidates(req *RouteRequest) []*ModelConfig {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	estimate := estimateRequestTokens(req)
	candidates := make([]*ModelConfig, 0, len(e.models))

	for _, m := range e.models {
		if !m.IsActive || e.isRateLimitedLocked(m.ID, now) || !e.withinCapacityLocked(m, estimate.totalTokens, now) {
			continue
		}
		candidates = append(candidates, m)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return modelTieBreakKey(candidates[i]) < modelTieBreakKey(candidates[j])
	})

	return candidates
}

func (e *Engine) evaluateCandidates(complexity Complexity, taskType TaskType, req *RouteRequest, exclude map[string]bool) []candidateEvaluation {
	e.mu.Lock()
	defer e.mu.Unlock()

	weights := weightsForRoute(complexity, taskType)

	now := time.Now()
	estimate := estimateRequestTokens(req)
	evaluations := make([]candidateEvaluation, 0, len(e.models))

	for _, m := range e.models {
		evaluation := candidateEvaluation{model: m}

		if exclude != nil && exclude[m.ID] {
			evaluation.reason = "excluded after prior failure"
			evaluations = append(evaluations, evaluation)
			continue
		}
		if !m.IsActive {
			evaluation.reason = "inactive"
			evaluations = append(evaluations, evaluation)
			continue
		}
		if e.isRateLimitedLocked(m.ID, now) {
			evaluation.reason = "rate limited cooldown active"
			evaluations = append(evaluations, evaluation)
			continue
		}

		e.pruneUsageLocked(m.ID, now)
		records := e.usageRecords[m.ID]
		evaluation.recentRPM = len(records)
		for _, record := range records {
			evaluation.recentTPM += record.tokens
		}

		switch {
		case m.MaxRPM > 0 && evaluation.recentRPM >= m.MaxRPM:
			evaluation.reason = fmt.Sprintf("rpm capacity reached (%d/%d)", evaluation.recentRPM, m.MaxRPM)
		case m.MaxTPM > 0 && evaluation.recentTPM+estimate.totalTokens > m.MaxTPM:
			evaluation.reason = fmt.Sprintf("tpm capacity exceeded (%d+%d>%d)", evaluation.recentTPM, estimate.totalTokens, m.MaxTPM)
		default:
			profile := routingProfileForModel(m)
			evaluation.eligible = true
			evaluation.profile = profile.Reason
			evaluation.score = weights.Reasoning*float64(m.Reasoning) +
				weights.Coding*float64(m.Coding) +
				weights.Creativity*float64(m.Creativity) +
				weights.Speed*float64(m.Speed) +
				weights.CostEfficiency*float64(m.CostEfficiency) +
				profile.Bonus(taskType)
		}

		evaluations = append(evaluations, evaluation)
	}

	sort.Slice(evaluations, func(i, j int) bool {
		if evaluations[i].eligible != evaluations[j].eligible {
			return evaluations[i].eligible
		}
		if evaluations[i].eligible && abs(evaluations[i].score-evaluations[j].score) > tieScoreEpsilon {
			return evaluations[i].score > evaluations[j].score
		}
		return modelTieBreakKey(evaluations[i].model) < modelTieBreakKey(evaluations[j].model)
	})

	return evaluations
}

func (e *Engine) isRateLimitedLocked(modelID string, now time.Time) bool {
	if t, ok := e.rateLimits[modelID]; ok {
		if now.Before(t) {
			return true
		}
		delete(e.rateLimits, modelID)
	}
	return false
}

func (e *Engine) withinCapacityLocked(model *ModelConfig, estimatedTokens int, now time.Time) bool {
	e.pruneUsageLocked(model.ID, now)
	records := e.usageRecords[model.ID]

	if model.MaxRPM > 0 && len(records) >= model.MaxRPM {
		return false
	}

	if model.MaxTPM > 0 {
		totalTokens := 0
		for _, record := range records {
			totalTokens += record.tokens
		}
		if totalTokens+estimatedTokens > model.MaxTPM {
			return false
		}
	}

	return true
}

func (e *Engine) pruneUsageLocked(modelID string, now time.Time) {
	records := e.usageRecords[modelID]
	if len(records) == 0 {
		return
	}

	keep := records[:0]
	for _, record := range records {
		if now.Sub(record.timestamp) < usageWindow {
			keep = append(keep, record)
		}
	}

	if len(keep) == 0 {
		delete(e.usageRecords, modelID)
		return
	}

	e.usageRecords[modelID] = keep
}

func (e *Engine) reserveUsage(modelID string, estimatedTokens int) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	e.pruneUsageLocked(modelID, now)
	requestID := uuid.NewString()
	e.usageRecords[modelID] = append(e.usageRecords[modelID], usageRecord{
		requestID: requestID,
		timestamp: now,
		tokens:    estimatedTokens,
	})
	return requestID
}

func (e *Engine) reconcileUsage(modelID, requestID string, actualTokens int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	records := e.usageRecords[modelID]
	for i := range records {
		if records[i].requestID != requestID {
			continue
		}
		if actualTokens == 0 {
			records = append(records[:i], records[i+1:]...)
			if len(records) == 0 {
				delete(e.usageRecords, modelID)
			} else {
				e.usageRecords[modelID] = records
			}
			return
		}
		records[i].tokens = actualTokens
		e.usageRecords[modelID] = records
		return
	}
}

func (e *Engine) markRateLimited(modelID string, until time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rateLimits[modelID] = until
}

func (e *Engine) observeStream(modelID, requestID string, estimate tokenEstimate, stream <-chan provider.StreamChunk) <-chan provider.StreamChunk {
	out := make(chan provider.StreamChunk, 64)
	go func() {
		defer close(out)
		actualTokens := estimate.promptTokens
		usageReported := false
		for chunk := range stream {
			if chunk.Error != nil && isRateLimitError(chunk.Error) {
				e.markRateLimited(modelID, time.Now().Add(providerRateLimitBackoff))
			}
			if chunk.Content != "" && !usageReported {
				actualTokens += estimateContentTokens(chunk.Content)
			}
			if chunk.Usage != nil {
				actualTokens = chunk.Usage.InputTokens + chunk.Usage.OutputTokens
				usageReported = true
			}
			out <- chunk
		}
		e.reconcileUsage(modelID, requestID, actualTokens)
	}()
	return out
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

func messagesForClassification(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "user" {
			continue
		}
		text := strings.TrimSpace(msgs[i].ExtractText())
		if text != "" {
			return text
		}
	}
	return strings.TrimSpace(messagesToString(msgs))
}

func estimateRequestTokens(req *RouteRequest) tokenEstimate {
	if req == nil {
		return tokenEstimate{
			promptTokens: 1,
			totalTokens:  defaultCompletionTokenReserve + 1,
		}
	}

	promptTokens := 0
	for i := range req.Messages {
		text := req.Messages[i].ExtractText()
		if text == "" {
			continue
		}
		promptTokens += len([]rune(text)) / 4
	}
	if promptTokens < 1 {
		promptTokens = 1
	}

	completionTokens := req.MaxTokens
	if completionTokens <= 0 {
		completionTokens = defaultCompletionTokenReserve
	}

	return tokenEstimate{
		promptTokens: promptTokens,
		totalTokens:  promptTokens + completionTokens,
	}
}

func estimateContentTokens(content string) int {
	tokens := len([]rune(content)) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") || strings.Contains(msg, "rate limit")
}

func modelTieBreakKey(model *ModelConfig) string {
	return fmt.Sprintf("%s|%s|%s", model.Name, model.ModelID, model.ID)
}

func primeStream(stream <-chan provider.StreamChunk) ([]provider.StreamChunk, bool) {
	for chunk := range stream {
		if chunk.Error != nil {
			return []provider.StreamChunk{chunk}, true
		}
		if chunk.Content != "" || chunk.Done {
			return []provider.StreamChunk{chunk}, false
		}
	}

	return []provider.StreamChunk{{
		Error: fmt.Errorf("stream ended before first content"),
		Done:  true,
	}}, true
}

func prependStream(buffered []provider.StreamChunk, stream <-chan provider.StreamChunk) <-chan provider.StreamChunk {
	out := make(chan provider.StreamChunk, 64)
	go func() {
		defer close(out)
		for _, chunk := range buffered {
			out <- chunk
		}
		for chunk := range stream {
			out <- chunk
		}
	}()
	return out
}

func bufferedStream(chunks []provider.StreamChunk) <-chan provider.StreamChunk {
	out := make(chan provider.StreamChunk, len(chunks))
	go func() {
		defer close(out)
		for _, chunk := range chunks {
			out <- chunk
		}
	}()
	return out
}

func chunkErrorMessage(chunks []provider.StreamChunk) string {
	for _, chunk := range chunks {
		if chunk.Error != nil {
			return chunk.Error.Error()
		}
	}
	return "stream ended before first content"
}

func eligibleModels(evaluations []candidateEvaluation) []*ModelConfig {
	models := make([]*ModelConfig, 0, len(evaluations))
	for _, evaluation := range evaluations {
		if evaluation.eligible {
			models = append(models, evaluation.model)
		}
	}
	return models
}

func diagnosticsFromEvaluations(mode, classificationInput string, classResult *ClassificationResult, estimatedTokens int, evaluations []candidateEvaluation) *RouteDiagnostics {
	diagnostics := &RouteDiagnostics{
		Mode:                mode,
		ClassificationInput: classificationInput,
		EstimatedTokens:     estimatedTokens,
	}
	if classResult != nil {
		diagnostics.ClassificationMethod = classResult.Method
		diagnostics.Complexity = classResult.Complexity.String()
		diagnostics.TaskType = classResult.TaskType.String()
	}

	for _, evaluation := range evaluations {
		reason := evaluation.reason
		if reason == "" && evaluation.profile != "" {
			reason = evaluation.profile
		}
		candidate := CandidateDiagnostic{
			Name:      evaluation.model.Name,
			ModelID:   evaluation.model.ModelID,
			Provider:  evaluation.model.Provider,
			Eligible:  evaluation.eligible,
			Score:     evaluation.score,
			Reason:    reason,
			RecentRPM: evaluation.recentRPM,
			RecentTPM: evaluation.recentTPM,
			MaxRPM:    evaluation.model.MaxRPM,
			MaxTPM:    evaluation.model.MaxTPM,
		}
		if evaluation.eligible {
			candidate.Decision = "eligible"
		} else {
			candidate.Decision = "skipped"
		}
		diagnostics.Candidates = append(diagnostics.Candidates, candidate)
	}

	diagnostics.Summary = diagnosticsSummary(diagnostics)
	return diagnostics
}

func raceDiagnostics(req *RouteRequest, candidates []*ModelConfig) *RouteDiagnostics {
	diagnostics := &RouteDiagnostics{
		Mode:            RouteRace.String(),
		EstimatedTokens: estimateRequestTokens(req).totalTokens,
	}
	for _, candidate := range candidates {
		diagnostics.Candidates = append(diagnostics.Candidates, CandidateDiagnostic{
			Name:     candidate.Name,
			ModelID:  candidate.ModelID,
			Provider: candidate.Provider,
			Eligible: true,
			Decision: "eligible",
		})
	}
	diagnostics.Summary = diagnosticsSummary(diagnostics)
	return diagnostics
}

func markDiagnosticDecision(candidates []CandidateDiagnostic, modelName, decision, reason string) []CandidateDiagnostic {
	if modelName == "" {
		return candidates
	}
	for i := range candidates {
		if candidates[i].Name != modelName {
			continue
		}
		candidates[i].Decision = decision
		if reason != "" {
			candidates[i].Reason = reason
		}
		return candidates
	}
	return candidates
}

func diagnosticsSummary(diagnostics *RouteDiagnostics) string {
	if diagnostics == nil {
		return ""
	}
	eligible := 0
	skipped := 0
	for _, candidate := range diagnostics.Candidates {
		if candidate.Eligible {
			eligible++
		} else {
			skipped++
		}
	}

	parts := []string{fmt.Sprintf("mode=%s", diagnostics.Mode)}
	if diagnostics.Complexity != "" {
		parts = append(parts, fmt.Sprintf("complexity=%s", diagnostics.Complexity))
	}
	if diagnostics.TaskType != "" {
		parts = append(parts, fmt.Sprintf("task=%s", diagnostics.TaskType))
	}
	if diagnostics.ClassificationMethod != "" {
		parts = append(parts, fmt.Sprintf("method=%s", diagnostics.ClassificationMethod))
	}
	if diagnostics.SelectedModel != "" {
		parts = append(parts, fmt.Sprintf("selected=%s", diagnostics.SelectedModel))
	}
	if diagnostics.FallbackUsed {
		parts = append(parts, "fallback=true")
	}
	parts = append(parts, fmt.Sprintf("eligible=%d", eligible))
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("skipped=%d", skipped))
	}
	if diagnostics.EstimatedTokens > 0 {
		parts = append(parts, fmt.Sprintf("estimated_tokens=%d", diagnostics.EstimatedTokens))
	}
	return strings.Join(parts, "; ")
}

func unavailableModelError(evaluations []candidateEvaluation) string {
	if len(evaluations) == 0 {
		return "no available model"
	}

	reasons := make([]string, 0, len(evaluations))
	for _, evaluation := range evaluations {
		if evaluation.eligible {
			continue
		}

		name := evaluation.model.Name
		if name == "" {
			name = evaluation.model.ModelID
		}
		reason := evaluation.reason
		if reason == "" {
			reason = "filtered out"
		}
		reasons = append(reasons, fmt.Sprintf("%s: %s", name, reason))
	}

	if len(reasons) == 0 {
		return "no available model"
	}

	return "no available model: " + strings.Join(reasons, "; ")
}

func candidateNameByID(candidates []*ModelConfig, modelID string) string {
	for _, candidate := range candidates {
		if candidate.ID == modelID {
			return candidate.Name
		}
	}
	return ""
}

func errorMessage(result *RouteResult) string {
	if result == nil {
		return "route result unavailable"
	}
	if result.ErrorMsg != "" {
		return result.ErrorMsg
	}
	return "request failed"
}

func (d *RouteDiagnostics) HeaderSummary() string {
	if d == nil {
		return ""
	}
	summary := strings.ReplaceAll(d.Summary, "\n", " ")
	summary = strings.ReplaceAll(summary, "\r", " ")
	if len(summary) > 240 {
		summary = summary[:240]
	}
	return summary
}

func (d *RouteDiagnostics) ToJSON() string {
	if d == nil {
		return ""
	}
	data, err := json.Marshal(d)
	if err != nil {
		return ""
	}
	return string(data)
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
