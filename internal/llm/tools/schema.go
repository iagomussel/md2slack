package tools

import "github.com/tmc/langchaingo/llms"

// GetLLMDefinitions matches the available tools to LLM Tool definitions
func GetLLMDefinitions() []llms.Tool {
	return []llms.Tool{
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "create_task",
				Description: "Create a new task.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"intent":          map[string]interface{}{"type": "string", "description": "User intent (what needs to be done)"},
						"title":           map[string]interface{}{"type": "string", "description": "Short title of the task"},
						"description":     map[string]interface{}{"type": "string", "description": "Detailed description"},
						"scope":           map[string]interface{}{"type": "string", "description": "Scope of the task (e.g., backend, ui)"},
						"type":            map[string]interface{}{"type": "string", "description": "Type of task (e.g., feature, fix, chore)"},
						"estimated_hours": map[string]interface{}{"type": "integer", "description": "Estimated hours (e.g., 2)"},
					},
					"required": []string{"intent"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "edit_task",
				Description: "Edit an existing task by index.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index":           map[string]interface{}{"type": "integer", "description": "Index of the task to edit"},
						"intent":          map[string]interface{}{"type": "string"},
						"title":           map[string]interface{}{"type": "string"},
						"description":     map[string]interface{}{"type": "string"},
						"scope":           map[string]interface{}{"type": "string"},
						"estimated_hours": map[string]interface{}{"type": "integer"},
					},
					"required": []string{"index"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "add_details",
				Description: "Add technical details to a task.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index":       map[string]interface{}{"type": "integer", "description": "Index of the task"},
						"description": map[string]interface{}{"type": "string", "description": "Technical details/summary to add"},
					},
					"required": []string{"index", "description"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "add_time",
				Description: "Add estimated hours to a task.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index": map[string]interface{}{"type": "integer", "description": "Index of the task"},
						"hours": map[string]interface{}{"type": "integer", "description": "Hours to add"},
					},
					"required": []string{"index", "hours"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "add_commit_reference",
				Description: "Link a commit to a task.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index": map[string]interface{}{"type": "integer", "description": "Index of the task"},
						"hash":  map[string]interface{}{"type": "string", "description": "Commit hash to link"},
					},
					"required": []string{"index", "hash"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "remove_task",
				Description: "Remove a task by index.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index": map[string]interface{}{"type": "integer", "description": "Index of the task to remove"},
					},
					"required": []string{"index"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "get_codebase_context",
				Description: "Search the codebase for context.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query":       map[string]interface{}{"type": "string", "description": "Search query"},
						"path":        map[string]interface{}{"type": "string", "description": "Directory to search (default .)"},
						"max_results": map[string]interface{}{"type": "integer", "description": "Max results (default 20)"},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "merge_tasks",
				Description: "Merge multiple tasks into one.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"indices":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "integer"}, "description": "Indices of tasks to merge"},
						"new_intent": map[string]interface{}{"type": "string", "description": "Intent for the merged task"},
						"new_scope":  map[string]interface{}{"type": "string", "description": "Scope for the merged task"},
					},
					"required": []string{"indices", "new_intent", "new_scope"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "split_task",
				Description: "Split a task into multiple tasks.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index": map[string]interface{}{"type": "integer", "description": "Index of task to split"},
						"new_tasks": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"intent":          map[string]interface{}{"type": "string"},
									"scope":           map[string]interface{}{"type": "string"},
									"type":            map[string]interface{}{"type": "string"},
									"technical_why":   map[string]interface{}{"type": "string"},
									"estimated_hours": map[string]interface{}{"type": "integer"},
									"commits":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								},
								"required": []string{"intent"},
							},
						},
					},
					"required": []string{"index", "new_tasks"},
				},
			},
		},
	}
}
