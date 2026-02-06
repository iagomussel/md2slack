package webui

import (
	"bytes"
	"embed"
	"encoding/json"
	"html"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	goldhtml "github.com/yuin/goldmark/renderer/html"

	"md2slack/internal/gitdiff"
	"md2slack/internal/renderer"
)

//go:embed all:dist
var distFS embed.FS

// indexHTML is used for testing and contains the content of dist/index.html
var indexHTML string

func init() {
	data, err := distFS.ReadFile("dist/index.html")
	if err == nil {
		indexHTML = string(data)
	}
}

type stageStatus string

const (
	stagePending stageStatus = "pending"
	stageRunning stageStatus = "running"
	stageDone    stageStatus = "done"
)

type Stage struct {
	Name      string      `json:"name"`
	Status    stageStatus `json:"status"`
	Note      string      `json:"note"`
	StartedAt time.Time   `json:"started_at,omitempty"`
	Duration  string      `json:"duration,omitempty"`
}

type State struct {
	Repo        string               `json:"repo"`
	Date        string               `json:"date"`
	Stages      []Stage              `json:"stages"`
	Logs        []string             `json:"logs"`
	Errors      []string             `json:"errors"`
	StatusLine  string               `json:"status_line"`
	Report      string               `json:"report"`
	ReportHTML  string               `json:"report_html"`
	Tasks       []gitdiff.TaskChange `json:"tasks"`
	NextActions []string             `json:"next_actions"`
}

