package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"md2slack/internal/gitdiff"
	"md2slack/internal/storage"
)

// CreateTaskTool implements the tools.Tool interface for creating tasks
type CreateTaskTool struct {
	RepoName string
	Date     string
	Tasks    []gitdiff.TaskChange
}

func (t *CreateTaskTool) Name() string {
	return "create_task"
}

func (t *CreateTaskTool) Description() string {
	return `Creates a new task. 
Parameters (JSON):
{
  "title": "string - task title",
  "description": "string - detailed description", 
  "time_estimate": "string - e.g. '2h', '30m'",
  "scope": "string - scope of the task",		
  "type": "string - type of the task",
  "estimated_hours": "float - estimated hours",
  "intent": "string - user's intended action",
  "status": "string - status of the task"
}`
}

func (t *CreateTaskTool) Call(ctx context.Context, input string) (string, error) {
	var params struct {
		Title          string   `json:"title"`
		Description    string   `json:"description"`
		TimeEstimate   string   `json:"time_estimate"`
		Commits        []string `json:"commits"`
		Scope          string   `json:"scope"`
		Type           string   `json:"type"`
		EstimatedHours float64  `json:"estimated_hours"`
		Intent         string   `json:"intent"`
		Status         string   `json:"status"`
	}

	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "ERROR:invalid parameters: %w fix it and try again", fmt.Errorf("invalid parameters: %w", err)
	}

	newTask := gitdiff.TaskChange{
		RepoName:       t.RepoName,
		Date:           t.Date,
		Title:          params.Title,
		Description:    params.Description,
		TimeEstimate:   params.TimeEstimate,
		TaskIntent:     params.Intent,
		Commits:        params.Commits,
		Scope:          params.Scope,
		TaskType:       params.Type,
		EstimatedHours: params.EstimatedHours,
		Intent:         params.Intent,
		Status:         params.Status,
	}
	if newTask.TaskType == "" {
		newTask.TaskType = "delivery"
	}

	id, updated, err := storage.CreateTask(t.RepoName, t.Date, newTask)
	if err != nil {
		return "", err
	}
	t.Tasks = updated

	result := map[string]interface{}{
		"status":  "created",
		"task_id": id,
		"task":    updated[len(updated)-1],
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// GetUpdatedTasks returns the current task list after modifications
func (t *CreateTaskTool) GetUpdatedTasks() []gitdiff.TaskChange {
	return t.Tasks
}
