package llm

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGetAdapter(t *testing.T) {
	opts := LLMOptions{Provider: "ollama", ModelName: "llama3.2"}
	adapter, err := createLLM(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to get adapter: %v", err)
	}
	if adapter == nil {
		t.Fatal("adapter is nil")
	}
}
func TestToolCallUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected ToolCall
	}{
		{
			name: "nested parameters",
			json: `{"tool": "create_task", "parameters": {"intent": "test", "estimated_hours": 4}}`,
			expected: ToolCall{
				Tool: "create_task",
				Parameters: map[string]interface{}{
					"intent":          "test",
					"estimated_hours": float64(4),
				},
			},
		},
		{
			name: "flat parameters",
			json: `{"tool": "create_task", "intent": "test", "estimated_hours": 4}`,
			expected: ToolCall{
				Tool: "create_task",
				Parameters: map[string]interface{}{
					"intent":          "test",
					"estimated_hours": float64(4),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tc ToolCall
			if err := json.Unmarshal([]byte(tt.json), &tc); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if tc.Tool != tt.expected.Tool {
				t.Errorf("expected tool %q, got %q", tt.expected.Tool, tc.Tool)
			}
			if len(tc.Parameters) != len(tt.expected.Parameters) {
				t.Errorf("expected %d parameters, got %d", len(tt.expected.Parameters), len(tc.Parameters))
			}
			for k, v := range tt.expected.Parameters {
				if tc.Parameters[k] != v {
					t.Errorf("parameter %q: expected %v, got %v", k, v, tc.Parameters[k])
				}
			}
		})
	}
}