type RunRequest struct {
	Date     string `json:"date"`
	RepoPath string `json:"repo_path"`
	Author   string `json:"author"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Server struct {
	addr                string
	mu                  sync.Mutex
	state               State
	stageNames          []string
	runCh               chan RunRequest
	onSend              func(report string) error
	onRefine            func(prompt string, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error)
	onSave              func(date string, tasks []gitdiff.TaskChange, report string) error
	onAction            func(action string, selected []int, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error)
	onChatWithCallbacks func(history []OpenAIMessage, tasks []gitdiff.TaskChange, callbacks ChatCallbacks) ([]gitdiff.TaskChange, string, error)
	onUpdateTask        func(index int, task gitdiff.TaskChange, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error)
	onLoadHistory       func(repo string, date string) ([]gitdiff.TaskChange, string, error)
	onClearTasks        func(repo string, date string) error
}

type ChatCallbacks struct {
	OnStreamChunk func(text string)
	OnToolStart   func(toolName string, paramsJSON string)
	OnToolEnd     func(toolName string, resultJSON string)
}

func Start(addr string, stageNames []string) *Server {
	s := &Server{addr: addr, stageNames: stageNames, runCh: make(chan RunRequest, 1)}
	s.Reset(stageNames, "", "")
	s.startHTTP()
	return s
}

func (s *Server) SetHandlers(onSend func(string) error, onRefine func(string, []gitdiff.TaskChange) ([]gitdiff.TaskChange, error), onSave func(string, []gitdiff.TaskChange, string) error) {
	s.onSend = onSend
	s.onRefine = onRefine
	s.onSave = onSave
}

func (s *Server) SetActionHandler(
	onAction func(action string, selected []int, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error),
	onUpdateTask func(index int, task gitdiff.TaskChange, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error),
) {
	s.onAction = onAction
	s.onUpdateTask = onUpdateTask
}

func (s *Server) SetChatWithCallbacks(
	onChatWithCallbacks func(history []OpenAIMessage, tasks []gitdiff.TaskChange, callbacks ChatCallbacks) ([]gitdiff.TaskChange, string, error),
) {
	s.onChatWithCallbacks = onChatWithCallbacks
}

func (s *Server) SetLoadClearHandlers(
	onLoadHistory func(repo string, date string) ([]gitdiff.TaskChange, string, error),
	onClearTasks func(repo string, date string) error,
) {
	s.onLoadHistory = onLoadHistory
	s.onClearTasks = onClearTasks
}

func (s *Server) RunChannel() <-chan RunRequest {
	return s.runCh
}

func (s *Server) Reset(stageNames []string, date string, repo string) {
	stages := make([]Stage, len(stageNames))
	for i, name := range stageNames {
		stages[i] = Stage{Name: name, Status: stagePending}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Repo = repo
	s.state.Date = date
	s.state.Stages = stages
	s.state.Logs = nil
	s.state.Errors = nil
	s.state.StatusLine = ""
}

func (s *Server) StageStart(idx int, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if idx < 0 || idx >= len(s.state.Stages) {
		return
	}
	stage := &s.state.Stages[idx]
	stage.Status = stageRunning
	if name != "" {
		stage.Name = name
	}
	stage.StartedAt = time.Now()
	stage.Duration = ""
}

func (s *Server) StageDone(idx int, note string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if idx < 0 || idx >= len(s.state.Stages) {
		return
	}
	stage := &s.state.Stages[idx]
	stage.Status = stageDone
	stage.Note = note
	if !stage.StartedAt.IsZero() {
		stage.Duration = time.Since(stage.StartedAt).Truncate(time.Millisecond).String()
	}
}

func (s *Server) Log(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Logs = appendLog(s.state.Logs, line, 300)
}

func (s *Server) Error(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Errors = appendLog(s.state.Errors, line, 20)
	s.state.Logs = appendLog(s.state.Logs, "ERROR: "+line, 300)
}

func (s *Server) Status(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.StatusLine = line
}

func (s *Server) Stop() {
	// No-op for now
}

func (s *Server) GetTasks() []gitdiff.TaskChange {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]gitdiff.TaskChange{}, s.state.Tasks...)
}

func (s *Server) SetTasks(tasks []gitdiff.TaskChange, nextActions []string) {
	s.mu.Lock()
	date := s.state.Date
	s.state.Tasks = tasks
	s.state.NextActions = nextActions
	s.mu.Unlock()

	// Automatically re-generate report whenever tasks change
	report := renderer.RenderReport(date, nil, tasks, nextActions)
	s.SetReport(report)
}

func (s *Server) SetReport(report string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Report = report
	s.state.ReportHTML = renderMarkdown(report)
}

func (s *Server) startHTTP() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/tasks", s.handleTasks)
	mux.HandleFunc("/api/refine", s.handleRefine)
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/action", s.handleAction)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/update-task", s.handleUpdateTask)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/scan-users", s.handleScanUsers)
	mux.HandleFunc("/api/recent-activity", s.handleRecentActivity)
	mux.HandleFunc("/api/git-graph", s.handleGitGraph)
	mux.HandleFunc("/api/load-history", s.handleLoadHistory)
	mux.HandleFunc("/api/clear-tasks", s.handleClearTasks)

	sub, _ := fs.Sub(distFS, "dist")
	fileServer := http.FileServer(http.FS(sub))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Serve from embed if it exists
		f, err := sub.Open(path)
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// Only fallback to index.html for non-asset requests (SPA routing)
		if !strings.Contains(path, ".") {
			indexData, err := distFS.ReadFile("dist/index.html")
			if err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(indexData)
				return
			}
		}

		http.NotFound(w, r)
	})

	srv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}
	go func() {
		log.Printf("webui: listening on http://%s", s.addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("webui: %v", err)
		}
	}()
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Tasks []gitdiff.TaskChange `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	nextActions := s.state.NextActions
	date := s.state.Date
	s.mu.Unlock()

	s.SetTasks(payload.Tasks, nextActions)
	if s.onSave != nil {
		_ = s.onSave(date, payload.Tasks, s.state.Report)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRefine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.onRefine == nil {
		http.Error(w, "refine not configured", http.StatusBadRequest)
		return
	}
	var payload struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	tasks := s.state.Tasks
	nextActions := s.state.NextActions
	date := s.state.Date
	s.mu.Unlock()

	refined, err := s.onRefine(payload.Prompt, tasks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.SetTasks(refined, nextActions)
	if s.onSave != nil {
		_ = s.onSave(date, refined, s.state.Report)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.onSend == nil {
		http.Error(w, "send not configured", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	report := s.state.Report
	s.mu.Unlock()
	if err := s.onSend(report); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.onAction == nil {
		http.Error(w, "action not configured", http.StatusBadRequest)
		return
	}
	var payload struct {
		Action   string `json:"action"`
		Selected []int  `json:"selected"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(payload.Action)
	if action == "" || len(payload.Selected) == 0 {
		http.Error(w, "action and selected are required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	tasks := s.state.Tasks
	nextActions := s.state.NextActions
	date := s.state.Date
	s.mu.Unlock()
	for _, idx := range payload.Selected {
		if idx < 0 || idx >= len(tasks) {
			http.Error(w, "selected index out of range", http.StatusBadRequest)
			return
		}
	}

	updated, err := s.onAction(action, payload.Selected, tasks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.SetTasks(updated, nextActions)
	if s.onSave != nil {
		_ = s.onSave(date, updated, s.state.Report)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload RunRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	payload.Date = strings.TrimSpace(payload.Date)
	payload.RepoPath = strings.TrimSpace(payload.RepoPath)
	payload.Author = strings.TrimSpace(payload.Author)
	if payload.Date == "" {
		http.Error(w, "date is required", http.StatusBadRequest)
		return
	}
	select {
	case s.runCh <- payload:
		s.Reset(s.stageNames, payload.Date, "")
		w.WriteHeader(http.StatusAccepted)
	default:
		http.Error(w, "run already in progress", http.StatusConflict)
	}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		settings, err := loadSettings("")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cwd, _ := os.Getwd()
		isRepo := gitdiff.GetRepoNameAt(cwd) != "unknown"
		before := normalizeList(settings.ProjectPaths)
		settings.ProjectPaths = ensureDefaultProjectPath(before, cwd, isRepo)
		if !equalStringSlices(before, settings.ProjectPaths) {
			_ = saveSettings("", settings)
		}
		projects := buildProjectInfo(settings.ProjectPaths)
		currentProject := ""
		if isRepo {
			currentProject = cwd
		}
		payload := struct {
			Settings       Settings      `json:"settings"`
			Projects       []ProjectInfo `json:"projects"`
			CurrentProject string        `json:"current_project"`
		}{
			Settings:       settings,
			Projects:       projects,
			CurrentProject: currentProject,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
		return
	}
	if r.Method == http.MethodPost {
		var payload struct {
			ProjectPaths []string `json:"project_paths"`
			Usernames    []string `json:"usernames"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		settings := Settings{ProjectPaths: payload.ProjectPaths, Usernames: payload.Usernames}
		cwd, _ := os.Getwd()
		isRepo := gitdiff.GetRepoNameAt(cwd) != "unknown"
		settings.ProjectPaths = ensureDefaultProjectPath(settings.ProjectPaths, cwd, isRepo)
		if err := saveSettings("", settings); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		projects := buildProjectInfo(settings.ProjectPaths)
		currentProject := ""
		if isRepo {
			currentProject = cwd
		}
		resp := struct {
			Settings       Settings      `json:"settings"`
			Projects       []ProjectInfo `json:"projects"`
			CurrentProject string        `json:"current_project"`
		}{Settings: settings, Projects: projects, CurrentProject: currentProject}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleScanUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	path := strings.TrimSpace(payload.Path)
	if path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	users, err := scanUsers(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := struct {
		Usernames []string `json:"usernames"`
	}{Usernames: users}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleRecentActivity(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	dates, err := gitdiff.GetRecentCommitDays(path, 30) // Last 30 days
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := struct {
		Dates []string `json:"dates"`
	}{Dates: dates}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGitGraph(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	commits, err := gitdiff.GetGitGraphData(path, 150)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"commits": commits})
}

func appendLog(list []string, line string, max int) []string {
	if line == "" {
		return list
	}
	list = append(list, line)
	if len(list) > max {
		list = list[len(list)-max:]
	}
	return list
}

func renderMarkdown(md string) string {
	if strings.TrimSpace(md) == "" {
		return "<em>(empty)</em>"
	}
	var buf bytes.Buffer
	mdr := goldmark.New(goldmark.WithRendererOptions(
		goldhtml.WithUnsafe(),
	))
	if err := mdr.Convert([]byte(md), &buf); err != nil {
		return "<pre>" + html.EscapeString(md) + "</pre>"
	}
	return buf.String()
}

func (s *Server) handleLoadHistory(w http.ResponseWriter, r *http.Request) {
	if s.onLoadHistory == nil {
		http.Error(w, "load history not configured", http.StatusBadRequest)
		return
	}
	repo := r.URL.Query().Get("repo")
	date := r.URL.Query().Get("date")
	if repo == "" || date == "" {
		http.Error(w, "repo and date are required", http.StatusBadRequest)
		return
	}

	tasks, report, err := s.onLoadHistory(repo, date)
	if err != nil {
		log.Printf("[handleLoadHistory] Error loading history for repo=%s, date=%s: %v", repo, date, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[handleLoadHistory] Loaded %d tasks for repo=%s, date=%s", len(tasks), repo, date)

	s.mu.Lock()
	s.state.Tasks = tasks
	s.state.Date = date
	s.state.Repo = gitdiff.GetRepoNameAt(repo)
	if report != "" {
		s.state.Report = report
		s.state.ReportHTML = renderMarkdown(report)
	} else {
		s.state.Report = ""
		s.state.ReportHTML = ""
	}
	// Also mark all stages as done if we loaded a report
	if report != "" {
		for i := range s.state.Stages {
			s.state.Stages[i].Status = stageDone
			s.state.Stages[i].Note = "Loaded from history"
		}
	} else {
		// Reset stages
		for i := range s.state.Stages {
			s.state.Stages[i].Status = stagePending
			s.state.Stages[i].Note = ""
		}
	}
	s.mu.Unlock()

	log.Printf("[handleLoadHistory] Returning %d tasks to client", len(tasks))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks)
}

func (s *Server) handleClearTasks(w http.ResponseWriter, r *http.Request) {
	if s.onClearTasks == nil {
		http.Error(w, "clear tasks not configured", http.StatusBadRequest)
		return
	}
	repo := r.URL.Query().Get("repo")
	date := r.URL.Query().Get("date")
	if repo == "" || date == "" {
		http.Error(w, "repo and date are required", http.StatusBadRequest)
		return
	}

	err := s.onClearTasks(repo, date)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	s.state.Tasks = nil
	s.state.Report = ""
	s.state.ReportHTML = ""
	// Reset stages
	for i := range s.state.Stages {
		s.state.Stages[i].Status = stagePending
		s.state.Stages[i].Note = ""
	}
	s.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}
