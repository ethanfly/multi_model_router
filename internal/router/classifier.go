package router

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Complexity represents the complexity level of a question.
type Complexity int

const (
	Simple    Complexity = iota
	Medium
	Complex
	Uncertain
)

// String returns the string representation of a Complexity value.
func (c Complexity) String() string {
	switch c {
	case Simple:
		return "simple"
	case Medium:
		return "medium"
	case Complex:
		return "complex"
	case Uncertain:
		return "medium" // fallback
	default:
		return "medium"
	}
}

// ClassificationResult holds the result of classifying a question.
type ClassificationResult struct {
	Complexity Complexity
	Confidence float64
	Method     string
}

// ModelAnalyzer is an optional interface for model-based complexity analysis.
type ModelAnalyzer interface {
	AnalyzeComplexity(ctx context.Context, question string) (Complexity, error)
}

// Classifier performs hybrid question classification using local rules
// and optional model pre-analysis.
type Classifier struct {
	modelAnalyzer ModelAnalyzer
}

// NewClassifier creates a new Classifier with the given optional ModelAnalyzer.
func NewClassifier(analyzer ModelAnalyzer) *Classifier {
	return &Classifier{
		modelAnalyzer: analyzer,
	}
}

// Classify performs hybrid classification on the given question.
// Layer 1: rule-based classification. If confidence >= 0.7, return immediately.
// Layer 2: model-based analysis (if analyzer is available).
// Fallback: Medium complexity.
func (c *Classifier) Classify(ctx context.Context, question string) (*ClassificationResult, error) {
	// Layer 1: rule-based classification
	result := c.classifyByRules(question)
	if result.Confidence >= 0.65 {
		return result, nil
	}

	// Layer 2: model-based analysis if available
	if c.modelAnalyzer != nil {
		complexity, err := c.modelAnalyzer.AnalyzeComplexity(ctx, question)
		if err == nil {
			return &ClassificationResult{
				Complexity: complexity,
				Confidence: 0.8,
				Method:     "model",
			}, nil
		}
	}

	// Fallback to Medium
	return &ClassificationResult{
		Complexity: Medium,
		Confidence: 0.5,
		Method:     "fallback",
	}, nil
}

// classifyByRules performs rule-based classification on the question.
func (c *Classifier) classifyByRules(question string) *ClassificationResult {
	score := 0.0

	// Length-based scoring (use rune count for correct CJK handling)
	length := utf8.RuneCountInString(question)
	if length < 10 {
		score -= 0.3
	} else if length >= 10 && length <= 50 {
		// no change
	} else if length > 50 && length <= 150 {
		score += 0.1
	} else if length > 150 {
		score += 0.4
	}

	// Complex keywords
	complexKeywords := []string{
		"设计", "架构", "推导", "证明", "优化", "重构",
		"design", "architect", "derive", "prove", "optimize", "refactor",
		"implement a system", "build a", "create a framework",
	}
	for _, kw := range complexKeywords {
		if strings.Contains(strings.ToLower(question), strings.ToLower(kw)) {
			score += 0.35
			break
		}
	}

	// Simple keywords
	simpleKeywords := []string{
		"翻译", "总结", "改写",
		"translate", "summarize", "rewrite",
		"what is", "define", "list",
	}
	for _, kw := range simpleKeywords {
		if strings.Contains(strings.ToLower(question), strings.ToLower(kw)) {
			score -= 0.35
			break
		}
	}

	// Code blocks detection
	codeBlockRe := regexp.MustCompile("(?s)```.*?```")
	matches := codeBlockRe.FindAllString(question, -1)
	if len(matches) == 1 {
		score += 0.15
	} else if len(matches) >= 2 {
		score += 0.4
	}

	// Multi-step indicators
	multiStepKeywords := []string{
		"步骤", "第一步", "首先",
		"step", "first,", "then,", "finally,",
	}
	multiStepCount := 0
	lowerQ := strings.ToLower(question)
	for _, kw := range multiStepKeywords {
		if strings.Contains(lowerQ, strings.ToLower(kw)) {
			multiStepCount++
		}
	}
	if multiStepCount >= 2 {
		score += 0.3
	}

	// Math symbols
	mathSymbols := []string{"∫", "∑", "∂", "∇", "prove", "theorem", "方程", "积分"}
	for _, sym := range mathSymbols {
		if strings.Contains(question, sym) {
			score += 0.3
			break
		}
	}

	// Chinese + code bonus
	hasChinese := false
	for _, r := range question {
		if r >= 0x4e00 && r <= 0x9fff {
			hasChinese = true
			break
		}
	}
	hasCode := strings.Contains(question, "func ") ||
		strings.Contains(question, "function ") ||
		strings.Contains(question, "class ") ||
		strings.Contains(question, "import ") ||
		strings.Contains(question, "```")
	if hasChinese && hasCode {
		score += 0.1
	}

	// Convert score to complexity
	var complexity Complexity
	if score >= 0.3 {
		complexity = Complex
	} else if score <= -0.2 {
		complexity = Simple
	} else {
		complexity = Uncertain
	}

	// Calculate confidence
	confidence := 0.5 + abs(score)*0.3
	if confidence > 1.0 {
		confidence = 1.0
	}

	return &ClassificationResult{
		Complexity: complexity,
		Confidence: confidence,
		Method:     "rules",
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
