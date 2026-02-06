package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"md2slack/internal/gitdiff"
	"md2slack/internal/storage"
)

type AddTimeTool struct {
	RepoName string
	Date     string
	Tasks    []gitdiff.TaskChange
}

func (t *AddTimeTool) Name() string {
	return "add_time"
}

func (t *AddTimeTool) Description() string {
	return "Add estimated hours to a task."
}

func (t *AddTimeTool) Call(ctx context.Context, input string) (string, error) {
	var params struct {
		Index int `json:"index"`
		Hours int `json:"hours"`
	}

	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "ERROR:invalid parameters", fmt.Errorf("invalid parameters: %w", err)
	}

	tasks, err := storage.LoadTasks(t.RepoName, t.Date)
	if err != nil {
		return "", err
	}

	if params.Index < 0 || params.Index >= len(tasks) {
		return "ERROR:index out of bounds", fmt.Errorf("index %d out of bounds", params.Index)
	}

	task := &tasks[params.Index]
	task.EstimatedHours += float64(params.Hours)

	updated, err := storage.UpdateTask(t.RepoName, t.Date, task.ID, *task)
	if err != nil {
		return "", err
	}
	t.Tasks = updated

	return fmt.Sprintf("time updated to %.1fh", task.EstimatedHours), nil
}

func (t *AddTimeTool) GetUpdatedTasks() []gitdiff.TaskChange {
	return t.Tasks
}
