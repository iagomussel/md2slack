package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"md2slack/internal/gitdiff"
	"md2slack/internal/storage"
)

// DeleteTaskTool implements the tools.Tool interface for deleting tasks
type DeleteTaskTool struct {
	RepoName string
	Date     string
	Tasks    *[]gitdiff.TaskChange
}

func (t *DeleteTaskTool) Name() string {
	return "delete_task"
}

func (t *DeleteTaskTool) Description() string {
	return `Deletes tasks by their indices.
Parameters (JSON):
{
  "indices": [0, 2, 5]  // array of task indices to delete
}`
}

func (t *DeleteTaskTool) Call(ctx context.Context, input string) (string, error) {
	fmt.Println("delete_task called with input:", input)
	var params struct {
		Indices []int    `json:"indices,omitempty"`
		TaskIDs []string `json:"task_ids,omitempty"`
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

	taskIDs := append([]string{}, params.TaskIDs...)
	if len(taskIDs) == 0 && len(params.Indices) > 0 {
		for _, idx := range params.Indices {
			if idx < 0 || idx >= len(tasks) {
				fmt.Println("ERROR:index out of bounds", idx)
				return "ERROR:index out of bounds", fmt.Errorf("index %d out of bounds (0-%d)", idx, len(tasks)-1)
			}
			taskIDs = append(taskIDs, tasks[idx].ID)
		}
	}
	if len(taskIDs) == 0 {
		fmt.Println("ERROR:no task_ids or indices provided")
		return "ERROR:no task_ids or indices provided", fmt.Errorf("no task_ids or indices provided")
	}

	updated, err := storage.DeleteTasks(t.RepoName, t.Date, taskIDs)
	if err != nil {
		fmt.Println("ERROR:failed to delete tasks", err)
		return "ERROR:failed to delete tasks", err
	}
	*t.Tasks = updated

	result := map[string]interface{}{
		"status":   "deleted",
		"count":    len(taskIDs),
		"task_ids": taskIDs,
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

func (t *DeleteTaskTool) GetUpdatedTasks() []gitdiff.TaskChange {
	return *t.Tasks
}
