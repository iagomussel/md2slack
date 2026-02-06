package webui

import (
	"encoding/json"
	"md2slack/internal/gitdiff"
	"net/http"
)

type ChatRequest struct {
	History []OpenAIMessage `json:"history"`
}

type ChatResponse struct {
	Message OpenAIMessage        `json:"message"`
	Tasks   []gitdiff.TaskChange `json:"tasks"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	currentTasks := s.state.Tasks
	s.mu.Unlock()

	if s.onChat == nil {
		http.Error(w, "chat handler not implemented", http.StatusNotImplemented)
		return
	}

	updatedTasks, responseText, err := s.onChat(req.History, currentTasks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.SetTasks(updatedTasks, s.state.NextActions)
	if s.onSave != nil {
		_ = s.onSave(s.state.Date, updatedTasks, s.state.Report)
	}

	resp := ChatResponse{
		Message: OpenAIMessage{Role: "assistant", Content: responseText},
		Tasks:   updatedTasks,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Index int                `json:"index"`
		Task  gitdiff.TaskChange `json:"task"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	currentTasks := s.state.Tasks
	s.mu.Unlock()

	if s.onUpdateTask == nil {
		http.Error(w, "update task handler not implemented", http.StatusNotImplemented)
		return
	}

	updated, err := s.onUpdateTask(req.Index, req.Task, currentTasks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.SetTasks(updated, s.state.NextActions)
	if s.onSave != nil {
		_ = s.onSave(s.state.Date, updated, s.state.Report)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}
