package webui

import (
	"encoding/json"
	"log"
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

	log.Printf("[handleChat] Received chat request with %d messages", len(req.History))

	s.mu.Lock()
	currentTasks := s.state.Tasks
	s.mu.Unlock()

	if s.onChat == nil {
		log.Printf("[handleChat] ERROR: onChat handler is nil")
		http.Error(w, "chat handler not implemented", http.StatusNotImplemented)
		return
	}

	// Set up SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send events as they happen
	sendEvent := func(eventType string, data interface{}) {
		jsonData, _ := json.Marshal(data)
		eventMsg := "event: " + eventType + "\ndata: " + string(jsonData) + "\n\n"
		_, _ = w.Write([]byte(eventMsg))
		flusher.Flush()
		log.Printf("[handleChat] Sent event: %s", eventType)
	}

	// Create streaming callbacks
	callbacks := ChatCallbacks{
		OnToolStart: func(toolName string, paramsJSON string) {
			sendEvent("tool_start", map[string]interface{}{
				"tool":   toolName,
				"params": json.RawMessage(paramsJSON),
			})
		},
		OnToolEnd: func(toolName string, resultJSON string) {
			sendEvent("tool_end", map[string]interface{}{
				"tool":   toolName,
				"result": json.RawMessage(resultJSON),
			})
		},
	}

	log.Printf("[handleChat] Calling chat handler with %d current tasks", len(currentTasks))

	var updatedTasks []gitdiff.TaskChange
	var responseText string
	var err error

	// Use callbacks version if available
	if s.onChatWithCallbacks != nil {
		log.Printf("[handleChat] Using onChatWithCallbacks")
		updatedTasks, responseText, err = s.onChatWithCallbacks(req.History, currentTasks, callbacks)
	} else {
		log.Printf("[handleChat] Using legacy onChat (no callbacks)")
		updatedTasks, responseText, err = s.onChat(req.History, currentTasks)
	}

	if err != nil {
		log.Printf("[handleChat] ERROR: chat handler returned error: %v", err)
		sendEvent("error", map[string]string{"message": err.Error()})
		return
	}

	log.Printf("[handleChat] chat returned: %d tasks, response length: %d", len(updatedTasks), len(responseText))

	s.SetTasks(updatedTasks, s.state.NextActions)
	if s.onSave != nil {
		_ = s.onSave(s.state.Date, updatedTasks, s.state.Report)
	}

	// Send final response
	sendEvent("message", map[string]interface{}{
		"text":  responseText,
		"tasks": updatedTasks,
	})
	sendEvent("done", map[string]string{"status": "complete"})
	log.Printf("[handleChat] Chat completed successfully")
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
