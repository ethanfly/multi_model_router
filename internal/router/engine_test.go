package router

import (
	"context"
	"testing"

	"multi_model_router/internal/provider"
)

func TestSelectModel_SimplePrefersSpeed(t *testing.T) {
	e := NewEngine(NewClassifier(nil))

	fastModel := &ModelConfig{
		ID:            "fast",
		Name:          "Fast Model",
		Provider:      "test",
		ModelID:       "fast-v1",
		Reasoning:     3,
		Coding:        3,
		Creativity:    3,
		Speed:         9,
		CostEfficiency: 5,
		IsActive:      true,
	}

	smartModel := &ModelConfig{
		ID:            "smart",
		Name:          "Smart Model",
		Provider:      "test",
		ModelID:       "smart-v1",
		Reasoning:     9,
		Coding:        3,
		Creativity:    3,
		Speed:         3,
		CostEfficiency: 5,
		IsActive:      true,
	}

	e.SetModels([]*ModelConfig{fastModel, smartModel})

	selected := e.selectModel(Simple)
	if selected == nil {
		t.Fatal("expected a model to be selected, got nil")
	}
	if selected.ID != "fast" {
		t.Errorf("expected fast model for Simple, got %s", selected.ID)
	}
}

func TestSelectModel_ComplexPrefersReasoning(t *testing.T) {
	e := NewEngine(NewClassifier(nil))

	fastModel := &ModelConfig{
		ID:            "fast",
		Name:          "Fast Model",
		Provider:      "test",
		ModelID:       "fast-v1",
		Reasoning:     3,
		Coding:        3,
		Creativity:    3,
		Speed:         9,
		CostEfficiency: 5,
		IsActive:      true,
	}

	smartModel := &ModelConfig{
		ID:            "smart",
		Name:          "Smart Model",
		Provider:      "test",
		ModelID:       "smart-v1",
		Reasoning:     9,
		Coding:        3,
		Creativity:    3,
		Speed:         3,
		CostEfficiency: 5,
		IsActive:      true,
	}

	e.SetModels([]*ModelConfig{fastModel, smartModel})

	selected := e.selectModel(Complex)
	if selected == nil {
		t.Fatal("expected a model to be selected, got nil")
	}
	if selected.ID != "smart" {
		t.Errorf("expected smart model for Complex, got %s", selected.ID)
	}
}

func TestSelectModel_SkipsInactive(t *testing.T) {
	e := NewEngine(NewClassifier(nil))

	inactiveModel := &ModelConfig{
		ID:            "inactive",
		Name:          "Inactive Model",
		Provider:      "test",
		ModelID:       "inactive-v1",
		Reasoning:     9,
		Coding:        9,
		Creativity:    9,
		Speed:         9,
		CostEfficiency: 9,
		IsActive:      false,
	}

	e.SetModels([]*ModelConfig{inactiveModel})

	selected := e.selectModel(Simple)
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

func (m *mockProvider) HealthCheck(ctx context.Context) error {
	return nil
}

func TestRouteAuto_Integration(t *testing.T) {
	e := NewEngine(NewClassifier(nil))
	e.AddProvider("test", &mockProvider{})

	model := &ModelConfig{
		ID:            "test-model",
		Name:          "Test Model",
		Provider:      "test",
		ModelID:       "test-v1",
		Reasoning:     7,
		Coding:        7,
		Creativity:    7,
		Speed:         7,
		CostEfficiency: 7,
		IsActive:      true,
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
