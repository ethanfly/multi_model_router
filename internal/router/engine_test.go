package router

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"multi_model_router/internal/provider"
)

func TestSelectModel_SimplePrefersCostEfficiency(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	fastModel := &ModelConfig{
		ID:             "fast",
		Name:           "Fast Model",
		Provider:       "test",
		ModelID:        "fast-v1",
		Reasoning:      4,
		Coding:         4,
		Creativity:     4,
		Speed:          9,
		CostEfficiency: 3,
		IsActive:       true,
	}

	budgetModel := &ModelConfig{
		ID:             "budget",
		Name:           "Budget Model",
		Provider:       "test",
		ModelID:        "budget-v1",
		Reasoning:      4,
		Coding:         4,
		Creativity:     4,
		Speed:          5,
		CostEfficiency: 9,
		IsActive:       true,
	}

	e.SetModels([]*ModelConfig{fastModel, budgetModel})

	selected := e.selectModel(Simple, &RouteRequest{})
	if selected == nil {
		t.Fatal("expected a model to be selected, got nil")
	}
	if selected.ID != "budget" {
		t.Errorf("expected budget model for Simple, got %s", selected.ID)
	}
}

func TestSelectModel_MediumPrefersCostEfficientBalancedModel(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	premiumModel := &ModelConfig{
		ID:             "premium",
		Name:           "Premium Model",
		Provider:       "test",
		ModelID:        "premium-v1",
		Reasoning:      8,
		Coding:         8,
		Creativity:     5,
		Speed:          6,
		CostEfficiency: 3,
		IsActive:       true,
	}

	balancedModel := &ModelConfig{
		ID:             "balanced",
		Name:           "Balanced Model",
		Provider:       "test",
		ModelID:        "balanced-v1",
		Reasoning:      7,
		Coding:         7,
		Creativity:     5,
		Speed:          6,
		CostEfficiency: 9,
		IsActive:       true,
	}

	e.SetModels([]*ModelConfig{premiumModel, balancedModel})

	selected := e.selectModel(Medium, &RouteRequest{})
	if selected == nil {
		t.Fatal("expected a model to be selected, got nil")
	}
	if selected.ID != "balanced" {
		t.Errorf("expected balanced model for Medium, got %s", selected.ID)
	}
}

func TestSelectModel_ComplexPrefersReasoning(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	fastModel := &ModelConfig{
		ID:             "fast",
		Name:           "Fast Model",
		Provider:       "test",
		ModelID:        "fast-v1",
		Reasoning:      3,
		Coding:         3,
		Creativity:     3,
		Speed:          9,
		CostEfficiency: 5,
		IsActive:       true,
	}

	smartModel := &ModelConfig{
		ID:             "smart",
		Name:           "Smart Model",
		Provider:       "test",
		ModelID:        "smart-v1",
		Reasoning:      9,
		Coding:         3,
		Creativity:     3,
		Speed:          3,
		CostEfficiency: 5,
		IsActive:       true,
	}

	e.SetModels([]*ModelConfig{fastModel, smartModel})

	selected := e.selectModel(Complex, &RouteRequest{})
	if selected == nil {
		t.Fatal("expected a model to be selected, got nil")
	}
	if selected.ID != "smart" {
		t.Errorf("expected smart model for Complex, got %s", selected.ID)
	}
}

func TestSelectModel_SkipsInactive(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	inactiveModel := &ModelConfig{
		ID:             "inactive",
		Name:           "Inactive Model",
		Provider:       "test",
		ModelID:        "inactive-v1",
		Reasoning:      9,
		Coding:         9,
		Creativity:     9,
		Speed:          9,
		CostEfficiency: 9,
		IsActive:       false,
	}

	e.SetModels([]*ModelConfig{inactiveModel})

	selected := e.selectModel(Simple, &RouteRequest{})
	if selected != nil {
		t.Errorf("expected nil for inactive-only models, got %s", selected.ID)
	}
}

// mockProvider implements provider.Provider for testing.
type mockProvider struct{}

