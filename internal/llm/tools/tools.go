package tools

import (
	"md2slack/internal/gitdiff"

	"github.com/tmc/langchaingo/tools"
)

// TaskTools holds all tools that can modify tasks and provides access to the updated task list
type TaskTools struct {
	CreateTask *CreateTaskTool
	UpdateTask *UpdateTaskTool
	DeleteTask *DeleteTaskTool
	// TODO: Add SplitTask, MergeTasks, SearchCodebase, etc.
}

// NewTaskTools creates a new set of task manipulation tools initialized with repo/date context
func NewTaskTools(repoName string, date string, currentTasks []gitdiff.TaskChange) *TaskTools {
	tasksCopy := make([]gitdiff.TaskChange, len(currentTasks))
	copy(tasksCopy, currentTasks)

	return &TaskTools{
		CreateTask: &CreateTaskTool{RepoName: repoName, Date: date, Tasks: tasksCopy},
		UpdateTask: &UpdateTaskTool{RepoName: repoName, Date: date, Tasks: tasksCopy},
		DeleteTask: &DeleteTaskTool{RepoName: repoName, Date: date, Tasks: tasksCopy},
	}
}

// AsList returns all tools as a slice of tools.Tool interface
func (tt *TaskTools) AsList() []tools.Tool {
	return []tools.Tool{
		tt.CreateTask,
		tt.UpdateTask,
		tt.DeleteTask,
	}
}

// GetUpdatedTasks returns the current state of tasks after all modifications
// It should be called after agent execution to get the final task list
func (tt *TaskTools) GetUpdatedTasks() []gitdiff.TaskChange {
	// All tools share the same task list reference, so we can return from any
	return tt.CreateTask.GetUpdatedTasks()
}

// Find returns a tool by name
func (tt *TaskTools) Find(name string) (tools.Tool, bool) {
	switch name {
	case "create_task":
		return tt.CreateTask, true
	case "update_task":
		return tt.UpdateTask, true
	case "delete_task":
		return tt.DeleteTask, true
	default:
		return nil, false
	}
}
