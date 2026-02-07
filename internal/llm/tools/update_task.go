package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"md2slack/internal/gitdiff"
	"md2slack/internal/storage"
)

// UpdateTaskTool implements the tools.Tool interface for updating tasks
type UpdateTaskTool struct {
	RepoName string
	Date     string
	Tasks    *[]gitdiff.TaskChange
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
  "details": "string - new details (optional)",
  "time_estimate": "string - new estimate (optional)",
  "intent": "string - new intent (optional)"
}`
}

func (t *UpdateTaskTool) Call(ctx context.Context, input string) (string, error) {
	fmt.Println("update_task called with input:", input)
	var params struct {
		Index        *int    `json:"index,omitempty"`
		TaskID       *string `json:"task_id,omitempty"`
		Title        *string `json:"title,omitempty"`
		Details      *string `json:"details,omitempty"`
		TimeEstimate *string `json:"time_estimate,omitempty"`
		Intent       *string `json:"intent,omitempty"`
	}

	if err := json.Unmarshal([]byte(input), &params); err != nil {
		fmt.Println("ERROR:invalid parameters", err)
		return "ERROR:invalid parameters", fmt.Errorf("invalid parameters: %w", err)
	}

	tasks, err := storage.LoadTasks(t.RepoName, t.Date)
	if err != nil {
		fmt.Println("ERROR:failed to load tasks", err)
		return "ERROR:failed to load tasks", err
	}
	var taskID string
	if params.TaskID != nil && *params.TaskID != "" {
		taskID = *params.TaskID
	} else if params.Index != nil {
		idx := *params.Index
		if idx < 0 || idx >= len(tasks) {
			fmt.Println("ERROR:index out of bounds", idx)
			return "ERROR:index out of bounds", fmt.Errorf("index %d out of bounds (0-%d)", idx, len(tasks)-1)
		}
		taskID = tasks[idx].ID
	} else {
		fmt.Println("ERROR:task_id or index is required")
		return "ERROR:task_id or index is required", fmt.Errorf("task_id or index is required")
	}
	var task *gitdiff.TaskChange
	for i := range tasks {
		if tasks[i].ID == taskID {
			task = &tasks[i]
			break
		}
	}
	if task == nil {
		fmt.Printf("ERROR:task_id %s not found\n", taskID)
		return "ERROR:task_id %s not found", fmt.Errorf("task_id %s not found", taskID)
	}

	if params.Title != nil {
		task.Title = *params.Title
	}
	if params.Details != nil {
		task.Details = *params.Details
	}
	if params.TimeEstimate != nil {
		task.TimeEstimate = *params.TimeEstimate
	}
	if params.Intent != nil {
		task.TaskIntent = *params.Intent
	}

	updated, err := storage.UpdateTask(t.RepoName, t.Date, taskID, *task)
	if err != nil {
		fmt.Println("ERROR:failed to update task", err)
		return "ERROR:failed to update task", err
	}
	*t.Tasks = updated

	result := map[string]interface{}{
		"status":  "updated",
		"task_id": taskID,
		"task":    task,
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

func (t *UpdateTaskTool) GetUpdatedTasks() []gitdiff.TaskChange {
	return *t.Tasks
}
