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
  "commits": ["array of commit hashes"],
  "intent": "string - user's intended action",
  "file_path": "string - associated file"
}`
}

func (t *CreateTaskTool) Call(ctx context.Context, input string) (string, error) {
	var params struct {
		Title        string   `json:"title"`
		Description  string   `json:"description"`
		TimeEstimate string   `json:"time_estimate"`
		Commits      []string `json:"commits"`
		Intent       string   `json:"intent"`
		FilePath     string   `json:"file_path"`
	}

	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "ERROR:invalid parameters: %w fix it and try again", fmt.Errorf("invalid parameters: %w", err)
	}

	newTask := gitdiff.TaskChange{
		Title:        params.Title,
		Description:  params.Description,
		TimeEstimate: params.TimeEstimate,
		TaskIntent:   params.Intent,
		Commits:      params.Commits,
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
