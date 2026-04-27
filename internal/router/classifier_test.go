package router

import "testing"

func TestClassifier_GreetingIsSimple(t *testing.T) {
	classifier := NewClassifier(nil, nil)

	result := classifier.classifyByRules("hello")
	if result.Complexity != Simple {
		t.Fatalf("expected simple for greeting, got %s", result.Complexity.String())
	}
}

func TestClassifier_TranslateRequestIsSimple(t *testing.T) {
	classifier := NewClassifier(nil, nil)

	result := classifier.classifyByRules("translate this sentence to Chinese")
	if result.Complexity != Simple {
		t.Fatalf("expected simple for translation request, got %s", result.Complexity.String())
	}
}

func TestClassifier_DebugCodeRequestIsComplex(t *testing.T) {
	classifier := NewClassifier(nil, nil)

	result := classifier.classifyByRules("Help me debug this Go panic:\n```go\nfunc main(){ panic(\"boom\") }\n```")
	if result.Complexity != Complex {
		t.Fatalf("expected complex for debug code request, got %s", result.Complexity.String())
	}
}

func TestClassifier_SystemDesignQuestionIsComplex(t *testing.T) {
	classifier := NewClassifier(nil, nil)

	result := classifier.classifyByRules("Design a distributed event pipeline, compare Kafka and Redis, and explain the trade-offs for consistency and latency.")
	if result.Complexity != Complex {
		t.Fatalf("expected complex for system design question, got %s", result.Complexity.String())
	}
}

func TestClassifier_DetectsAgenticCodingTask(t *testing.T) {
	classifier := NewClassifier(nil, nil)

	result := classifier.classifyByRules("Build a Claude Code tool-use bridge that preserves streaming tool calls and fixes failing Go tests.")
	if result.TaskType != TaskAgentic {
		t.Fatalf("expected agentic task type, got %s", result.TaskType.String())
	}
	if result.Complexity != Complex {
		t.Fatalf("expected complex for agentic coding task, got %s", result.Complexity.String())
	}
}

func TestClassifier_DetectsCreativeWritingTask(t *testing.T) {
	classifier := NewClassifier(nil, nil)

	result := classifier.classifyByRules("Write a polished product launch announcement with a warmer tone and three headline options.")
	if result.TaskType != TaskCreative {
		t.Fatalf("expected creative task type, got %s", result.TaskType.String())
	}
}
