package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"md2slack/internal/gitdiff"
	"md2slack/internal/storage"
)

type AddDetailsTool struct {
	RepoName string
	Date     string
	Tasks    []gitdiff.TaskChange
}

func (t *AddDetailsTool) Name() string {
	return "add_details"
}

func (t *AddDetailsTool) Description() string {
	return "Add technical details to a task."
}

func (t *AddDetailsTool) Call(ctx context.Context, input string) (string, error) {
	var params struct {
		Index       int    `json:"index"`
		Description string `json:"description"`
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
	// Append if it exists or overwrite? Assuming overwrite or append. Let's overwrite TechnicalWhy as it seems to be the field for generic technical details.
	if task.TechnicalWhy != "" {
		task.TechnicalWhy += "\n" + params.Description
	} else {
		task.TechnicalWhy = params.Description
	}

	updated, err := storage.UpdateTask(t.RepoName, t.Date, task.ID, *task)
	if err != nil {
		return "", err
	}
	t.Tasks = updated

	return "details added", nil
}

func (t *AddDetailsTool) GetUpdatedTasks() []gitdiff.TaskChange {
	return t.Tasks
}
