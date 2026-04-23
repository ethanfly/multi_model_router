package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
		t.Errorf("expected path /chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":5,"completion_tokens":1}}`)
	}))
	defer server.Close()

	p := NewOpenAI(server.URL, "test-key")
	if err := p.HealthCheck(context.Background(), "gpt-4o-mini"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestOpenAIHealthCheckFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer server.Close()

	p := NewOpenAI(server.URL, "test-key")
	err := p.HealthCheck(context.Background(), "gpt-4o-mini")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

func TestOpenAIBaseURLNormalization(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"https://api.openai.com/v1", "https://api.openai.com/v1"},
		{"https://api.openai.com/v1/", "https://api.openai.com/v1"},
		{"https://ark.cn-beijing.volces.com/api/v3", "https://ark.cn-beijing.volces.com/api/v3"},
		{"https://api.deepseek.com", "https://api.deepseek.com"},
	}
	for _, tc := range cases {
		p := NewOpenAI(tc.input, "key")
		if p.BaseURL != tc.expected {
			t.Errorf("NewOpenAI(%q): expected %q, got %q", tc.input, tc.expected, p.BaseURL)
		}
	}
}

func TestOpenAIListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"gpt-4o"},{"id":"gpt-4o-mini"}]}`)
	}))
	defer server.Close()

	p := NewOpenAI(server.URL, "test-key")
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "gpt-4o" {
		t.Errorf("expected first model gpt-4o, got %s", models[0].ID)
	}
	if models[1].ID != "gpt-4o-mini" {
		t.Errorf("expected second model gpt-4o-mini, got %s", models[1].ID)
	}
}

func TestOpenAIChatCompletionStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}],\"model\":\"gpt-4o\"}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}],\"model\":\"gpt-4o\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := NewOpenAI(server.URL, "test-key")
	ch, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	content, _, err := CollectStream(ch)
	if err != nil {
		t.Fatalf("expected no error from CollectStream, got %v", err)
	}
	if content != "Hello world" {
		t.Errorf("expected content 'Hello world', got %q", content)
	}
}
