package llm

import (
	"context"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

type OllamaAdapter struct {
	llm *ollama.LLM
}

func NewOllamaAdapter(model string, baseURL string) (*OllamaAdapter, error) {
	opts := []ollama.Option{ollama.WithModel(model)}
	if baseURL != "" {
		opts = append(opts, ollama.WithServerURL(baseURL))
	}
	l, err := ollama.New(opts...)
	if err != nil {
		return nil, err
	}
	return &OllamaAdapter{llm: l}, nil
}

func (a *OllamaAdapter) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	return a.llm.GenerateContent(ctx, messages, options...)
}