func (m *mockProvider) ChatCompletion(ctx context.Context, req *provider.ChatRequest) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 2)
	ch <- provider.StreamChunk{
		Content: "mock response",
		Done:    false,
	}
	ch <- provider.StreamChunk{
		Content: "",
		Done:    true,
		Usage: &provider.Usage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}
	close(ch)
	return ch, nil
}

func (m *mockProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{}, nil
}

func (m *mockProvider) HealthCheck(ctx context.Context, modelID string) error {
	return nil
}

type failingStreamProvider struct {
	err error
}

func (p *failingStreamProvider) ChatCompletion(ctx context.Context, req *provider.ChatRequest) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 1)
	ch <- provider.StreamChunk{Error: p.err, Done: true, Model: req.Model}
	close(ch)
	return ch, nil
}

func (p *failingStreamProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return nil, nil
}

func (p *failingStreamProvider) HealthCheck(ctx context.Context, modelID string) error {
	return nil
}

type usageReportingProvider struct {
	content string
	usage   provider.Usage
}

func (p *usageReportingProvider) ChatCompletion(ctx context.Context, req *provider.ChatRequest) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 3)
	ch <- provider.StreamChunk{Content: p.content, Model: req.Model}
	ch <- provider.StreamChunk{Usage: &provider.Usage{
		InputTokens:  p.usage.InputTokens,
		OutputTokens: p.usage.OutputTokens,
	}, Model: req.Model}
	ch <- provider.StreamChunk{Done: true, Model: req.Model}
	close(ch)
	return ch, nil
}

func (p *usageReportingProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return nil, nil
}

func (p *usageReportingProvider) HealthCheck(ctx context.Context, modelID string) error {
	return nil
}

type delayedSuccessProvider struct {
	delay   time.Duration
	content string
}

func (p *delayedSuccessProvider) ChatCompletion(ctx context.Context, req *provider.ChatRequest) (<-chan provider.StreamChunk, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(p.delay):
	}

	ch := make(chan provider.StreamChunk, 2)
	ch <- provider.StreamChunk{Content: p.content, Model: req.Model}
	ch <- provider.StreamChunk{Done: true, Model: req.Model}
	close(ch)
	return ch, nil
}

func (p *delayedSuccessProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return nil, nil
}

func (p *delayedSuccessProvider) HealthCheck(ctx context.Context, modelID string) error {
	return nil
}

func TestRouteAuto_Integration(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	model := &ModelConfig{
		ID:               "test-model",
		Name:             "Test Model",
		Provider:         "test",
		ModelID:          "test-v1",
		Reasoning:        7,
		Coding:           7,
		Creativity:       7,
		Speed:            7,
		CostEfficiency:   7,
		IsActive:         true,
		ProviderInstance: &mockProvider{},
	}
	e.SetModels([]*ModelConfig{model})

	req := &RouteRequest{
		Messages: []provider.Message{
			{Role: "user", Content: "翻译这段话"},
		},
		Mode: RouteAuto,
	}

	result := e.Route(context.Background(), req)
	if result.Status != "success" {
		t.Errorf("expected success, got %s: %s", result.Status, result.ErrorMsg)
	}

	// "翻译这段话" should be classified as Simple (complexity=0)
	if result.Complexity != int64(Simple) {
		t.Errorf("expected complexity=Simple(0), got %d", result.Complexity)
	}

	// Collect stream content
	content, _, err := provider.CollectStream(result.Stream)
	if err != nil {
		t.Fatalf("error collecting stream: %v", err)
	}
	if content != "mock response" {
		t.Errorf("expected 'mock response', got %q", content)
	}
}

