package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type AnthropicProvider struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewAnthropic(baseURL, apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: &http.Client{},
	}
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicEvent struct {
	Type         string `json:"type"`
	Message      *struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Delta *struct {
		Text string `json:"text"`
	} `json:"delta"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *AnthropicProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	// Extract system messages and build the messages list
	var systemPrompt string
	var msgs []anthropicMessage
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt += m.Content + "\n"
		} else {
			msgs = append(msgs, anthropicMessage{Role: m.Role, Content: m.Content})
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	antReq := anthropicRequest{
		Model:       req.Model,
		Messages:    msgs,
		System:      strings.TrimSpace(systemPrompt),
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}

	body, err := json.Marshal(antReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := ReadBody(resp.Body, 4096)
		return nil, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk, 64)
	go p.streamAnthropic(resp.Body, ch, req.Model)
	return ch, nil
}

func (p *AnthropicProvider) streamAnthropic(body io.ReadCloser, ch chan<- StreamChunk, model string) {
	defer close(ch)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var inputTokens int

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event anthropicEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "message_start":
			if event.Message != nil {
				model = event.Message.Model
				if event.Message.Usage != nil {
					inputTokens = event.Message.Usage.InputTokens
				}
			}
		case "content_block_delta":
			if event.Delta != nil && event.Delta.Text != "" {
				ch <- StreamChunk{
					Content: event.Delta.Text,
					Model:   model,
				}
			}
		case "message_delta":
			usage := &Usage{InputTokens: inputTokens}
			if event.Usage != nil {
				usage.OutputTokens = event.Usage.OutputTokens
			}
			ch <- StreamChunk{
				Usage: usage,
				Model: model,
			}
		case "message_stop":
			ch <- StreamChunk{Done: true, Model: model}
			return
		}
	}
}

func (p *AnthropicProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	models := []ModelInfo{
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic"},
		{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", Provider: "anthropic"},
	}
	return models, nil
}

func (p *AnthropicProvider) HealthCheck(ctx context.Context) error {
	// Send a minimal request to verify credentials
	body, _ := json.Marshal(anthropicRequest{
		Model:     "claude-haiku-4-5-20251001",
		Messages:  []anthropicMessage{{Role: "user", Content: "hi"}},
		MaxTokens: 1,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode >= 500 {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}
	return nil
}

var _ Provider = (*AnthropicProvider)(nil)
