package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"md2slack/internal/gitdiff"
)

// DeleteTaskTool implements the tools.Tool interface for deleting tasks
type DeleteTaskTool struct {
	Tasks []gitdiff.TaskChange
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
	var params struct {
		Indices []int `json:"indices"`
	}

	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if len(params.Indices) == 0 {
		return "", fmt.Errorf("no indices provided")
	}

	// Validate all indices first
	for _, idx := range params.Indices {
		if idx < 0 || idx >= len(t.Tasks) {
			return "", fmt.Errorf("index %d out of bounds (0-%d)", idx, len(t.Tasks)-1)
		}
	}

	// Sort indices in descending order to delete from end to start
	sortedIndices := make([]int, len(params.Indices))
	copy(sortedIndices, params.Indices)
	for i := 0; i < len(sortedIndices); i++ {
		for j := i + 1; j < len(sortedIndices); j++ {
			if sortedIndices[i] < sortedIndices[j] {
				sortedIndices[i], sortedIndices[j] = sortedIndices[j], sortedIndices[i]
			}
		}
	}

	// Delete from end to start to maintain indices
	deleted := []gitdiff.TaskChange{}
	for _, idx := range sortedIndices {
		deleted = append(deleted, t.Tasks[idx])
		t.Tasks = append(t.Tasks[:idx], t.Tasks[idx+1:]...)
	}

	result := map[string]interface{}{
		"status":  "deleted",
		"count":   len(deleted),
		"deleted": deleted,
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

func (t *DeleteTaskTool) GetUpdatedTasks() []gitdiff.TaskChange {
	return t.Tasks
}
