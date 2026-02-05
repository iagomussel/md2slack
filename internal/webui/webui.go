package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
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

type Server struct {
	addr     string
	mu       sync.Mutex
	state    State
	runCh    chan string
	onSend   func(report string) error
	onRefine func(prompt string, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error)
	onSave   func(date string, tasks []gitdiff.TaskChange, report string) error
	onAction func(action string, selected []int, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error)
}

func Start(addr string, stageNames []string) *Server {
	s := &Server{addr: addr, runCh: make(chan string, 1)}
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

func (s *Server) RunChannel() <-chan string {
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
	var payload struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	date := strings.TrimSpace(payload.Date)
	if date == "" {
		http.Error(w, "date is required", http.StatusBadRequest)
		return
	}
	select {
	case s.runCh <- date:
		s.mu.Lock()
		s.state.Date = date
		s.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	default:
		http.Error(w, "run already in progress", http.StatusConflict)
	}
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
  <title>md2slack Web UI</title>
  <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="min-h-screen bg-slate-100 text-slate-900">
  <header class="border-b border-slate-200 bg-slate-900 text-white">
    <div class="mx-auto flex max-w-7xl items-center justify-between px-6 py-4">
      <div class="flex items-center gap-4">
        <div class="h-10 w-10 rounded-lg bg-slate-700"></div>
        <div>
          <h1 class="text-lg font-semibold tracking-wide">md2slack Web UI</h1>
          <div id="meta" class="text-xs text-slate-300"></div>
        </div>
      </div>
      <div class="text-xs text-slate-300">web mode</div>
    </div>
  </header>

  <main class="mx-auto max-w-7xl px-6 py-6">
    <div class="grid gap-6 lg:grid-cols-12">
      <section class="lg:col-span-3 space-y-6">
        <div class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <div class="mb-3 text-xs font-semibold uppercase tracking-widest text-slate-500">Stages</div>
          <ul id="stages" class="space-y-2 text-sm"></ul>
        </div>
        <div class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <div class="mb-3 text-xs font-semibold uppercase tracking-widest text-slate-500">Status</div>
          <div id="status" class="text-sm text-slate-700"></div>
        </div>
        <div class="rounded-xl border border-slate-200 bg-slate-900 p-4 shadow-sm">
          <div class="mb-3 text-xs font-semibold uppercase tracking-widest text-slate-300">Logs</div>
          <div id="logs" class="h-64 overflow-auto whitespace-pre-wrap text-xs text-slate-200"></div>
        </div>
      </section>

      <section class="lg:col-span-5 space-y-6">
        <div class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
            <div class="text-xs font-semibold uppercase tracking-widest text-slate-500">Run + Tasks</div>
            <div class="flex items-center gap-2">
              <input type="text" id="date" placeholder="MM-DD-YYYY" class="w-36 rounded-md border border-slate-300 px-2 py-1 text-xs" />
              <button class="rounded-md bg-slate-900 px-3 py-1 text-xs font-semibold text-white" id="run">Run</button>
              <button class="rounded-md bg-slate-700 px-3 py-1 text-xs font-semibold text-white" id="send">Send to Slack</button>
            </div>
          </div>

          <div class="grid gap-4 md:grid-cols-2">
            <div class="space-y-3">
              <div class="text-xs font-semibold uppercase tracking-widest text-slate-500">Task List</div>
              <div id="task-list" class="space-y-2"></div>
              <div class="flex flex-wrap gap-2 pt-2">
                <button class="rounded-md bg-slate-900 px-2 py-1 text-xs font-semibold text-white" id="action-merge">Merge</button>
                <button class="rounded-md bg-slate-700 px-2 py-1 text-xs font-semibold text-white" id="action-split">Split</button>
                <button class="rounded-md bg-slate-700 px-2 py-1 text-xs font-semibold text-white" id="action-longer">Make longer</button>
                <button class="rounded-md bg-slate-700 px-2 py-1 text-xs font-semibold text-white" id="action-shorter">Make shorter</button>
                <button class="rounded-md bg-slate-700 px-2 py-1 text-xs font-semibold text-white" id="action-improve">Improve text</button>
                <button class="rounded-md bg-red-600 px-2 py-1 text-xs font-semibold text-white" id="action-remove">Remove</button>
              </div>
            </div>

            <div class="space-y-3">
              <div class="text-xs font-semibold uppercase tracking-widest text-slate-500">Editor</div>
              <div class="rounded-lg border border-slate-200 p-3">
                <label class="block text-xs text-slate-500">Title</label>
                <input id="edit-intent" class="mt-1 w-full rounded-md border border-slate-300 px-2 py-1 text-sm" />
                <label class="mt-3 block text-xs text-slate-500">Time (hours)</label>
                <input id="edit-hours" type="number" min="0" class="mt-1 w-24 rounded-md border border-slate-300 px-2 py-1 text-sm" />
                <label class="mt-3 block text-xs text-slate-500">Status</label>
                <select id="edit-status" class="mt-1 w-full rounded-md border border-slate-300 px-2 py-1 text-sm">
                  <option value="done">done</option>
                  <option value="inprogress">inprogress</option>
                  <option value="onhold">onhold</option>
                </select>
                <label class="mt-3 block text-xs text-slate-500">Description</label>
                <textarea id="edit-desc" class="mt-1 h-36 w-full rounded-md border border-slate-300 p-2 text-sm"></textarea>
                <div class="mt-3 flex gap-2">
                  <button class="rounded-md bg-slate-900 px-3 py-2 text-xs font-semibold text-white" id="save-task">Save task</button>
                  <button class="rounded-md bg-slate-200 px-3 py-2 text-xs font-semibold text-slate-700" id="new-task">New task</button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <div class="mb-3 text-xs font-semibold uppercase tracking-widest text-slate-500">Tasks (Preview)</div>
          <div id="tasks" class="max-h-80 overflow-auto text-sm text-slate-700"></div>
        </div>
      </section>

      <section class="lg:col-span-4 space-y-6">
        <div class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <div class="mb-3 text-xs font-semibold uppercase tracking-widest text-slate-500">Rendered Preview</div>
          <div id="preview" class="prose max-w-none max-h-[720px] overflow-auto"></div>
        </div>
      </section>
    </div>
  </main>

  <script>
    const state = { editingDate: false, selected: new Set(), tasks: [], activeIndex: null };

    function escapeHtml(str) {
      return str.replace(/[&<>"']/g, s => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[s]));
    }

    function renderStages(stages) {
      const el = document.getElementById('stages');
      el.innerHTML = '';
      stages.forEach((s, i) => {
        const li = document.createElement('li');
        let icon = '*';
        if (s.status === 'running') icon = '>';
        if (s.status === 'done') icon = 'v';
        li.textContent = icon + " " + (i + 1) + ". " + s.name + (s.note ? " - " + s.note : "") + (s.duration ? " (" + s.duration + ")" : "");
        el.appendChild(li);
      });
    }

    function renderLogs(lines) {
      const el = document.getElementById('logs');
      el.textContent = lines.join('\\n');
      el.scrollTop = el.scrollHeight;
    }

    function renderTaskList(tasks) {
      const el = document.getElementById('task-list');
      el.innerHTML = '';
      if (!tasks || tasks.length === 0) {
        el.innerHTML = '<div class="text-xs text-slate-500">(empty)</div>';
        return;
      }
      tasks.forEach((t, i) => {
        const wrap = document.createElement('div');
        wrap.className = "flex items-start gap-2 rounded-md border border-slate-200 p-2 text-xs";
        const cb = document.createElement('input');
        cb.type = 'checkbox';
        cb.checked = state.selected.has(i);
        cb.addEventListener('change', () => {
          if (cb.checked) state.selected.add(i); else state.selected.delete(i);
        });
        const body = document.createElement('div');
        body.className = "flex-1 cursor-pointer";
        body.addEventListener('click', () => selectActive(i));
        const title = document.createElement('div');
        title.className = "text-sm font-semibold";
        title.textContent = t.task_intent || '(no intent)';
        const meta = document.createElement('div');
        meta.className = "text-xs text-slate-500";
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
      let html = '<ul class="space-y-3">';
      tasks.forEach(t => {
        const intent = t.task_intent || '(no intent)';
        const scope = t.scope ? ' [' + t.scope + ']' : '';
        const hours = t.estimated_hours ? ' - ' + t.estimated_hours + 'h' : '';
        const type = t.task_type ? ' (' + t.task_type + ')' : '';
        const status = t.status ? ' [' + t.status + ']' : '';
        html += '<li class="rounded-lg border border-slate-200 p-3">';
        html += '<div class="text-sm font-semibold text-slate-900">' + escapeHtml(intent) + '<span class="text-xs text-slate-500">' + escapeHtml(scope + type + hours + status) + '</span></div>';
        if (t.technical_why) {
          const lines = String(t.technical_why).split('\\n').filter(Boolean);
          if (lines.length) {
            html += '<ul class="mt-2 list-disc pl-5 text-xs text-slate-700">';
            lines.forEach(l => {
              html += '<li>' + escapeHtml(l) + '</li>';
            });
            html += '</ul>';
          }
        }
        if (t.commits && t.commits.length) {
          html += '<div class="mt-2 text-xs text-slate-500">commits: ' + escapeHtml(t.commits.join(', ')) + '</div>';
        }
        html += '</li>';
      });
      html += '</ul>';
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
        dateInput.value = data.date || '';
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

    document.getElementById('run').addEventListener('click', async () => {
      const date = document.getElementById('date').value.trim();
      if (!date) return alert('Digite uma data no formato MM-DD-YYYY');
      await postJSON('/run', { date });
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

    loadState();
    setInterval(loadState, 1500);
  </script>
</body>
</html>`
