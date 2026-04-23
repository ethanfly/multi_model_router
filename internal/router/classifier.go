package router

import (
	"context"
	"encoding/json"
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

// ClassifierConfig holds configurable parameters for the rule-based classifier.
type ClassifierConfig struct {
	ComplexKeywords    []string `json:"complex_keywords"`
	SimpleKeywords     []string `json:"simple_keywords"`
	MultiStepKeywords  []string `json:"multi_step_keywords"`
	MathSymbols        []string `json:"math_symbols"`
	CodingKeywords     []string `json:"coding_keywords"`
	ReasoningKeywords  []string `json:"reasoning_keywords"`
	ComplexThreshold   float64  `json:"complex_threshold"`
	SimpleThreshold    float64  `json:"simple_threshold"`
}

// DefaultClassifierConfig returns the built-in default classifier configuration.
func DefaultClassifierConfig() *ClassifierConfig {
	return &ClassifierConfig{
		ComplexKeywords: []string{
			"设计", "架构", "推导", "证明", "优化", "重构", "实现", "构建", "部署", "调试",
			"分析", "评估", "对比", "权衡", "方案", "策略", "模式",
			"design", "architect", "derive", "prove", "optimize", "refactor",
			"implement a system", "build a", "create a framework",
			"troubleshoot", "debug", "migrate", "integrate",
			"best practice", "design pattern", "trade-off", "compare",
		},
		SimpleKeywords: []string{
			"翻译", "总结", "改写", "你好", "谢谢", "是什么", "什么是", "解释一下",
			"translate", "summarize", "rewrite", "what is", "define", "list",
			"hello", "hi ", "thanks", "please explain",
			"how to say", "meaning of", "convert",
		},
		MultiStepKeywords: []string{
			"步骤", "第一步", "首先", "然后", "接下来", "最后", "流程",
			"step", "first,", "then,", "finally,", "next,", "after that",
			"workflow", "pipeline", "procedure",
		},
		MathSymbols: []string{
			"∫", "∑", "∂", "∇", "方程", "积分", "微分", "矩阵", "向量",
			"prove", "theorem", "lemma", "corollary",
			"∂²", "∞", "≈", "≠", "≤", "≥", "∀", "∃",
		},
		CodingKeywords: []string{
			"函数", "类", "接口", "算法", "排序", "递归", "并发", "异步",
			"数据库", "缓存", "索引", "事务", "锁",
			"function", "class ", "interface", "algorithm", "sort",
			"recursion", "concurrent", "async", "await",
			"database", "cache", "index", "transaction", "lock",
			"api", "endpoint", "middleware", "handler",
			"test", "unit test", "integration test", "benchmark",
			"docker", "kubernetes", "container",
		},
		ReasoningKeywords: []string{
			"为什么", "原因", "逻辑", "推理", "因果", "假设",
			"why", "reason", "because", "logic", "inference",
			"hypothesis", "assumption", "therefore", "conclusion",
			"analyze", "evaluate", "assess", "investigate",
			"pros and cons", "advantages", "disadvantages",
		},
		ComplexThreshold: 0.3,
		SimpleThreshold:  -0.2,
	}
}

// ParseClassifierConfig parses JSON into a ClassifierConfig, falling back to defaults.
func ParseClassifierConfig(data string) *ClassifierConfig {
	if data == "" {
		return DefaultClassifierConfig()
	}
	cfg := DefaultClassifierConfig()
	if err := json.Unmarshal([]byte(data), cfg); err != nil {
		return DefaultClassifierConfig()
	}
	// Ensure thresholds have sensible defaults
	if cfg.ComplexThreshold == 0 {
		cfg.ComplexThreshold = 0.3
	}
	if cfg.SimpleThreshold == 0 {
		cfg.SimpleThreshold = -0.2
	}
	return cfg
}

// ToJSON serializes the config to JSON.
func (c *ClassifierConfig) ToJSON() string {
	data, err := json.Marshal(c)
	if err != nil {
		b, _ := json.Marshal(DefaultClassifierConfig())
		return string(b)
	}
	return string(data)
}

// Classifier performs hybrid question classification using local rules
// and optional model pre-analysis.
type Classifier struct {
	modelAnalyzer ModelAnalyzer
	config        *ClassifierConfig
}

// NewClassifier creates a new Classifier with the given config and optional ModelAnalyzer.
// If config is nil, default configuration is used.
func NewClassifier(config *ClassifierConfig, analyzer ModelAnalyzer) *Classifier {
	if config == nil {
		config = DefaultClassifierConfig()
	}
	return &Classifier{
		modelAnalyzer: analyzer,
		config:        config,
	}
}

// Classify performs hybrid classification on the given question.
// Layer 1: rule-based classification. If confidence >= 0.65, return immediately.
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
	cfg := c.config

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

	// Complex keywords (from config)
	for _, kw := range cfg.ComplexKeywords {
		if strings.Contains(strings.ToLower(question), strings.ToLower(kw)) {
			score += 0.35
			break
		}
	}

	// Simple keywords (from config)
	for _, kw := range cfg.SimpleKeywords {
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

	// Multi-step indicators (from config)
	multiStepCount := 0
	lowerQ := strings.ToLower(question)
	for _, kw := range cfg.MultiStepKeywords {
		if strings.Contains(lowerQ, strings.ToLower(kw)) {
			multiStepCount++
		}
	}
	if multiStepCount >= 2 {
		score += 0.3
	}

	// Math symbols (from config)
	for _, sym := range cfg.MathSymbols {
		if strings.Contains(question, sym) {
			score += 0.3
			break
		}
	}

	// Coding keywords (from config)
	for _, kw := range cfg.CodingKeywords {
		if strings.Contains(lowerQ, strings.ToLower(kw)) {
			score += 0.25
			break
		}
	}

	// Reasoning keywords (from config)
	for _, kw := range cfg.ReasoningKeywords {
		if strings.Contains(lowerQ, strings.ToLower(kw)) {
			score += 0.2
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

	// Convert score to complexity using configurable thresholds
	var complexity Complexity
	if score >= cfg.ComplexThreshold {
		complexity = Complex
	} else if score <= cfg.SimpleThreshold {
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
