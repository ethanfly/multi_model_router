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

type OpenAIProvider struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewOpenAI(baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: &http.Client{},
	}
}

type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Model string `json:"model"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	oaiReq := openaiRequest{
		Model:       req.Model,
		Messages:    make([]openaiMessage, len(req.Messages)),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}
	for i, m := range req.Messages {
		oaiReq.Messages[i] = openaiMessage{Role: m.Role, Content: m.Content}
	}

	body, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := ReadBody(resp.Body, 4096)
		return nil, fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk, 64)
	go p.streamOpenAI(resp.Body, ch, req.Model)
	return ch, nil
}

func (p *OpenAIProvider) streamOpenAI(body io.ReadCloser, ch chan<- StreamChunk, model string) {
	defer close(ch)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- StreamChunk{Done: true, Model: model}
			return
		}

		var resp openaiStreamResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue
		}

		for _, choice := range resp.Choices {
			content := choice.Delta.Content
			if content != "" || (choice.FinishReason != nil && *choice.FinishReason == "stop") {
				chunk := StreamChunk{
					Content: content,
					Model:   resp.Model,
				}
				if resp.Usage != nil {
					chunk.Usage = &Usage{
						InputTokens:  resp.Usage.PromptTokens,
						OutputTokens: resp.Usage.CompletionTokens,
					}
				}
				ch <- chunk
			}
		}
	}
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]ModelInfo, len(result.Data))
	for i, m := range result.Data {
		models[i] = ModelInfo{ID: m.ID, Name: m.ID, Provider: "openai"}
	}
	return models, nil
}

func (p *OpenAIProvider) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", p.BaseURL+"/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}
	return nil
}

var _ Provider = (*OpenAIProvider)(nil)
