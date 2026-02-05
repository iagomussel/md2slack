package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type stageStatus string

const (
	stagePending stageStatus = "pending"
	stageRunning stageStatus = "running"
	stageDone    stageStatus = "done"
)

type stage struct {
	Name      string
	Status    stageStatus
	Note      string
	StartedAt time.Time
	Duration  time.Duration
}

type model struct {
	stages    []stage
	logs      []string
	errors    []string
	status    string
	startTime time.Time
	width     int
	height    int
	frame     int
	logVP     viewport.Model
	autoFollow bool
	tailMode  bool
}

type stageStartMsg struct {
	Index int
	Name  string
}

type stageDoneMsg struct {
	Index int
	Note  string
}

type logMsg struct{ Line string }
type errorMsg struct{ Line string }
type statusMsg struct{ Line string }
type finishMsg struct{}
type tickMsg time.Time

type UI struct {
	program *tea.Program
}

func Start(stageNames []string) *UI {
	stages := make([]stage, len(stageNames))
	for i, name := range stageNames {
		stages[i] = stage{Name: name, Status: stagePending}
	}
	m := model{
		stages:    stages,
		startTime: time.Now(),
		logVP:     viewport.New(80, 10),
		autoFollow: true,
		tailMode:  true,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	ui := &UI{program: p}
	go func() {
		_ = p.Start()
	}()
	return ui
}

func (ui *UI) StageStart(idx int, name string) {
	if ui == nil {
		return
	}
	ui.program.Send(stageStartMsg{Index: idx, Name: name})
}

func (ui *UI) StageDone(idx int, note string) {
	if ui == nil {
		return
	}
	ui.program.Send(stageDoneMsg{Index: idx, Note: note})
}

func (ui *UI) Log(line string) {
	if ui == nil {
		return
	}
	ui.program.Send(logMsg{Line: line})
}

func (ui *UI) Error(line string) {
	if ui == nil {
		return
	}
	ui.program.Send(errorMsg{Line: line})
}

func (ui *UI) Status(line string) {
	if ui == nil {
		return
	}
	ui.program.Send(statusMsg{Line: line})
}

func (ui *UI) Stop() {
	if ui == nil {
		return
	}
	ui.program.Send(finishMsg{})
}

func (m model) Init() tea.Cmd {
	return tea.Tick(time.Millisecond*250, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case stageStartMsg:
		if v.Index >= 0 && v.Index < len(m.stages) {
			m.stages[v.Index].Status = stageRunning
			if v.Name != "" {
				m.stages[v.Index].Name = v.Name
			}
			m.stages[v.Index].StartedAt = time.Now()
		}
	case stageDoneMsg:
		if v.Index >= 0 && v.Index < len(m.stages) {
			stage := &m.stages[v.Index]
			stage.Status = stageDone
			stage.Note = v.Note
			if !stage.StartedAt.IsZero() {
				stage.Duration = time.Since(stage.StartedAt)
			}
		}
	case logMsg:
		m.logs = appendLog(m.logs, v.Line, 200)
		m.logVP.SetContent(strings.Join(m.logs, "\n"))
		if m.autoFollow || m.tailMode {
			m.logVP.GotoBottom()
		}
	case errorMsg:
		m.errors = appendLog(m.errors, v.Line, 5)
		m.logs = appendLog(m.logs, "ERROR: "+v.Line, 80)
		m.logVP.SetContent(strings.Join(m.logs, "\n"))
		if m.autoFollow || m.tailMode {
			m.logVP.GotoBottom()
		}
	case statusMsg:
		m.status = v.Line
	case tickMsg:
		m.frame = (m.frame + 1) % len(thinkingFrames)
		return m, tea.Tick(time.Millisecond*250, func(t time.Time) tea.Msg { return tickMsg(t) })
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.logVP.Width = max(40, v.Width-40)
		m.logVP.Height = max(8, v.Height-16)
	case finishMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		switch v.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "t":
			m.tailMode = !m.tailMode
			if m.tailMode {
				m.autoFollow = true
				m.logVP.GotoBottom()
			}
		case "up", "k":
			m.logVP.LineUp(1)
			m.autoFollow = false
			m.tailMode = false
		case "down", "j":
			m.logVP.LineDown(1)
			if m.logVP.AtBottom() {
				m.autoFollow = true
			}
			if m.logVP.AtBottom() {
				m.tailMode = true
			} else {
				m.tailMode = false
			}
		case "pgup":
			m.logVP.LineUp(10)
			m.autoFollow = false
			m.tailMode = false
		case "pgdown":
			m.logVP.LineDown(10)
			if m.logVP.AtBottom() {
				m.autoFollow = true
			}
			if m.logVP.AtBottom() {
				m.tailMode = true
			} else {
				m.tailMode = false
			}
		case "home":
			m.logVP.GotoTop()
			m.autoFollow = false
			m.tailMode = false
		case "end":
			m.logVP.GotoBottom()
			m.autoFollow = true
			m.tailMode = true
		}
	}
	return m, nil
}

