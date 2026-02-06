package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
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

type Server struct {
	addr     string
	mu       sync.Mutex
	state    State
	runCh    chan RunRequest
	onSend   func(report string) error
	onRefine func(prompt string, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error)
	onSave   func(date string, tasks []gitdiff.TaskChange, report string) error
	onAction func(action string, selected []int, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error)
}

func Start(addr string, stageNames []string) *Server {
	s := &Server{addr: addr, runCh: make(chan RunRequest, 1)}
	s.Reset(stageNames, "", "")
	s.startHTTP()
	return s
}

func (s *Server) SetHandlers(onSend func(string) error, onRefine func(string, []gitdiff.TaskChange) ([]gitdiff.TaskChange, error), onSave func(string, []gitdiff.TaskChange, string) error) {
	s.onSend = onSend
	s.onRefine = onRefine
	s.onSave = onSave
}

func (s *Server) SetActionHandler(onAction func(action string, selected []int, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error)) {
	s.onAction = onAction
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
	s.state = State{
		Repo:   repo,
		Date:   date,
		Stages: stages,
		Logs:   nil,
		Errors: nil,
	}
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

func (s *Server) SetTasks(tasks []gitdiff.TaskChange, nextActions []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Tasks = tasks
	s.state.NextActions = nextActions
}

func (s *Server) SetReport(report string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Report = report
	s.state.ReportHTML = renderMarkdown(report)
}

func (s *Server) startHTTP() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/state", s.handleState)
	mux.HandleFunc("/tasks", s.handleTasks)
	mux.HandleFunc("/refine", s.handleRefine)
	mux.HandleFunc("/send", s.handleSend)
	mux.HandleFunc("/run", s.handleRun)
	mux.HandleFunc("/action", s.handleAction)
	mux.HandleFunc("/settings", s.handleSettings)
	mux.HandleFunc("/scan-users", s.handleScanUsers)

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

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
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
	report := renderer.RenderReport(date, nil, payload.Tasks, nextActions)
	s.SetReport(report)
	if s.onSave != nil {
		_ = s.onSave(date, payload.Tasks, report)
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
	report := renderer.RenderReport(date, nil, refined, nextActions)
	s.SetReport(report)

	if s.onSave != nil {
		_ = s.onSave(date, refined, report)
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
	report := renderer.RenderReport(date, nil, updated, nextActions)
	s.SetReport(report)
	if s.onSave != nil {
		_ = s.onSave(date, updated, report)
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
		s.mu.Lock()
		s.state.Date = payload.Date
		s.mu.Unlock()
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
		settings.ProjectPaths = ensureDefaultProjectPath(settings.ProjectPaths, cwd, isRepo)
		projects := buildProjectInfo(settings.ProjectPaths)
		payload := struct {
			Settings Settings      `json:"settings"`
			Projects []ProjectInfo `json:"projects"`
		}{
			Settings: settings,
			Projects: projects,
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
		if err := saveSettings("", settings); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		projects := buildProjectInfo(settings.ProjectPaths)
		resp := struct {
			Settings Settings      `json:"settings"`
			Projects []ProjectInfo `json:"projects"`
		}{Settings: settings, Projects: projects}
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

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ss Web UI</title>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
  <link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@400;500;600&family=Rajdhani:wght@500;600;700&display=swap" rel="stylesheet" />
  <style>
    :root {
      --bg-0: #0b0d11;
      --bg-1: #0f1319;
      --bg-2: #151b23;
      --bg-3: #1b232e;
      --line-1: #293241;
      --line-2: #3a4658;
      --text-0: #f2f6fb;
      --text-1: #cdd6e1;
      --text-2: #9aa6b4;
      --amber-400: #ffc36b;
      --amber-500: #ffb04a;
      --amber-700: #c77812;
      --green-500: #3bd48a;
      --red-500: #f06363;
      --shadow-1: 0 14px 30px rgba(0, 0, 0, 0.45);
      --shadow-2: inset 0 1px 0 rgba(255, 255, 255, 0.04);
      --glow-amber: 0 0 0 1px rgba(255, 176, 74, 0.35), 0 0 18px rgba(255, 176, 74, 0.22);
      --glow-soft: 0 0 0 1px rgba(121, 135, 156, 0.3), 0 0 18px rgba(8, 10, 14, 0.4);
    }

    * {
      box-sizing: border-box;
    }

    body {
      margin: 0;
      font-family: "IBM Plex Sans", system-ui, sans-serif;
      color: var(--text-1);
      background: var(--bg-0);
    }

    .app-shell {
      min-height: 100vh;
      background:
        radial-gradient(1200px 620px at 15% -10%, #1a202a 0%, transparent 60%),
        radial-gradient(900px 500px at 110% -20%, #151a22 0%, transparent 55%),
        linear-gradient(180deg, #0b0d11 0%, #0c1117 100%);
      position: relative;
      overflow: hidden;
    }

    .app-shell::before {
      content: "";
      position: absolute;
      inset: 0;
      background:
        linear-gradient(120deg, rgba(255, 255, 255, 0.02) 0%, transparent 40%),
        repeating-linear-gradient(0deg, rgba(255, 255, 255, 0.02), rgba(255, 255, 255, 0.02) 1px, transparent 1px, transparent 4px);
      opacity: 0.22;
      pointer-events: none;
    }

    .app-header {
      position: relative;
      z-index: 1;
      border-bottom: 1px solid var(--line-1);
      background: rgba(11, 14, 18, 0.86);
      backdrop-filter: blur(8px);
    }

    .app-header-inner {
      max-width: 1240px;
      margin: 0 auto;
      padding: 20px 24px;
      display: flex;
      align-items: center;
      justify-content: space-between;
    }

    .brand {
      display: flex;
      align-items: center;
      gap: 14px;
    }

    .brand-icon {
      width: 44px;
      height: 44px;
      border-radius: 12px;
      background: linear-gradient(135deg, #1d2633, #0f1319);
      border: 1px solid var(--line-1);
      box-shadow: var(--glow-soft);
      position: relative;
    }

    .brand-icon::after {
      content: "";
      position: absolute;
      inset: 8px;
      border-radius: 8px;
      background: radial-gradient(circle at 30% 30%, rgba(255, 176, 74, 0.6), transparent 60%);
      opacity: 0.4;
    }

    .brand h1 {
      margin: 0;
      font-family: "Rajdhani", "IBM Plex Sans", sans-serif;
      font-size: 20px;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--text-0);
    }

    .meta {
      font-size: 12px;
      color: var(--text-2);
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }

    .mode-pill {
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.2em;
      color: var(--amber-400);
      padding: 6px 10px;
      border: 1px solid rgba(255, 176, 74, 0.4);
      border-radius: 999px;
      box-shadow: var(--glow-amber);
    }

    .app-glow {
      height: 2px;
      background: linear-gradient(90deg, transparent, rgba(255, 176, 74, 0.7), transparent);
      opacity: 0.5;
    }

    .app-main {
      position: relative;
      z-index: 1;
      max-width: 1240px;
      margin: 0 auto;
      padding: 26px 24px 40px;
    }

    .app-grid {
      display: grid;
      gap: 24px;
    }

    .flex-1 {
      flex: 1;
    }

    @media (min-width: 1100px) {
      .app-grid {
        grid-template-columns: 300px minmax(0, 1fr) 460px;
      }
    }

    .stack {
      display: flex;
      flex-direction: column;
      gap: 20px;
    }

    .panel {
      background: var(--bg-2);
      border: 1px solid var(--line-1);
      border-radius: 18px;
      padding: 16px;
      box-shadow: var(--shadow-2);
      animation: panelIn 0.6s ease both;
      animation-delay: var(--delay, 0s);
    }

    .panel.workspace {
      background: linear-gradient(180deg, rgba(20, 26, 34, 0.96), rgba(16, 22, 30, 0.98));
      border-color: rgba(255, 176, 74, 0.25);
      box-shadow: var(--shadow-1);
    }

    .workspace-field {
      display: flex;
      flex-direction: column;
      gap: 6px;
      margin-bottom: 12px;
    }

    .workspace-row {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center;
    }

    .panel-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 12px;
      font-family: "Rajdhani", "IBM Plex Sans", sans-serif;
      font-size: 12px;
      letter-spacing: 0.2em;
      text-transform: uppercase;
      color: var(--text-2);
    }

    .panel-header .led {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--amber-500);
      box-shadow: 0 0 8px rgba(255, 176, 74, 0.6);
    }

    .panel-terminal {
      background: #0b0f14;
      border-color: #1f2630;
      box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.02);
    }

    .panel-terminal .panel-header {
      color: #9aa6b4;
    }

    .terminal-body {
      height: 260px;
      overflow: auto;
      white-space: pre-wrap;
      font-family: "IBM Plex Mono", "IBM Plex Sans", monospace;
      font-size: 11px;
      color: #c5ced8;
      padding-right: 8px;
      background:
        repeating-linear-gradient(180deg, rgba(255, 255, 255, 0.02), rgba(255, 255, 255, 0.02) 1px, transparent 1px, transparent 4px);
      border-radius: 12px;
    }

    .stages {
      list-style: none;
      padding: 0;
      margin: 0;
      display: flex;
      flex-direction: column;
      gap: 10px;
    }

    .stage-item {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 8px 10px;
      border-radius: 12px;
      border: 1px solid var(--line-1);
      background: var(--bg-3);
      font-size: 12px;
      color: var(--text-1);
    }

    .stage-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--line-2);
      box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.3);
    }

    .stage-item.stage-running .stage-dot {
      background: var(--amber-500);
      box-shadow: 0 0 10px rgba(255, 176, 74, 0.7);
      animation: pulse 1.4s ease infinite;
    }

    .stage-item.stage-done .stage-dot {
      background: var(--green-500);
      box-shadow: 0 0 8px rgba(59, 212, 138, 0.5);
    }

    .status-line {
      font-size: 13px;
      color: var(--text-0);
      padding: 10px 12px;
      border-radius: 12px;
      border: 1px solid var(--line-1);
      background: linear-gradient(180deg, rgba(24, 32, 42, 0.8), rgba(16, 22, 30, 0.8));
    }

    .panel-grid {
      display: grid;
      gap: 16px;
    }

    @media (min-width: 820px) {
      .panel-grid {
        grid-template-columns: repeat(2, minmax(0, 1fr));
      }
    }

    .toolbar {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
    }

    .control-row {
      display: flex;
      align-items: center;
      gap: 10px;
      flex-wrap: wrap;
    }

    .input,
    .select,
    .textarea {
      background: var(--bg-3);
      border: 1px solid var(--line-1);
      border-radius: 10px;
      color: var(--text-0);
      font-size: 12px;
      padding: 8px 10px;
      outline: none;
      transition: border 0.2s ease, box-shadow 0.2s ease;
    }

    .input:focus,
    .select:focus,
    .textarea:focus {
      border-color: var(--amber-500);
      box-shadow: var(--glow-amber);
    }

    .textarea {
      width: 100%;
      min-height: 140px;
      resize: vertical;
      font-size: 13px;
    }

    .btn {
      border-radius: 10px;
      border: 1px solid transparent;
      font-size: 11px;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      font-weight: 600;
      padding: 8px 14px;
      cursor: pointer;
      transition: transform 0.2s ease, box-shadow 0.2s ease, border 0.2s ease, background 0.2s ease;
    }

    .btn:active {
      transform: translateY(1px);
    }

    .btn-primary {
      background: linear-gradient(135deg, #ffb04a, #ff9828);
      color: #1a1207;
      box-shadow: 0 6px 18px rgba(255, 152, 40, 0.35);
    }

    .btn-secondary {
      background: var(--bg-3);
      border-color: var(--line-2);
      color: var(--text-1);
    }

    .btn-secondary:hover {
      border-color: var(--amber-500);
      color: var(--amber-400);
      box-shadow: var(--glow-amber);
    }

    .btn-danger {
      background: rgba(240, 99, 99, 0.2);
      border-color: rgba(240, 99, 99, 0.5);
      color: #ffb3b3;
    }

    .task-list {
      display: flex;
      flex-direction: column;
      gap: 10px;
    }

    .task-row {
      display: flex;
      gap: 10px;
      align-items: flex-start;
      border-radius: 12px;
      border: 1px solid var(--line-1);
      padding: 10px;
      background: var(--bg-3);
      cursor: pointer;
      transition: border 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease;
    }

    .task-row:hover {
      transform: translateY(-1px);
      border-color: var(--line-2);
    }

    .task-row.active {
      border-color: var(--amber-500);
      box-shadow: var(--glow-amber);
    }

    .task-meta {
      font-size: 11px;
      color: var(--text-2);
    }

    .task-title {
      font-weight: 600;
      color: var(--text-0);
      font-size: 13px;
    }

    .checkbox {
      appearance: none;
      width: 16px;
      height: 16px;
      border-radius: 4px;
      border: 1px solid var(--line-2);
      background: #0f1319;
      position: relative;
      margin-top: 2px;
      cursor: pointer;
    }

    .checkbox:checked {
      background: var(--amber-500);
      border-color: var(--amber-500);
      box-shadow: 0 0 10px rgba(255, 176, 74, 0.5);
    }

    .checkbox:checked::after {
      content: "";
      position: absolute;
      width: 4px;
      height: 8px;
      border: 2px solid #1a1207;
      border-top: 0;
      border-left: 0;
      transform: rotate(45deg);
      left: 5px;
      top: 1px;
    }

    .action-grid {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-top: 12px;
    }

    .editor-card {
      border-radius: 14px;
      border: 1px solid var(--line-1);
      padding: 14px;
      background: linear-gradient(180deg, rgba(22, 28, 36, 0.9), rgba(15, 20, 27, 0.95));
    }

    .editor-card label {
      display: block;
      margin-top: 12px;
      font-size: 11px;
      letter-spacing: 0.14em;
      text-transform: uppercase;
      color: var(--text-2);
    }

    .editor-card label:first-of-type {
      margin-top: 0;
    }

    .editor-actions {
      display: flex;
      gap: 10px;
      margin-top: 14px;
    }

    .preview-body {
      max-height: 720px;
      overflow: auto;
      font-size: 13px;
      line-height: 1.6;
      color: var(--text-1);
    }

    .slack-preview {
      background: #0f1216;
      border: 1px solid #2a313b;
      border-radius: 14px;
      padding: 18px 18px 22px;
      min-height: 520px;
      max-height: 820px;
      box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.02);
      font-family: "Lato", "IBM Plex Sans", sans-serif;
      color: #e9eef5;
    }

    .slack-preview h1,
    .slack-preview h2,
    .slack-preview h3 {
      color: #f8fafc;
      letter-spacing: 0.02em;
    }

    .slack-preview ul {
      padding-left: 20px;
    }

    .slack-preview li {
      margin-bottom: 6px;
    }

    .slack-preview code {
      background: rgba(255, 176, 74, 0.12);
      color: #ffd39a;
      padding: 1px 4px;
    }

    .slack-preview strong {
      color: #ffffff;
    }

    .slack-preview em {
      color: #b8c3d0;
    }

    .preview-body h1,
    .preview-body h2,
    .preview-body h3 {
      color: var(--text-0);
      font-family: "Rajdhani", "IBM Plex Sans", sans-serif;
      letter-spacing: 0.04em;
      margin-top: 1.2em;
    }

    .preview-body a {
      color: var(--amber-400);
      text-decoration: none;
    }

    .preview-body code {
      background: rgba(255, 176, 74, 0.1);
      padding: 2px 4px;
      border-radius: 4px;
    }

    .task-preview-list {
      display: flex;
      flex-direction: column;
      gap: 12px;
    }

    .task-preview-item {
      border-radius: 14px;
      border: 1px solid var(--line-1);
      padding: 12px;
      background: var(--bg-3);
    }

    .task-preview-header {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
      align-items: center;
      justify-content: space-between;
      font-size: 13px;
      color: var(--text-0);
      font-weight: 600;
    }

    .task-preview-meta {
      font-size: 11px;
      color: var(--text-2);
    }

    .chip {
      display: inline-flex;
      align-items: center;
      padding: 2px 8px;
      border-radius: 999px;
      border: 1px solid var(--line-2);
      font-size: 10px;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--text-2);
    }

    .chip.status-done {
      border-color: rgba(59, 212, 138, 0.4);
      color: #8cebc0;
    }

    .chip.status-inprogress {
      border-color: rgba(255, 176, 74, 0.5);
      color: var(--amber-400);
    }

    .chip.status-onhold {
      border-color: rgba(240, 99, 99, 0.5);
      color: #ffb3b3;
    }

    @keyframes panelIn {
      from { opacity: 0; transform: translateY(12px); }
      to { opacity: 1; transform: translateY(0); }
    }

    @keyframes pulse {
      0% { box-shadow: 0 0 6px rgba(255, 176, 74, 0.4); }
      50% { box-shadow: 0 0 12px rgba(255, 176, 74, 0.9); }
      100% { box-shadow: 0 0 6px rgba(255, 176, 74, 0.4); }
    }

    @media (max-width: 720px) {
      .app-header-inner {
        flex-direction: column;
        align-items: flex-start;
        gap: 12px;
      }
      .mode-pill {
        align-self: flex-start;
      }
    }

    .modal-backdrop {
      position: fixed;
      inset: 0;
      background: rgba(5, 6, 8, 0.72);
      display: none;
      align-items: center;
      justify-content: center;
      z-index: 50;
    }

    .modal-backdrop.active {
      display: flex;
    }

    .modal {
      width: min(480px, 92vw);
      background: #121820;
      border: 1px solid var(--line-1);
      border-radius: 16px;
      padding: 18px;
      box-shadow: var(--shadow-1);
    }

    .modal h3 {
      margin: 0 0 10px;
      font-family: "Rajdhani", "IBM Plex Sans", sans-serif;
      text-transform: uppercase;
      letter-spacing: 0.16em;
      font-size: 12px;
      color: var(--text-1);
    }
  </style>
</head>
<body>
  <div class="app-shell">
    <header class="app-header">
      <div class="app-header-inner">
        <div class="brand">
          <div class="brand-icon"></div>
          <div>
            <h1>ss Web UI</h1>
            <div id="meta" class="meta"></div>
          </div>
        </div>
        <div class="mode-pill">web mode</div>
      </div>
      <div class="app-glow"></div>
    </header>

    <main class="app-main">
      <div class="app-grid">
        <section class="stack">
          <div class="panel workspace" style="--delay: 0.02s">
            <div class="panel-header">Workspace <span class="led"></span></div>
            <div class="workspace-field">
              <label class="meta">Project</label>
              <div class="workspace-row">
                <select id="project-select" class="select" data-testid="project-select"></select>
                <button class="btn btn-secondary" id="add-project" data-testid="add-project">Add path</button>
              </div>
            </div>
            <div class="workspace-field">
              <label class="meta">Git Username</label>
              <div class="workspace-row">
                <select id="user-select" class="select" data-testid="user-select"></select>
                <button class="btn btn-secondary" id="scan-users" data-testid="scan-users">Scan</button>
              </div>
              <div class="workspace-row">
                <input id="new-user" class="input" placeholder="Add username" data-testid="new-user" />
                <button class="btn btn-secondary" id="add-user" data-testid="add-user">Add</button>
              </div>
            </div>
          </div>
          <div class="panel" style="--delay: 0.05s">
            <div class="panel-header">Stages <span class="led"></span></div>
            <ul id="stages" class="stages"></ul>
          </div>
          <div class="panel" style="--delay: 0.1s">
            <div class="panel-header">Status <span class="led"></span></div>
            <div id="status" class="status-line"></div>
          </div>
          <div class="panel panel-terminal" style="--delay: 0.15s">
            <div class="panel-header">Logs <span class="led"></span></div>
            <div id="logs" class="terminal-body"></div>
          </div>
        </section>

        <section class="stack">
          <div class="panel" style="--delay: 0.08s">
            <div class="toolbar">
              <div class="panel-header">Run + Tasks <span class="led"></span></div>
              <div class="control-row">
                <input type="date" id="date" class="input" data-testid="date-input" />
                <button class="btn btn-primary" id="run">Run</button>
                <button class="btn btn-secondary" id="send">Send to Slack</button>
              </div>
            </div>

            <div class="panel-grid">
              <div class="stack">
                <div class="panel-header">Task List <span class="led"></span></div>
                <div id="task-list" class="task-list"></div>
                <div class="action-grid">
                  <button class="btn btn-primary" id="action-merge">Merge</button>
                  <button class="btn btn-secondary" id="action-split">Split</button>
                  <button class="btn btn-secondary" id="action-longer">Make longer</button>
                  <button class="btn btn-secondary" id="action-shorter">Make shorter</button>
                  <button class="btn btn-secondary" id="action-improve">Improve text</button>
                  <button class="btn btn-danger" id="action-remove">Remove</button>
                </div>
              </div>

              <div class="stack">
                <div class="panel-header">Editor <span class="led"></span></div>
                <div class="editor-card">
                  <label>Title</label>
                  <input id="edit-intent" class="input" />
                  <label>Time (hours)</label>
                  <input id="edit-hours" type="number" min="0" class="input" />
                  <label>Status</label>
                  <select id="edit-status" class="select">
                    <option value="done">done</option>
                    <option value="inprogress">inprogress</option>
                    <option value="onhold">onhold</option>
                  </select>
                  <label>Description</label>
                  <textarea id="edit-desc" class="textarea"></textarea>
                  <div class="editor-actions">
                    <button class="btn btn-primary" id="save-task">Save task</button>
                    <button class="btn btn-secondary" id="new-task">New task</button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="panel" style="--delay: 0.12s">
            <div class="panel-header">Tasks (Preview) <span class="led"></span></div>
            <div id="tasks" class="preview-body"></div>
          </div>
        </section>

        <section class="stack">
          <div class="panel" style="--delay: 0.18s">
            <div class="panel-header">Rendered Preview <span class="led"></span></div>
            <div id="preview" class="preview-body slack-preview"></div>
          </div>
        </section>
      </div>
    </main>
  </div>

  <div class="modal-backdrop" id="project-modal">
    <div class="modal">
      <h3>Add Project Path</h3>
      <input id="project-path-input" class="input" placeholder="/path/to/repo" data-testid="project-path-input" />
      <div class="editor-actions">
        <button class="btn btn-primary" id="save-project-path">Save</button>
        <button class="btn btn-secondary" id="cancel-project-path">Cancel</button>
      </div>
    </div>
  </div>

  <script>
    const state = {
      editingDate: false,
      selected: new Set(),
      tasks: [],
      activeIndex: null,
      settings: { project_paths: [], usernames: [] },
      projects: [],
      selectedProject: '',
      selectedUser: '',
    };

    function escapeHtml(str) {
      return str.replace(/[&<>"']/g, s => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;','\'':'&#39;'}[s]));
    }

    function renderStages(stages) {
      const el = document.getElementById('stages');
      el.innerHTML = '';
      stages.forEach((s, i) => {
        const li = document.createElement('li');
        li.className = "stage-item stage-" + (s.status || 'pending');
        const dot = document.createElement('span');
        dot.className = "stage-dot";
        const text = document.createElement('span');
        text.textContent = (i + 1) + ". " + s.name + (s.note ? " - " + s.note : "") + (s.duration ? " (" + s.duration + ")" : "");
        li.appendChild(dot);
        li.appendChild(text);
        el.appendChild(li);
      });
    }

    function renderLogs(lines) {
      const el = document.getElementById('logs');
      el.textContent = lines.join('\n');
      el.scrollTop = el.scrollHeight;
    }

    function toISODate(mdy) {
      const match = /^(\d{2})-(\d{2})-(\d{4})$/.exec(mdy || '');
      if (!match) return mdy;
      return match[3] + "-" + match[1] + "-" + match[2];
    }

    function toMDYDate(iso) {
      const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso || '');
      if (!match) return iso;
      return match[2] + "-" + match[3] + "-" + match[1];
    }

    function renderWorkspace() {
      const projectSelect = document.getElementById('project-select');
      const userSelect = document.getElementById('user-select');
      projectSelect.innerHTML = '';
      const projects = state.projects || [];
      if (projects.length === 0) {
        const opt = document.createElement('option');
        opt.value = '';
        opt.textContent = '(no projects)';
        projectSelect.appendChild(opt);
      } else {
        projects.forEach(p => {
          const opt = document.createElement('option');
          opt.value = p.path;
          opt.textContent = p.name + " — " + p.path;
          projectSelect.appendChild(opt);
        });
      }
      if (!state.selectedProject && projects.length) {
        state.selectedProject = projects[0].path;
      }
      projectSelect.value = state.selectedProject || '';

      userSelect.innerHTML = '';
      const users = state.settings.usernames || [];
      if (users.length === 0) {
        const opt = document.createElement('option');
        opt.value = '';
        opt.textContent = '(no usernames)';
        userSelect.appendChild(opt);
      } else {
        users.forEach(u => {
          const opt = document.createElement('option');
          opt.value = u;
          opt.textContent = u;
          userSelect.appendChild(opt);
        });
      }
      if (!state.selectedUser && users.length) {
        state.selectedUser = users[0];
      }
      userSelect.value = state.selectedUser || '';
    }

    async function loadSettings() {
      const res = await fetch('/settings');
      if (!res.ok) return;
      const data = await res.json();
      state.settings = data.settings || { project_paths: [], usernames: [] };
      state.projects = data.projects || [];
      renderWorkspace();
    }

    async function saveSettings() {
      const payload = {
        project_paths: state.settings.project_paths || [],
        usernames: state.settings.usernames || [],
      };
      const data = await postJSON('/settings', payload);
      if (data) {
        state.settings = data.settings || state.settings;
        state.projects = data.projects || state.projects;
        renderWorkspace();
      }
    }

    function renderTaskList(tasks) {
      const el = document.getElementById('task-list');
      el.innerHTML = '';
      if (!tasks || tasks.length === 0) {
        el.innerHTML = '<div class="task-meta">(empty)</div>';
        return;
      }
      tasks.forEach((t, i) => {
        const wrap = document.createElement('div');
        wrap.className = "task-row" + (state.activeIndex === i ? " active" : "");
        const cb = document.createElement('input');
        cb.type = 'checkbox';
        cb.className = 'checkbox';
        cb.checked = state.selected.has(i);
        cb.addEventListener('change', () => {
          if (cb.checked) state.selected.add(i); else state.selected.delete(i);
        });
        const body = document.createElement('div');
        body.className = "flex-1";
        body.addEventListener('click', () => selectActive(i));
        const title = document.createElement('div');
        title.className = "task-title";
        title.textContent = t.task_intent || '(no intent)';
        const meta = document.createElement('div');
        meta.className = "task-meta";
        meta.textContent = (t.status || 'done') + (t.estimated_hours ? " - " + t.estimated_hours + "h" : "");
        body.appendChild(title);
        body.appendChild(meta);
        wrap.appendChild(cb);
        wrap.appendChild(body);
        el.appendChild(wrap);
      });
    }

    function renderTasksPreview(tasks) {
      const el = document.getElementById('tasks');
      if (!tasks || tasks.length === 0) {
        el.innerHTML = '<em>(empty)</em>';
        return;
      }
      let html = '<div class="task-preview-list">';
      tasks.forEach(t => {
        const intent = t.task_intent || '(no intent)';
        const scope = t.scope ? ' [' + t.scope + ']' : '';
        const hours = t.estimated_hours ? ' - ' + t.estimated_hours + 'h' : '';
        const type = t.task_type ? ' (' + t.task_type + ')' : '';
        const status = t.status ? ' [' + t.status + ']' : '';
        const statusClass = t.status ? ' status-' + t.status : '';
        html += '<div class="task-preview-item">';
        html += '<div class="task-preview-header"><span>' + escapeHtml(intent) + '</span><span class="chip' + statusClass + '">' + escapeHtml(t.status || 'done') + '</span></div>';
        html += '<div class="task-preview-meta">' + escapeHtml(scope + type + hours + status) + '</div>';
        if (t.technical_why) {
          const lines = String(t.technical_why).split('\n').filter(Boolean);
          if (lines.length) {
            html += '<ul class="task-preview-meta" style="margin-top: 8px; padding-left: 18px;">';
            lines.forEach(l => {
              html += '<li>' + escapeHtml(l) + '</li>';
            });
            html += '</ul>';
          }
        }
        if (t.commits && t.commits.length) {
          html += '<div class="task-preview-meta" style="margin-top: 8px;">commits: ' + escapeHtml(t.commits.join(', ')) + '</div>';
        }
        html += '</div>';
      });
      html += '</div>';
      el.innerHTML = html;
    }

    function selectActive(index) {
      state.activeIndex = index;
      const t = state.tasks[index];
      if (!t) return;
      document.getElementById('edit-intent').value = t.task_intent || '';
      document.getElementById('edit-hours').value = t.estimated_hours || '';
      document.getElementById('edit-status').value = t.status || 'done';
      document.getElementById('edit-desc').value = t.technical_why || '';
      renderTaskList(state.tasks);
    }

    async function loadState() {
      const res = await fetch('/state');
      const data = await res.json();
      document.getElementById('meta').textContent = data.repo && data.date ? (data.repo + " - " + data.date) : '';
      renderStages(data.stages || []);
      renderLogs(data.logs || []);
      state.tasks = data.tasks || [];
      renderTaskList(state.tasks);
      renderTasksPreview(state.tasks);
      const dateInput = document.getElementById('date');
      if (!state.editingDate && dateInput) {
        dateInput.value = toISODate(data.date || '');
      }
      document.getElementById('status').textContent = data.status_line || '';
      document.getElementById('preview').innerHTML = data.report_html || '';
      if (state.activeIndex === null && state.tasks.length > 0) {
        selectActive(0);
      }
    }

    async function postJSON(url, body) {
      const res = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const text = await res.text();
        alert(text || 'Request failed');
        return null;
      }
      if (res.headers.get('content-type') && res.headers.get('content-type').includes('application/json')) {
        return await res.json();
      }
      return null;
    }

    function saveTasks() {
      return postJSON('/tasks', { tasks: state.tasks });
    }

    document.getElementById('date').addEventListener('focus', () => state.editingDate = true);
    document.getElementById('date').addEventListener('blur', () => state.editingDate = false);

    document.getElementById('project-select').addEventListener('change', (e) => {
      state.selectedProject = e.target.value;
    });

    document.getElementById('user-select').addEventListener('change', (e) => {
      state.selectedUser = e.target.value;
    });

    document.getElementById('add-project').addEventListener('click', () => {
      document.getElementById('project-modal').classList.add('active');
      document.getElementById('project-path-input').value = '';
    });

    document.getElementById('cancel-project-path').addEventListener('click', () => {
      document.getElementById('project-modal').classList.remove('active');
    });

    document.getElementById('save-project-path').addEventListener('click', async () => {
      const value = document.getElementById('project-path-input').value.trim();
      if (!value) return;
      state.settings.project_paths = state.settings.project_paths || [];
      state.settings.project_paths.push(value);
      await saveSettings();
      state.selectedProject = value;
      document.getElementById('project-modal').classList.remove('active');
    });

    document.getElementById('add-user').addEventListener('click', async () => {
      const value = document.getElementById('new-user').value.trim();
      if (!value) return;
      state.settings.usernames = state.settings.usernames || [];
      state.settings.usernames.push(value);
      await saveSettings();
      state.selectedUser = value;
      document.getElementById('new-user').value = '';
    });

    document.getElementById('scan-users').addEventListener('click', async () => {
      if (!state.selectedProject) return alert('Select a project path first.');
      const res = await postJSON('/scan-users', { path: state.selectedProject });
      if (res && res.usernames) {
        state.settings.usernames = (state.settings.usernames || []).concat(res.usernames);
        await saveSettings();
      }
    });

    document.getElementById('run').addEventListener('click', async () => {
      const dateValue = document.getElementById('date').value.trim();
      const date = toMDYDate(dateValue);
      if (!date) return alert('Digite uma data no formato MM-DD-YYYY');
      await postJSON('/run', { date, repo_path: state.selectedProject, author: state.selectedUser });
    });
    document.getElementById('send').addEventListener('click', async () => {
      await postJSON('/send', {});
    });

    document.getElementById('save-task').addEventListener('click', async () => {
      if (state.activeIndex === null) return;
      const t = state.tasks[state.activeIndex];
      if (!t) return;
      t.task_intent = document.getElementById('edit-intent').value.trim();
      const hours = parseInt(document.getElementById('edit-hours').value, 10);
      t.estimated_hours = Number.isNaN(hours) ? null : hours;
      t.status = document.getElementById('edit-status').value;
      t.technical_why = document.getElementById('edit-desc').value.trim();
      await saveTasks();
      renderTaskList(state.tasks);
      renderTasksPreview(state.tasks);
    });

    document.getElementById('new-task').addEventListener('click', async () => {
      const t = {
        task_type: 'delivery',
        task_intent: 'New task',
        scope: '',
        commits: [],
        estimated_hours: 1,
        technical_why: '',
        status: 'done'
      };
      state.tasks.push(t);
      state.activeIndex = state.tasks.length - 1;
      await saveTasks();
      renderTaskList(state.tasks);
      renderTasksPreview(state.tasks);
      selectActive(state.activeIndex);
    });

    document.getElementById('action-remove').addEventListener('click', async () => {
      const selected = Array.from(state.selected).sort((a,b) => b - a);
      if (selected.length === 0) return alert('Selecione tasks para remover.');
      selected.forEach(i => state.tasks.splice(i, 1));
      state.selected.clear();
      state.activeIndex = null;
      await saveTasks();
      renderTaskList(state.tasks);
      renderTasksPreview(state.tasks);
    });

    async function runAction(action) {
      const selected = Array.from(state.selected);
      if (selected.length === 0) return alert('Selecione tasks primeiro.');
      if (action === 'split_task' && selected.length !== 1) return alert('Selecione apenas 1 task para split.');
      if (action === 'merge_tasks' && selected.length < 2) return alert('Selecione 2+ tasks para merge.');
      const updated = await postJSON('/action', { action, selected });
      if (updated) {
        state.tasks = updated;
        state.selected.clear();
        renderTaskList(state.tasks);
        renderTasksPreview(state.tasks);
        state.activeIndex = null;
      }
    }

    document.getElementById('action-merge').addEventListener('click', () => runAction('merge_tasks'));
    document.getElementById('action-split').addEventListener('click', () => runAction('split_task'));
    document.getElementById('action-longer').addEventListener('click', () => runAction('make_longer'));
    document.getElementById('action-shorter').addEventListener('click', () => runAction('make_shorter'));
    document.getElementById('action-improve').addEventListener('click', () => runAction('improve_text'));

    loadSettings();
    loadState();
    setInterval(loadState, 1500);
  </script>
</body>
</html>`
