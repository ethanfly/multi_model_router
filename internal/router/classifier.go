package router

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	codeBlockRe     = regexp.MustCompile("(?s)```.*?```")
	inlineCodeRe    = regexp.MustCompile("`[^`\n]+`")
	listMarkerRe    = regexp.MustCompile(`(?m)^\s*(\d+[\.\)]|[-*])\s+`)
	stackTraceRe    = regexp.MustCompile(`(?i)(traceback|stack trace|panic:|exception|caused by:|fatal:)`)
	codeSignalRe    = regexp.MustCompile(`(?i)\b(func|function|class|interface|struct|import|select|insert|update|delete|curl|npm|yarn|pnpm|go test|pytest|docker|kubectl|sql|http|json|yaml|regex)\b`)
	pathSignalRe    = regexp.MustCompile(`(?i)([a-z]:\\|/[\w\-.]+/|[\w\-.]+\.(go|ts|tsx|js|jsx|py|java|rs|sql|yaml|yml|json|md|sh))`)
	shortGreetingRe = regexp.MustCompile(`(?i)^\s*(hi|hello|hey|thanks|thank you|ok|okay|got it|cool|nice|你好|您好|谢谢|收到|好的)\s*[!.?？。]*\s*$`)
)

// Complexity represents the complexity level of a question.
type Complexity int

const (
	Simple Complexity = iota
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
	ComplexKeywords   []string `json:"complex_keywords"`
	SimpleKeywords    []string `json:"simple_keywords"`
	MultiStepKeywords []string `json:"multi_step_keywords"`
	MathSymbols       []string `json:"math_symbols"`
	CodingKeywords    []string `json:"coding_keywords"`
	ReasoningKeywords []string `json:"reasoning_keywords"`
	ComplexThreshold  float64  `json:"complex_threshold"`
	SimpleThreshold   float64  `json:"simple_threshold"`
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
			"distributed", "scalability", "latency", "root cause", "incident",
			"performance bottleneck", "eventual consistency", "fault tolerance",
		},
		SimpleKeywords: []string{
			"翻译", "总结", "改写", "你好", "谢谢", "是什么", "什么是", "解释一下",
			"translate", "summarize", "rewrite", "what is", "define", "list",
			"hello", "hi ", "thanks", "please explain",
			"how to say", "meaning of", "convert",
			"paraphrase", "proofread", "fix grammar", "rephrase", "short summary",
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
			"panic", "traceback", "stack trace", "golang", "typescript", "python", "javascript",
		},
		ReasoningKeywords: []string{
			"为什么", "原因", "逻辑", "推理", "因果", "假设",
			"why", "reason", "because", "logic", "inference",
			"hypothesis", "assumption", "therefore", "conclusion",
			"analyze", "evaluate", "assess", "investigate",
			"pros and cons", "advantages", "disadvantages",
			"tradeoff", "which is better", "should i", "why does",
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

	trimmed := strings.TrimSpace(question)
	lowerQ := strings.ToLower(trimmed)

	// Length-based scoring (use rune count for correct CJK handling)
	length := utf8.RuneCountInString(trimmed)
	wordCount := len(strings.Fields(lowerQ))
	switch {
	case length < 8:
		score -= 0.35
	case length < 20:
		score -= 0.1
	case length > 150:
		score += 0.4
	case length > 50:
		score += 0.1
	}

	if shortGreetingRe.MatchString(trimmed) {
		score -= 0.55
	}

	complexHits := keywordHits(lowerQ, cfg.ComplexKeywords)
	simpleHits := keywordHits(lowerQ, cfg.SimpleKeywords)
	codingHits := keywordHits(lowerQ, cfg.CodingKeywords)
	reasoningHits := keywordHits(lowerQ, cfg.ReasoningKeywords)
	multiStepHits := keywordHits(lowerQ, cfg.MultiStepKeywords)
	mathHits := rawHits(trimmed, cfg.MathSymbols)

	score += cappedWeight(complexHits, 0.18, 0.54)
	score -= cappedWeight(simpleHits, 0.18, 0.54)
	score += cappedWeight(codingHits, 0.16, 0.48)
	score += cappedWeight(reasoningHits, 0.14, 0.42)
	score += cappedWeight(mathHits, 0.2, 0.4)

	matches := codeBlockRe.FindAllString(trimmed, -1)
	if len(matches) == 1 {
		score += 0.2
	} else if len(matches) >= 2 {
		score += 0.45
	}

	if inlineCodeRe.MatchString(trimmed) {
		score += 0.12
	}

	listMarkers := len(listMarkerRe.FindAllString(trimmed, -1))
	if multiStepHits >= 1 || listMarkers >= 2 {
		score += 0.12
	}
	if multiStepHits >= 2 || listMarkers >= 3 {
		score += 0.3
	}

	if codeSignalRe.MatchString(trimmed) {
		score += 0.18
	}
	if pathSignalRe.MatchString(trimmed) {
		score += 0.15
	}
	if stackTraceRe.MatchString(trimmed) {
		score += 0.3
	}

	questionMarks := strings.Count(trimmed, "?") + strings.Count(trimmed, "？")
	if questionMarks >= 2 {
		score += 0.12
	}

	// Chinese + code bonus
	hasChinese := false
	for _, r := range trimmed {
		if r >= 0x4e00 && r <= 0x9fff {
			hasChinese = true
			break
		}
	}
	hasCode := codeBlockRe.MatchString(trimmed) || codeSignalRe.MatchString(trimmed) || pathSignalRe.MatchString(trimmed)
	if hasChinese && hasCode {
		score += 0.1
	}

	if length <= 20 && wordCount <= 4 && simpleHits > 0 && complexHits == 0 && codingHits == 0 {
		score -= 0.2
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
	confidence := 0.5 + abs(score)*0.32
	if complexity == Uncertain {
		confidence -= 0.05
	}
	if confidence < 0.5 {
		confidence = 0.5
	}
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

func keywordHits(question string, keywords []string) int {
	count := 0
	for _, kw := range keywords {
		kw = strings.TrimSpace(strings.ToLower(kw))
		if kw == "" {
			continue
		}
		if strings.Contains(question, kw) {
			count++
		}
	}
	return count
}

func rawHits(question string, patterns []string) int {
	count := 0
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.Contains(question, pattern) {
			count++
		}
	}
	return count
}

func cappedWeight(matches int, perMatch, cap float64) float64 {
	score := float64(matches) * perMatch
	if score > cap {
		return cap
	}
	return score
}
