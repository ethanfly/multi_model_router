package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicChatCompletionStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key test-key, got %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version 2023-06-01, got %s", r.Header.Get("anthropic-version"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-4-20250514\",\"usage\":{\"input_tokens\":10}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"Hello\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\" from Claude\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":5}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	p := NewAnthropic(server.URL, "test-key")
	ch, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	content, usage, err := CollectStream(ch)
	if err != nil {
		t.Fatalf("expected no error from CollectStream, got %v", err)
	}
	if content != "Hello from Claude" {
		t.Errorf("expected content 'Hello from Claude', got %q", content)
	}
	if usage == nil {
		t.Fatal("expected usage to be non-nil")
	}
	if usage.InputTokens != 10 {
		t.Errorf("expected input tokens 10, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 5 {
		t.Errorf("expected output tokens 5, got %d", usage.OutputTokens)
	}
}

func TestAnthropicHealthCheckBadAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer server.Close()

	p := NewAnthropic(server.URL, "bad-key")
	err := p.HealthCheck(context.Background(), "claude-haiku-4-5-20251001")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}
