package llm

import (
	"context"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

type OpenAIAdapter struct {
	llm *openai.LLM
}

func NewOpenAIAdapter(model string, token string, baseURL string) (*OpenAIAdapter, error) {
	opts := []openai.Option{openai.WithModel(model)}
	if token != "" {
		opts = append(opts, openai.WithToken(token))
	}
	if baseURL != "" {
		opts = append(opts, openai.WithBaseURL(baseURL))
	}
	l, err := openai.New(opts...)
	if err != nil {
		return nil, err
	}
	return &OpenAIAdapter{llm: l}, nil
}

func (a *OpenAIAdapter) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	return a.llm.GenerateContent(ctx, messages, options...)
}
