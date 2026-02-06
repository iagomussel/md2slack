package storage

import (
	"path/filepath"
	"testing"

	"md2slack/internal/gitdiff"
)

func TestTaskCRUD(t *testing.T) {
	t.Setenv("MD2SLACK_DB_PATH", filepath.Join(t.TempDir(), "md2slack.db"))
	ResetForTest()

	repo := "repoA"
	dateA := "2026-02-06"
	dateB := "2026-02-07"

	idA, tasksA, err := CreateTask(repo, dateA, gitdiff.TaskChange{
		TaskType:   "meeting",
		TaskIntent: "meet team",
		Scope:      "core",
	})
	if err != nil {
		t.Fatalf("CreateTask error: %v", err)
	}
	if idA == "" {
		t.Fatalf("expected task id, got empty string")
	}
	if len(tasksA) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasksA))
	}
	if tasksA[0].ID != idA {
		t.Fatalf("expected task_id %s, got %s", idA, tasksA[0].ID)
	}

	idB, _, err := CreateTask(repo, dateB, gitdiff.TaskChange{
		TaskType:   "delivery",
		TaskIntent: "build feature",
		Scope:      "core",
	})
	if err != nil {
		t.Fatalf("CreateTask dateB error: %v", err)
	}
	if idB == "" {
		t.Fatalf("expected task id for dateB, got empty string")
	}

	tasksA[0].TaskIntent = "meet team updated"
	updated, err := UpdateTask(repo, dateA, idA, tasksA[0])
	if err != nil {
		t.Fatalf("UpdateTask error: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected 1 task after update, got %d", len(updated))
	}
	if updated[0].TaskIntent != "meet team updated" {
		t.Fatalf("expected updated intent, got %q", updated[0].TaskIntent)
	}

	loadedA, err := LoadTasks(repo, dateA)
	if err != nil {
		t.Fatalf("LoadTasks dateA error: %v", err)
	}
	if len(loadedA) != 1 {
		t.Fatalf("expected 1 task loaded for dateA, got %d", len(loadedA))
	}

	loadedB, err := LoadTasks(repo, dateB)
	if err != nil {
		t.Fatalf("LoadTasks dateB error: %v", err)
	}
	if len(loadedB) != 1 {
		t.Fatalf("expected 1 task loaded for dateB, got %d", len(loadedB))
	}

	remaining, err := DeleteTasks(repo, dateA, []string{idA})
	if err != nil {
		t.Fatalf("DeleteTasks error: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected 0 tasks after delete, got %d", len(remaining))
	}
	stillB, err := LoadTasks(repo, dateB)
	if err != nil {
		t.Fatalf("LoadTasks dateB after delete error: %v", err)
	}
	if len(stillB) != 1 {
		t.Fatalf("expected dateB untouched, got %d", len(stillB))
	}
}
