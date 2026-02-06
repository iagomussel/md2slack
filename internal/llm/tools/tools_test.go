package tools

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"md2slack/internal/gitdiff"
	"md2slack/internal/storage"
)

func TestToolsPersistTasks(t *testing.T) {
	t.Setenv("MD2SLACK_DB_PATH", filepath.Join(t.TempDir(), "md2slack.db"))
	storage.ResetForTest()

	repo := "repoA"
	date := "2026-02-06"

	toolset := NewTaskTools(repo, date, nil)

	createInput := map[string]interface{}{
		"title":         "Meet team",
		"intent":        "meet team",
		"time_estimate": "2h",
	}
	b, _ := json.Marshal(createInput)
	if _, err := toolset.CreateTask.Call(nil, string(b)); err != nil {
		t.Fatalf("CreateTask error: %v", err)
	}
	loaded, err := storage.LoadTasks(repo, date)
	if err != nil {
		t.Fatalf("LoadTasks error: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 task, got %d", len(loaded))
	}
	if loaded[0].TaskID == 0 {
		t.Fatalf("expected task_id set")
	}
	id := loaded[0].TaskID

	updateInput := map[string]interface{}{
		"task_id": id,
		"intent":  "meet team updated",
	}
	b, _ = json.Marshal(updateInput)
	if _, err := toolset.UpdateTask.Call(nil, string(b)); err != nil {
		t.Fatalf("UpdateTask error: %v", err)
	}
	loaded, _ = storage.LoadTasks(repo, date)
	if loaded[0].TaskIntent != "meet team updated" {
		t.Fatalf("expected updated intent, got %q", loaded[0].TaskIntent)
	}

	deleteInput := map[string]interface{}{
		"task_ids": []int{id},
	}
	b, _ = json.Marshal(deleteInput)
	if _, err := toolset.DeleteTask.Call(nil, string(b)); err != nil {
		t.Fatalf("DeleteTask error: %v", err)
	}
	loaded, _ = storage.LoadTasks(repo, date)
	if len(loaded) != 0 {
		t.Fatalf("expected 0 tasks after delete, got %d", len(loaded))
	}

	// Ensure tools can still use index fallback
	_, _, _ = storage.CreateTask(repo, date, gitdiff.TaskChange{TaskIntent: "fallback", Scope: "core"})
	loaded, _ = storage.LoadTasks(repo, date)
	if len(loaded) == 0 {
		t.Fatalf("expected task for index fallback")
	}
	updateInput = map[string]interface{}{
		"index":  0,
		"intent": "fallback updated",
	}
	b, _ = json.Marshal(updateInput)
	if _, err := toolset.UpdateTask.Call(nil, string(b)); err != nil {
		t.Fatalf("UpdateTask index fallback error: %v", err)
	}
}
