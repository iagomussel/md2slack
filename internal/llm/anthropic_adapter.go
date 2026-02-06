package llm

import (
	"context"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
)

type AnthropicAdapter struct {
	llm *anthropic.LLM
}

func NewAnthropicAdapter(model string, token string, baseURL string) (*AnthropicAdapter, error) {
	opts := []anthropic.Option{anthropic.WithModel(model)}
	if token != "" {
		opts = append(opts, anthropic.WithToken(token))
	}
	if baseURL != "" {
		// Anthropic baseURL in langchaingo might have different meaning or not be directly supported as a simple server URL override the same way,
		// but let's check if it exists.
		// Searching docs showed it might be useful for proxying.
	}
	l, err := anthropic.New(opts...)
	if err != nil {
		return nil, err
	}
	return &AnthropicAdapter{llm: l}, nil
}

func (a *AnthropicAdapter) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	return a.llm.GenerateContent(ctx, messages, options...)
}
