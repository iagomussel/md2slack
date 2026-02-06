package webui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"md2slack/internal/gitdiff"
	"md2slack/internal/storage"
)

func TestHandleUpdateTaskPersists(t *testing.T) {
	t.Setenv("MD2SLACK_DB_PATH", filepath.Join(t.TempDir(), "md2slack.db"))
	storage.ResetForTest()

	repo := "repoA"
	date := "2026-02-06"

	_, tasks, err := storage.CreateTask(repo, date, gitdiff.TaskChange{
		TaskType:   "meeting",
		TaskIntent: "meet team",
		Scope:      "core",
	})
	if err != nil {
		t.Fatalf("CreateTask error: %v", err)
	}

	s := &Server{}
	s.Reset(nil, date, repo)
	s.SetTasks(tasks, nil)

	updatedTask := tasks[0]
	updatedTask.TaskIntent = "meet team updated"
	body, _ := json.Marshal(map[string]interface{}{
		"index": 0,
		"task":  updatedTask,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/update-task", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleUpdateTask(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []gitdiff.TaskChange
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 task, got %d", len(resp))
	}
	if resp[0].TaskIntent != "meet team updated" {
		t.Fatalf("expected updated intent, got %q", resp[0].TaskIntent)
	}

	loaded, err := storage.LoadTasks(repo, date)
	if err != nil {
		t.Fatalf("LoadTasks error: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 task loaded, got %d", len(loaded))
	}
	if loaded[0].TaskIntent != "meet team updated" {
		t.Fatalf("expected persisted intent, got %q", loaded[0].TaskIntent)
	}
}