func TestRouteAuto_FallsBackWhenPrimaryStreamFailsBeforeContent(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	primary := &ModelConfig{
		ID:               "primary",
		Name:             "Primary",
		Provider:         "test",
		ModelID:          "primary-v1",
		Reasoning:        9,
		Coding:           9,
		Creativity:       6,
		Speed:            6,
		CostEfficiency:   4,
		IsActive:         true,
		ProviderInstance: &failingStreamProvider{err: fmt.Errorf("stream read error: upstream reset")},
	}

	fallback := &ModelConfig{
		ID:               "fallback",
		Name:             "Fallback",
		Provider:         "test",
		ModelID:          "fallback-v1",
		Reasoning:        8,
		Coding:           8,
		Creativity:       6,
		Speed:            5,
		CostEfficiency:   5,
		IsActive:         true,
		ProviderInstance: &mockProvider{},
	}

	e.SetModels([]*ModelConfig{primary, fallback})

	result := e.Route(context.Background(), &RouteRequest{
		Messages: []provider.Message{{Role: "user", Content: "请帮我设计一个高并发服务架构"}},
		Mode:     RouteAuto,
	})

	if result == nil || result.Status != "success" {
		t.Fatalf("expected success with fallback, got %#v", result)
	}
	if result.ModelName != "Fallback" {
		t.Fatalf("expected fallback model to be surfaced, got %q", result.ModelName)
	}

	content, _, err := provider.CollectStream(result.Stream)
	if err != nil {
		t.Fatalf("unexpected stream error after fallback: %v", err)
	}
	if content != "mock response" {
		t.Fatalf("expected fallback content, got %q", content)
	}
}

func TestSelectModel_SkipsModelsAtRPMCapacity(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	limited := &ModelConfig{
		ID:             "limited",
		Name:           "Limited",
		Provider:       "test",
		ModelID:        "limited-v1",
		Reasoning:      9,
		Coding:         9,
		Creativity:     5,
		Speed:          5,
		CostEfficiency: 5,
		MaxRPM:         1,
		IsActive:       true,
	}

	backup := &ModelConfig{
		ID:             "backup",
		Name:           "Backup",
		Provider:       "test",
		ModelID:        "backup-v1",
		Reasoning:      8,
		Coding:         8,
		Creativity:     5,
		Speed:          5,
		CostEfficiency: 5,
		IsActive:       true,
	}

	e.SetModels([]*ModelConfig{limited, backup})
	e.reserveUsage("limited", 100)

	selected := e.selectModel(Complex, &RouteRequest{})
	if selected == nil {
		t.Fatal("expected backup model to be selected")
	}
	if selected.ID != "backup" {
		t.Fatalf("expected backup model when limited model is at RPM capacity, got %s", selected.ID)
	}
}

func TestSelectModel_SkipsModelsAtTPMCapacity(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	limited := &ModelConfig{
		ID:             "limited",
		Name:           "Limited",
		Provider:       "test",
		ModelID:        "limited-v1",
		Reasoning:      9,
		Coding:         9,
		Creativity:     5,
		Speed:          5,
		CostEfficiency: 5,
		MaxTPM:         600,
		IsActive:       true,
	}

	backup := &ModelConfig{
		ID:             "backup",
		Name:           "Backup",
		Provider:       "test",
		ModelID:        "backup-v1",
		Reasoning:      8,
		Coding:         8,
		Creativity:     5,
		Speed:          5,
		CostEfficiency: 5,
		IsActive:       true,
	}

	e.SetModels([]*ModelConfig{limited, backup})
	e.reserveUsage("limited", 550)

	selected := e.selectModel(Complex, &RouteRequest{
		Messages:  []provider.Message{{Role: "user", Content: "请帮我重构这个大型系统"}},
		MaxTokens: 200,
	})
	if selected == nil {
		t.Fatal("expected backup model to be selected")
	}
	if selected.ID != "backup" {
		t.Fatalf("expected backup model when limited model is at TPM capacity, got %s", selected.ID)
	}
}

