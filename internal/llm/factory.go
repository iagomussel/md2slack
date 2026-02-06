package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

// createLLM creates a native langchaingo LLM based on options
func createLLM(ctx context.Context, opts LLMOptions) (llms.Model, error) {
	provider := strings.ToLower(opts.Provider)
	switch provider {
	case "ollama":
		oOpts := []ollama.Option{ollama.WithModel(opts.ModelName)}
		if opts.BaseUrl != "" {
			oOpts = append(oOpts, ollama.WithServerURL(opts.BaseUrl))
		}
		return ollama.New(oOpts...)

	case "openai", "codex":
		oOpts := []openai.Option{openai.WithModel(opts.ModelName)}
		if opts.Token != "" {
			oOpts = append(oOpts, openai.WithToken(opts.Token))
		}
		if opts.BaseUrl != "" {
			oOpts = append(oOpts, openai.WithBaseURL(opts.BaseUrl))
		}
		return openai.New(oOpts...)

	case "anthropic":
		aOpts := []anthropic.Option{anthropic.WithModel(opts.ModelName)}
		if opts.Token != "" {
			aOpts = append(aOpts, anthropic.WithToken(opts.Token))
		}
		if opts.BaseUrl != "" {
			aOpts = append(aOpts, anthropic.WithBaseURL(opts.BaseUrl))
		}
		return anthropic.New(aOpts...)

	default:
		return nil, fmt.Errorf("unknown provider: %s", opts.Provider)
	}
}