func (m model) View() string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	elapsed := time.Since(m.startTime).Truncate(time.Millisecond)
	header := headerStyle.Render(fmt.Sprintf("md2slack • running • %s", elapsed))

	stageBox := renderStages(m.stages)
	thinkingBox := renderThinking(thinkingFrames[m.frame])
	totalWidth := m.width
	if totalWidth <= 0 {
		totalWidth = 120
	}
	statusWidth := 32
	if totalWidth < 80 {
		statusWidth = 24
	}
	logWidth := totalWidth - statusWidth - 4
	if logWidth < 40 {
		logWidth = 40
	}
	m.logVP.Width = logWidth - 4
	if m.logVP.Width < 30 {
		m.logVP.Width = 30
	}
	if m.height > 0 {
		m.logVP.Height = max(8, m.height-16)
	}
	logTitle := "Stream Logs (↑/↓ PgUp/PgDn, t=tail)"
	if m.tailMode {
		logTitle = "Stream Logs (tailing)"
	}
	logBox := renderLogs(logTitle, m.logVP, logWidth)
	statusBox := renderStatus("Status Result", m.status, statusWidth, m.logVP.Height+3)

	top := lipgloss.JoinHorizontal(lipgloss.Top, stageBox, thinkingBox)
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, logBox, statusBox)

	return lipgloss.JoinVertical(lipgloss.Left, header, top, bottom)
}

func renderStages(stages []stage) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("Stages")
	lines := []string{title}
	for i, s := range stages {
		icon := "·"
		color := lipgloss.Color("244")
		switch s.Status {
		case stageRunning:
			icon = "▶"
			color = lipgloss.Color("214")
		case stageDone:
			icon = "✓"
			color = lipgloss.Color("46")
		}
		label := fmt.Sprintf("%d. %s", i+1, s.Name)
		if s.Status == stageDone && s.Duration > 0 {
			label = fmt.Sprintf("%s (%s)", label, s.Duration.Truncate(time.Millisecond))
		}
		if s.Note != "" {
			label = fmt.Sprintf("%s — %s", label, s.Note)
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(color).Render(icon)+" "+label)
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	return box.Render(strings.Join(lines, "\n"))
}

type thinkingFrame struct {
	Title string
	Art   string
	Color lipgloss.Color
}

var thinkingFrames = []thinkingFrame{
	{Title: "AI Thinking ◐", Color: lipgloss.Color("212"), Art: thinkingArt},
	{Title: "AI Thinking ◓", Color: lipgloss.Color("213"), Art: thinkingArt},
	{Title: "AI Thinking ◑", Color: lipgloss.Color("214"), Art: thinkingArt},
	{Title: "AI Thinking ◒", Color: lipgloss.Color("215"), Art: thinkingArt},
}

const thinkingArt = "================================.\n" +
	"     .-.   .-.     .--.                         |\n" +
	"    | OO| | OO|   / _.-' .-.   .-.  .-.   .''.  |\n" +
	"    |   | |   |   \\  '-. '-'   '-'  '-'   '..'  |\n" +
	"    '^^^' '^^^'    '--'                         |\n" +
	"===============.  .-.  .================.  .-.  |\n" +
	"               | |   | |                |  '-'  |\n" +
	"               | |   | |                |       |\n" +
	"               | ':-:' |                |  .-.  |\n" +
	"l42            |  '-'  |                |  '-'  |\n" +
	"==============='       '================'       |"

func renderThinking(frame thinkingFrame) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(frame.Color).Render(frame.Title)
	body := lipgloss.NewStyle().Foreground(frame.Color).Render(frame.Art)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	return box.Render(title + "\n" + body)
}

func renderLogs(title string, vp viewport.Model, width int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	content := titleStyle.Render(title) + "\n" + vp.View()
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(width).Height(vp.Height + 3)
	return box.Render(content)
}

func renderStatus(title string, line string, width int, height int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	content := titleStyle.Render(title)
	if strings.TrimSpace(line) == "" {
		content += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("(empty)")
	} else {
		content += "\n" + line
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(width).Height(height)
	return box.Render(content)
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

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
