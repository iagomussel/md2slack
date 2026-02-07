package tools

import (
	"md2slack/internal/gitdiff"

	"github.com/tmc/langchaingo/tools"
)

// TaskTools holds all tools that can modify tasks and provides access to the updated task list
type TaskTools struct {
	tasks              *[]gitdiff.TaskChange
	CreateTask         *CreateTaskTool
	UpdateTask         *UpdateTaskTool
	DeleteTask         *DeleteTaskTool
	AddDetails         *AddDetailsTool
	AddTime            *AddTimeTool
	AddCommitReference *AddCommitReferenceTool
}

// NewTaskTools creates a new set of task manipulation tools initialized with repo/date context
func NewTaskTools(repoName string, date string, currentTasks []gitdiff.TaskChange) *TaskTools {
	tasksCopy := make([]gitdiff.TaskChange, len(currentTasks))
	copy(tasksCopy, currentTasks)

	ptr := &tasksCopy

	return &TaskTools{
		tasks:              ptr,
		CreateTask:         &CreateTaskTool{RepoName: repoName, Date: date, Tasks: ptr},
		UpdateTask:         &UpdateTaskTool{RepoName: repoName, Date: date, Tasks: ptr},
		DeleteTask:         &DeleteTaskTool{RepoName: repoName, Date: date, Tasks: ptr},
		AddDetails:         &AddDetailsTool{Tasks: ptr},
		AddTime:            &AddTimeTool{Tasks: ptr},
		AddCommitReference: &AddCommitReferenceTool{Tasks: ptr},
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
	return *tt.tasks
}

// Find returns a tool by name
func (tt *TaskTools) Find(name string) (tools.Tool, bool) {
	switch name {
	case "create_task":
		return tt.CreateTask, true
	case "update_task", "edit_task":
		return tt.UpdateTask, true
	case "delete_task", "remove_task":
		return tt.DeleteTask, true
	case "add_details":
		return tt.AddDetails, true
	case "add_time":
		return tt.AddTime, true
	case "add_commit_reference":
		return tt.AddCommitReference, true
	default:
		return nil, false
	}
}