func TestSelectModel_TieBreakIsStable(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	alpha := &ModelConfig{
		ID:             "b-id",
		Name:           "Alpha",
		Provider:       "test",
		ModelID:        "alpha-v1",
		Reasoning:      7,
		Coding:         7,
		Creativity:     7,
		Speed:          7,
		CostEfficiency: 7,
		IsActive:       true,
	}

	beta := &ModelConfig{
		ID:             "a-id",
		Name:           "Beta",
		Provider:       "test",
		ModelID:        "beta-v1",
		Reasoning:      7,
		Coding:         7,
		Creativity:     7,
		Speed:          7,
		CostEfficiency: 7,
		IsActive:       true,
	}

	e.SetModels([]*ModelConfig{alpha, beta})

	for i := 0; i < 50; i++ {
		selected := e.selectModel(Medium, &RouteRequest{})
		if selected == nil {
			t.Fatal("expected a model to be selected")
		}
		if selected.Name != "Alpha" {
			t.Fatalf("expected deterministic tie-break to prefer Alpha, got %s", selected.Name)
		}
	}
}

func TestRouteAuto_ClassifiesUsingLatestUserTurns(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	fastModel := &ModelConfig{
		ID:               "fast",
		Name:             "Fast",
		Provider:         "test",
		ModelID:          "fast-v1",
		Reasoning:        3,
		Coding:           3,
		Creativity:       3,
		Speed:            9,
		CostEfficiency:   8,
		IsActive:         true,
		ProviderInstance: &mockProvider{},
	}

	smartModel := &ModelConfig{
		ID:               "smart",
		Name:             "Smart",
		Provider:         "test",
		ModelID:          "smart-v1",
		Reasoning:        9,
		Coding:           9,
		Creativity:       6,
		Speed:            4,
		CostEfficiency:   4,
		IsActive:         true,
		ProviderInstance: &mockProvider{},
	}

	e.SetModels([]*ModelConfig{fastModel, smartModel})

	result := e.Route(context.Background(), &RouteRequest{
		Messages: []provider.Message{
			{Role: "system", Content: "You are a coding assistant."},
			{Role: "user", Content: "请帮我设计一个高并发微服务架构，并分析扩容、缓存、容灾和一致性策略。"},
			{Role: "assistant", Content: "这里是一段很长的复杂分析，包含 architecture, optimize, trade-off, database, benchmark 等关键词。"},
			{Role: "user", Content: "hello"},
		},
		Mode: RouteAuto,
	})

	if result == nil || result.Status != "success" {
		t.Fatalf("expected success, got %#v", result)
	}
	if result.ModelName != "Fast" {
		t.Fatalf("expected latest user turn to route to fast model, got %q", result.ModelName)
	}
}

func TestUsageReconciliation_UsesActualUsageAfterStreamCompletes(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	model := &ModelConfig{
		ID:               "reconciled",
		Name:             "Reconciled",
		Provider:         "test",
		ModelID:          "reconciled-v1",
		Reasoning:        9,
		Coding:           9,
		Creativity:       6,
		Speed:            6,
		CostEfficiency:   5,
		MaxTPM:           200,
		IsActive:         true,
		ProviderInstance: &usageReportingProvider{content: "ok", usage: provider.Usage{InputTokens: 20, OutputTokens: 10}},
	}

	e.SetModels([]*ModelConfig{model})

	req := &RouteRequest{
		Messages:  []provider.Message{{Role: "user", Content: "Please design a service."}},
		Mode:      RouteAuto,
		MaxTokens: 40,
	}

	result := e.Route(context.Background(), req)
	if result == nil || result.Status != "success" {
		t.Fatalf("expected success, got %#v", result)
	}
	if _, _, err := provider.CollectStream(result.Stream); err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	selected := e.selectModel(Complex, req)
	if selected == nil {
		t.Fatal("expected model to remain selectable after usage reconciliation")
	}
	if selected.ID != "reconciled" {
		t.Fatalf("expected reconciled model to still be selectable, got %s", selected.ID)
	}
}

