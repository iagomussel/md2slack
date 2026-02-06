package llm

import "testing"

func TestGetAdapter(t *testing.T) {
	opts := LLMOptions{Provider: "ollama", ModelName: "llama3.2"}
	adapter, err := getAdapter(opts)
	if err != nil {
		t.Fatalf("failed to get adapter: %v", err)
	}
	if adapter == nil {
		t.Fatal("adapter is nil")
	}
}
