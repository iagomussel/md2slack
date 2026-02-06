package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"md2slack/internal/gitdiff"
	"md2slack/internal/storage"
)

type AddCommitReferenceTool struct {
	RepoName string
	Date     string
	Tasks    []gitdiff.TaskChange
}

func (t *AddCommitReferenceTool) Name() string {
	return "add_commit_reference"
}

func (t *AddCommitReferenceTool) Description() string {
	return "Link a commit to a task."
}

func (t *AddCommitReferenceTool) Call(ctx context.Context, input string) (string, error) {
	var params struct {
		Index int    `json:"index"`
		Hash  string `json:"hash"`
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

	// Check for duplicates
	exists := false
	for _, c := range task.Commits {
		if c == params.Hash {
			exists = true
			break
		}
	}
	if !exists {
		task.Commits = append(task.Commits, params.Hash)
	}

	updated, err := storage.UpdateTask(t.RepoName, t.Date, task.ID, *task)
	if err != nil {
		return "", err
	}
	t.Tasks = updated

	return fmt.Sprintf("commit %s associated", params.Hash), nil
}

func (t *AddCommitReferenceTool) GetUpdatedTasks() []gitdiff.TaskChange {
	return t.Tasks
}