func TestExplainSelection_IncludesSkipReasons(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	limited := &ModelConfig{
		ID:             "limited",
		Name:           "Limited",
		Provider:       "test",
		ModelID:        "limited-v1",
		Reasoning:      9,
		Coding:         9,
		Creativity:     6,
		Speed:          5,
		CostEfficiency: 4,
		MaxTPM:         100,
		IsActive:       true,
	}

	available := &ModelConfig{
		ID:             "available",
		Name:           "Available",
		Provider:       "test",
		ModelID:        "available-v1",
		Reasoning:      8,
		Coding:         8,
		Creativity:     6,
		Speed:          6,
		CostEfficiency: 5,
		IsActive:       true,
	}

	e.SetModels([]*ModelConfig{limited, available})
	e.reserveUsage("limited", 90)

	selection := e.ExplainSelection(context.Background(), &RouteRequest{
		Messages:  []provider.Message{{Role: "user", Content: "please design a distributed system"}},
		Mode:      RouteAuto,
		MaxTokens: 40,
	})

	if selection == nil || selection.Model == nil {
		t.Fatalf("expected a selected model, got %#v", selection)
	}
	if selection.Model.Name != "Available" {
		t.Fatalf("expected available model, got %q", selection.Model.Name)
	}
	if selection.Diagnostics == nil {
		t.Fatal("expected diagnostics to be present")
	}

	foundSkipped := false
	for _, candidate := range selection.Diagnostics.Candidates {
		if candidate.Name != "Limited" {
			continue
		}
		foundSkipped = true
		if candidate.Decision != "skipped" {
			t.Fatalf("expected limited candidate to be skipped, got %q", candidate.Decision)
		}
		if candidate.Reason == "" {
			t.Fatal("expected skip reason for limited candidate")
		}
	}
	if !foundSkipped {
		t.Fatal("expected limited candidate diagnostics to be present")
	}
	if selection.Diagnostics.Summary == "" {
		t.Fatal("expected diagnostics summary")
	}
}

func TestExplainSelection_NoAvailableModelIncludesCandidateReasons(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	limited := &ModelConfig{
		ID:             "limited",
		Name:           "Limited",
		Provider:       "test",
		ModelID:        "limited-v1",
		Reasoning:      9,
		Coding:         9,
		Creativity:     6,
		Speed:          5,
		CostEfficiency: 4,
		MaxTPM:         100,
		IsActive:       true,
	}

	inactive := &ModelConfig{
		ID:             "inactive",
		Name:           "Inactive",
		Provider:       "test",
		ModelID:        "inactive-v1",
		Reasoning:      8,
		Coding:         8,
		Creativity:     6,
		Speed:          6,
		CostEfficiency: 5,
		IsActive:       false,
	}

	e.SetModels([]*ModelConfig{limited, inactive})
	e.reserveUsage("limited", 90)

	selection := e.ExplainSelection(context.Background(), &RouteRequest{
		Messages:  []provider.Message{{Role: "user", Content: "please design a distributed system"}},
		Mode:      RouteAuto,
		MaxTokens: 40,
	})

	if selection == nil {
		t.Fatal("expected selection result")
	}
	if selection.Model != nil {
		t.Fatalf("expected no selected model, got %q", selection.Model.Name)
	}
	if selection.ErrorMsg == "" {
		t.Fatal("expected no available model error")
	}
	if !strings.Contains(selection.ErrorMsg, "Limited: tpm capacity exceeded") {
		t.Fatalf("expected TPM reason in error, got %q", selection.ErrorMsg)
	}
	if !strings.Contains(selection.ErrorMsg, "Inactive: inactive") {
		t.Fatalf("expected inactive reason in error, got %q", selection.ErrorMsg)
	}
}

