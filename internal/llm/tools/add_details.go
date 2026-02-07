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
	Tasks    *[]gitdiff.TaskChange
}

func (t *AddDetailsTool) Name() string {
	return "add_details"
}

func (t *AddDetailsTool) Description() string {
	return "Add technical details to a task."
}

func (t *AddDetailsTool) Call(ctx context.Context, input string) (string, error) {
	fmt.Println("add_details called with input:", input)
	var params struct {
		Index   int    `json:"index"`
		Details string `json:"details"`
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

	if params.Index < 0 || params.Index >= len(tasks) {
		fmt.Println("ERROR:index out of bounds", params.Index)
		return "ERROR:index out of bounds", fmt.Errorf("index %d out of bounds", params.Index)
	}

	task := &tasks[params.Index]
	if task.Details != "" {
		task.Details += "\n" + params.Details
	} else {
		task.Details = params.Details
	}

	updated, err := storage.UpdateTask(t.RepoName, t.Date, task.ID, *task)
	if err != nil {
		fmt.Println("ERROR:failed to update task", err)
		return "ERROR:failed to update task", err
	}
	*t.Tasks = updated

	return "Details added", nil
}

func (t *AddDetailsTool) GetUpdatedTasks() []gitdiff.TaskChange {
	return *t.Tasks
}
