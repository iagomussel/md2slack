package tools

import "github.com/tmc/langchaingo/llms"

// GetLLMDefinitions matches the available tools to LLM Tool definitions
func GetLLMDefinitions() []llms.Tool {
	return []llms.Tool{
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "create_task",
				Description: "Create a new task in the list.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title":         map[string]interface{}{"type": "string", "description": "The title of the task"},
						"description":   map[string]interface{}{"type": "string", "description": "Detailed description"},
						"time_estimate": map[string]interface{}{"type": "string", "description": "Estimate like '2h', '30m'"},
						"intent":        map[string]interface{}{"type": "string", "description": "User intent"},
					},
					"required": []string{"title"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "update_task",
				Description: "Update an existing task.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index":         map[string]interface{}{"type": "integer", "description": "Index of task to update"},
						"title":         map[string]interface{}{"type": "string"},
						"description":   map[string]interface{}{"type": "string"},
						"time_estimate": map[string]interface{}{"type": "string"},
						"intent":        map[string]interface{}{"type": "string"},
					},
					"required": []string{"index"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "delete_task",
				Description: "Delete tasks from the list.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"indices": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "integer"},
							"description": "List of indices to delete",
						},
					},
					"required": []string{"indices"},
				},
			},
		},
	}
}
