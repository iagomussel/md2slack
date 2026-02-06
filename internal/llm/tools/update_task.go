package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"md2slack/internal/gitdiff"
)

// UpdateTaskTool implements the tools.Tool interface for updating tasks
type UpdateTaskTool struct {
	Tasks []gitdiff.TaskChange
}

func (t *UpdateTaskTool) Name() string {
	return "update_task"
}

func (t *UpdateTaskTool) Description() string {
	return `Updates an existing task by index.
Parameters (JSON):
{
  "index": 0,
  "title": "string - new title (optional)",
  "description": "string - new description (optional)",
  "time_estimate": "string - new estimate (optional)",
  "intent": "string - new intent (optional)"
}`
}

func (t *UpdateTaskTool) Call(ctx context.Context, input string) (string, error) {
	var params struct {
		Index        int     `json:"index"`
		Title        *string `json:"title,omitempty"`
		Description  *string `json:"description,omitempty"`
		TimeEstimate *string `json:"time_estimate,omitempty"`
		Intent       *string `json:"intent,omitempty"`
	}

	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if params.Index < 0 || params.Index >= len(t.Tasks) {
		return "", fmt.Errorf("index %d out of bounds (0-%d)", params.Index, len(t.Tasks)-1)
	}

	task := &t.Tasks[params.Index]

	if params.Title != nil {
		task.Title = *params.Title
	}
	if params.Description != nil {
		task.Description = *params.Description
	}
	if params.TimeEstimate != nil {
		task.TimeEstimate = *params.TimeEstimate
	}
	if params.Intent != nil {
		task.Intent = *params.Intent
	}

	result := map[string]interface{}{
		"status": "updated",
		"index":  params.Index,
		"task":   task,
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

func (t *UpdateTaskTool) GetUpdatedTasks() []gitdiff.TaskChange {
	return t.Tasks
}
