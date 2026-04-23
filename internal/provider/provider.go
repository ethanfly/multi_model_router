package provider

import (
	"context"
	"io"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream"`
}

type StreamChunk struct {
	Content   string
	Done      bool
	Model     string
	Usage     *Usage
	Error     error
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ModelInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

type Provider interface {
	ChatCompletion(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
	ListModels(ctx context.Context) ([]ModelInfo, error)
	HealthCheck(ctx context.Context) error
}

func CollectStream(ch <-chan StreamChunk) (string, *Usage, error) {
	var content string
	var usage *Usage
	for chunk := range ch {
		if chunk.Error != nil {
			return content, usage, chunk.Error
		}
		content += chunk.Content
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	return content, usage, nil
}

func ReadBody(body io.ReadCloser, limit int64) ([]byte, error) {
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, limit))
}