func TestRouteAuto_NoAvailableModelIncludesCandidateReasons(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	limited := &ModelConfig{
		ID:             "limited",
		Name:           "Limited",
		Provider:       "test",
		ModelID:        "limited-v1",
		Reasoning:      9,
		Coding:         9,
		Creativity:     6,
		Speed:          5,
		CostEfficiency: 4,
		MaxTPM:         100,
		IsActive:       true,
	}

	inactive := &ModelConfig{
		ID:             "inactive",
		Name:           "Inactive",
		Provider:       "test",
		ModelID:        "inactive-v1",
		Reasoning:      8,
		Coding:         8,
		Creativity:     6,
		Speed:          6,
		CostEfficiency: 5,
		IsActive:       false,
	}

	e.SetModels([]*ModelConfig{limited, inactive})
	e.reserveUsage("limited", 90)

	result := e.Route(context.Background(), &RouteRequest{
		Messages:  []provider.Message{{Role: "user", Content: "please design a distributed system"}},
		Mode:      RouteAuto,
		MaxTokens: 40,
	})

	if result == nil {
		t.Fatal("expected route result")
	}
	if result.Status != "error" {
		t.Fatalf("expected error, got %#v", result)
	}
	if !strings.Contains(result.ErrorMsg, "Limited: tpm capacity exceeded") {
		t.Fatalf("expected TPM reason in error, got %q", result.ErrorMsg)
	}
	if !strings.Contains(result.ErrorMsg, "Inactive: inactive") {
		t.Fatalf("expected inactive reason in error, got %q", result.ErrorMsg)
	}
}

func TestRouteRace_IgnoresEarlyFailingStreamAndReturnsFirstValidContent(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	bad := &ModelConfig{
		ID:               "bad",
		Name:             "Bad",
		Provider:         "test",
		ModelID:          "bad-v1",
		Reasoning:        6,
		Coding:           6,
		Creativity:       6,
		Speed:            6,
		CostEfficiency:   6,
		IsActive:         true,
		ProviderInstance: &failingStreamProvider{err: fmt.Errorf("upstream reset before first token")},
	}

	good := &ModelConfig{
		ID:               "good",
		Name:             "Good",
		Provider:         "test",
		ModelID:          "good-v1",
		Reasoning:        6,
		Coding:           6,
		Creativity:       6,
		Speed:            6,
		CostEfficiency:   6,
		IsActive:         true,
		ProviderInstance: &delayedSuccessProvider{delay: 15 * time.Millisecond, content: "winner"},
	}

	e.SetModels([]*ModelConfig{bad, good})

	result := e.Route(context.Background(), &RouteRequest{
		Messages:  []provider.Message{{Role: "user", Content: "hello"}},
		Mode:      RouteRace,
		MaxTokens: 32,
	})

	if result == nil || result.Status != "success" {
		t.Fatalf("expected race success, got %#v", result)
	}
	if result.ModelName != "Good" {
		t.Fatalf("expected valid stream winner, got %q", result.ModelName)
	}

	content, _, err := provider.CollectStream(result.Stream)
	if err != nil {
		t.Fatalf("unexpected winner stream error: %v", err)
	}
	if content != "winner" {
		t.Fatalf("expected winner content, got %q", content)
	}
}

func TestRouteRace_SkipsModelsAtCapacity(t *testing.T) {
	e := NewEngine(NewClassifier(nil, nil))

	limited := &ModelConfig{
		ID:               "limited",
		Name:             "Limited",
		Provider:         "test",
		ModelID:          "limited-v1",
		Reasoning:        8,
		Coding:           8,
		Creativity:       6,
		Speed:            6,
		CostEfficiency:   6,
		MaxTPM:           100,
		IsActive:         true,
		ProviderInstance: &mockProvider{},
	}

	available := &ModelConfig{
		ID:               "available",
		Name:             "Available",
		Provider:         "test",
		ModelID:          "available-v1",
		Reasoning:        7,
		Coding:           7,
		Creativity:       6,
		Speed:            6,
		CostEfficiency:   6,
		IsActive:         true,
		ProviderInstance: &mockProvider{},
	}

	e.SetModels([]*ModelConfig{limited, available})
	e.reserveUsage("limited", 90)

	result := e.Route(context.Background(), &RouteRequest{
		Messages:  []provider.Message{{Role: "user", Content: "hello"}},
		Mode:      RouteRace,
		MaxTokens: 40,
	})

	if result == nil || result.Status != "success" {
		t.Fatalf("expected race success, got %#v", result)
	}
	if result.ModelName != "Available" {
		t.Fatalf("expected at-capacity model to be skipped, got %q", result.ModelName)
	}
}
