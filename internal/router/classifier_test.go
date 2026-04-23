package router

import (
	"context"
	"testing"
)

func TestClassifyByRules_SimpleTranslation(t *testing.T) {
	c := NewClassifier(nil)
	result := c.classifyByRules("翻译这段话")
	if result.Complexity != Simple {
		t.Errorf("expected Simple, got %v", result.Complexity)
	}
}

func TestClassifyByRules_SimpleShort(t *testing.T) {
	c := NewClassifier(nil)
	result := c.classifyByRules("What is Python?")
	if result.Complexity != Simple {
		t.Errorf("expected Simple, got %v", result.Complexity)
	}
}

func TestClassifyByRules_ComplexArchitecture(t *testing.T) {
	c := NewClassifier(nil)
	question := "请帮我设计一个高并发的微服务架构系统，需要考虑服务发现、负载均衡、熔断降级、分布式追踪等多个方面，并且需要支持千万级用户同时在线，请问应该如何设计和优化整个系统的架构？"
	result := c.classifyByRules(question)
	if result.Complexity != Complex {
		t.Errorf("expected Complex, got %v (score details: confidence=%v)", result.Complexity, result.Confidence)
	}
}

func TestClassifyByRules_ComplexWithCode(t *testing.T) {
	c := NewClassifier(nil)
	question := "请帮我优化架构设计，以下是代码：```go\nfunc main() {}\n``` 和 ```python\ndef hello(): pass\n```"
	result := c.classifyByRules(question)
	if result.Complexity != Complex {
		t.Errorf("expected Complex, got %v", result.Complexity)
	}
}

func TestClassifyByRules_Medium(t *testing.T) {
	c := NewClassifier(nil)
	question := "分析一下这段代码的性能瓶颈在哪里"
	result := c.classifyByRules(question)
	if result.Complexity == Simple {
		t.Errorf("expected not Simple, got %v", result.Complexity)
	}
}

func TestClassifyHybrid_UncertainFallsBack(t *testing.T) {
	c := NewClassifier(nil) // nil analyzer
	// Use a question that should yield Uncertain from rules
	question := "帮我看看这个"
	result, err := c.Classify(context.Background(), question)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With nil analyzer and uncertain rules, should fallback to Medium
	if result.Complexity != Medium {
		t.Errorf("expected Medium fallback, got %v (method=%s)", result.Complexity, result.Method)
	}
}

func TestClassifyHybrid_ModelOverride(t *testing.T) {
	mock := &mockAnalyzer{result: Complex}
	c := NewClassifier(mock)
	// Use a short question that might not trigger high confidence in rules
	question := "帮我看看这个"
	result, err := c.Classify(context.Background(), question)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Complexity != Complex {
		t.Errorf("expected Complex from model override, got %v", result.Complexity)
	}
	if result.Method != "model" {
		t.Errorf("expected method=model, got %s", result.Method)
	}
}

// mockAnalyzer implements ModelAnalyzer for testing.
type mockAnalyzer struct {
	result Complexity
}

func (m *mockAnalyzer) AnalyzeComplexity(ctx context.Context, question string) (Complexity, error) {
	return m.result, nil
}
