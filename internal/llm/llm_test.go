package llm

import "testing"

func TestAnthropicProviderUsesMessagesEndpoint(t *testing.T) {
	opts := LLMOptions{Provider: "anthropic", BaseUrl: "https://api.anthropic.com/v1/messages", ModelName: "claude-3-5-sonnet-20241022"}
	url := resolveLLMURL(opts)
	if url != "https://api.anthropic.com/v1/messages" {
		t.Fatalf("unexpected url: %s", url)
	}
}
