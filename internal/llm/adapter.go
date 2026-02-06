package llm

import (
	"context"

	"github.com/tmc/langchaingo/llms"
)

type LLMAdapter interface {
	GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error)
}
