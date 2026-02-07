package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"md2slack/internal/gitdiff"
	"md2slack/internal/llm/tools"
	"strings"
	"time"
)

type LLMOptions struct {
	Provider        string
	Temperature     float64
	TopP            float64
	RepeatPenalty   float64
	ContextSize     int
	ModelName       string
	BaseUrl         string
	Token           string
	RepoName        string
	Date            string
	Quiet           bool
	OnToolLog       func(string)
	OnToolStatus    func(string)
	OnToolLogLegacy func(string) // For backward compatibility if needed, but we should use Agent
	OnLLMLog        func(string)
	OnStreamChunk   func(string)
	OnToolStart     func(toolName string, paramsJSON string)
	OnToolEnd       func(toolName string, resultJSON string)
	OnTasksUpdate   func(tasks []gitdiff.TaskChange)
	Timeout         time.Duration
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolCall struct {
	Tool       string                 `json:"tool"`
	Parameters map[string]interface{} `json:"parameters"`
}

func (tc *ToolCall) UnmarshalJSON(data []byte) error {
	type Alias ToolCall
	var aux Alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.Parameters != nil {
		tc.Tool = aux.Tool
		tc.Parameters = aux.Parameters
		return nil
	}
	var flat map[string]interface{}
	if err := json.Unmarshal(data, &flat); err != nil {
		return err
	}
	tc.Tool = strings.ToLower(castString(flat["tool"]))
	delete(flat, "tool")
	tc.Parameters = flat
	return nil
}

func ExtractCommitIntent(change gitdiff.SemanticChange, commitMsg string, options LLMOptions) (*gitdiff.CommitChange, error) {
	system := readPromptFile("commit_intent_extractor.txt")
	if system == "" {
		return nil, errors.New("prompt file commit_intent_extractor.txt not found")
	}
	prompt := fmt.Sprintf("Commit: %s\nMessage: %s\nSignals: %v", change.CommitHash, commitMsg, change.Signals)
	var out gitdiff.CommitChange
	agent := NewAgent(options, nil)
	err := agent.CallJSON(context.Background(), []OpenAIMessage{{Role: "user", Content: prompt}}, system, &out)
	return &out, err
}

func SummarizeCommit(commit gitdiff.Commit, diff gitdiff.CommitDiff, semantic gitdiff.CommitSemantic, options LLMOptions) (*gitdiff.CommitSummary, error) {
	system := readPromptFile("commit_summarizer.txt")
	if system == "" {
		return nil, errors.New("prompt file commit_summarizer.txt not found")
	}
	semanticJSON, _ := json.MarshalIndent(semantic, "", "  ")
	prompt := fmt.Sprintf("Commit: %s\nMessage: %s\nSemantic (JSON): %s\nRaw Diff:\n%s",
		commit.Hash, commit.Message, string(semanticJSON), diff.Diff)
	var out gitdiff.CommitSummary
	agent := NewAgent(options, nil)
	err := agent.CallJSON(context.Background(), []OpenAIMessage{{Role: "user", Content: prompt}}, system, &out)
	if err == nil && out.CommitHash == "" {
		out.CommitHash = commit.Hash
	}
	return &out, err
}

func SummarizeCommits(commits []gitdiff.Commit, diffs []gitdiff.CommitDiff, semantics []gitdiff.CommitSemantic, options LLMOptions) ([]gitdiff.CommitSummary, error) {
	diffMap := make(map[string]gitdiff.CommitDiff, len(diffs))
	for _, d := range diffs {
		diffMap[d.CommitHash] = d
	}
	semMap := make(map[string]gitdiff.CommitSemantic, len(semantics))
	for _, s := range semantics {
		semMap[s.CommitHash] = s
	}
	var out []gitdiff.CommitSummary
	for _, c := range commits {
		summary, err := SummarizeCommit(c, diffMap[c.Hash], semMap[c.Hash], options)
		if err != nil {
			return out, err
		}
		summary.CommitHash = c.Hash
		out = append(out, *summary)
	}
	return out, nil
}

func SynthesizeTasks(commits []gitdiff.CommitChange, previousTasks []gitdiff.TaskChange, extraContext string, options LLMOptions) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_synthesizer.txt")
	if system == "" {
		return nil, errors.New("prompt file task_synthesizer.txt not found")
	}
	commitsJSON, _ := json.MarshalIndent(commits, "", "  ")
	prevJSON, _ := json.MarshalIndent(previousTasks, "", "  ")
	prompt := fmt.Sprintf("Extra Context: %s\nPrevious Tasks: %s\nCommits: %s", extraContext, string(prevJSON), string(commitsJSON))
	var out []gitdiff.TaskChange
	agent := NewAgent(options, nil)
	err := agent.CallJSON(context.Background(), []OpenAIMessage{{Role: "user", Content: prompt}}, system, &out)
	return out, err
}

func IncorporateExtraContext(extraContext string, options LLMOptions) ([]gitdiff.TaskChange, error) {
	if extraContext == "" {
		return nil, nil
	}
	system := readPromptFile("task_tools_manual.txt")
	if system == "" {
		return nil, errors.New("prompt file task_tools_manual.txt not found")
	}
	prompt := fmt.Sprintf("USER EXTRA CONTEXT:\n%s\n\nPlease create initial tasks based ON THIS CONTEXT. Use the tools provided.", extraContext)
	var currentTasks []gitdiff.TaskChange
	taskTools := tools.NewTaskTools(options.RepoName, options.Date, currentTasks)
	agent := NewAgent(options, taskTools)
	_, _, err := agent.StreamChat([]OpenAIMessage{{Role: "user", Content: prompt}}, system)
	if err != nil {
		return nil, err
	}
	updated := taskTools.GetUpdatedTasks()
	for i := range updated {
		updated[i].IsManual = true
	}
	return updated, nil
}

func GenerateTasksFromContext(commits []gitdiff.Commit, summaries []gitdiff.CommitSummary, semantics []gitdiff.CommitSemantic, extraContext string, options LLMOptions, allowedCommits map[string]struct{}) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_tools_generate.txt")
	if system == "" {
		return nil, errors.New("prompt file task_tools_generate.txt not found")
	}
	commitsJSON, _ := json.MarshalIndent(commits, "", "  ")
	summaryJSON, _ := json.MarshalIndent(summaries, "", "  ")
	semanticJSON, _ := json.MarshalIndent(semantics, "", "  ")
	allowedText := strings.Join(sortedCommitList(allowedCommits), ", ")
	prompt := fmt.Sprintf("Extra Context: %s\nValid Phase 1 Commits: %s\nCommits (JSON): %s\nCommit Summaries (JSON): %s\nSemantic (JSON): %s",
		extraContext, allowedText, string(commitsJSON), string(summaryJSON), string(semanticJSON))

	var currentTasks []gitdiff.TaskChange
	taskTools := tools.NewTaskTools(options.RepoName, options.Date, currentTasks)
	agent := NewAgent(options, taskTools)
	_, _, err := agent.StreamChat([]OpenAIMessage{{Role: "user", Content: prompt}}, system)
	return taskTools.GetUpdatedTasks(), err
}

func ReviewTasks(currentTasks []gitdiff.TaskChange, commits []gitdiff.Commit, summaries []gitdiff.CommitSummary, semantics []gitdiff.CommitSemantic, extraContext string, options LLMOptions, allowedCommits map[string]struct{}) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_tools_review.txt")
	if system == "" {
		return nil, errors.New("prompt file task_tools_review.txt not found")
	}
	commitsJSON, _ := json.MarshalIndent(commits, "", "  ")
	summaryJSON, _ := json.MarshalIndent(summaries, "", "  ")
	semanticJSON, _ := json.MarshalIndent(semantics, "", "  ")
	tasksJSON, _ := json.MarshalIndent(currentTasks, "", "  ")
	allowedText := strings.Join(sortedCommitList(allowedCommits), ", ")
	prompt := fmt.Sprintf("Extra Context: %s\nValid Phase 1 Commits: %s\nCommits (JSON): %s\nCommit Summaries (JSON): %s\nSemantic (JSON): %s\nCurrent Tasks (JSON): %s",
		extraContext, allowedText, string(commitsJSON), string(summaryJSON), string(semanticJSON), string(tasksJSON))

	taskTools := tools.NewTaskTools(options.RepoName, options.Date, currentTasks)
	agent := NewAgent(options, taskTools)
	_, _, err := agent.StreamChat([]OpenAIMessage{{Role: "user", Content: prompt}}, system)
	return taskTools.GetUpdatedTasks(), err
}

func IncorporateCommit(commit gitdiff.CommitChange, currentTasks []gitdiff.TaskChange, manualTasks []gitdiff.TaskChange, extraContext string, options LLMOptions, allowedCommits map[string]struct{}) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_tools.txt")
	if system == "" {
		return nil, errors.New("prompt file task_tools.txt not found")
	}
	var manualSB strings.Builder
	for _, t := range manualTasks {
		manualSB.WriteString(fmt.Sprintf("- %s (%s)\n", t.TaskIntent, t.Scope))
	}
	manualContext := manualSB.String()
	if manualContext == "" {
		manualContext = "(none)"
	}

	var sb strings.Builder
	for i, t := range currentTasks {
		sb.WriteString(fmt.Sprintf("[%d] %s (%s) [%s]\n", i, t.TaskIntent, t.Scope, t.TaskType))
	}
	tasksState := sb.String()
	if tasksState == "" {
		tasksState = "(no commit tasks yet)"
	}

	allowedText := strings.Join(sortedCommitList(allowedCommits), ", ")
	commitJSON, _ := json.MarshalIndent(commit, "", "  ")
	prompt := fmt.Sprintf("Extra Context: %s\nManual Tasks (Context):\n%s\nCurrent State:\n%s\nValid Commits: %s\nNew Commit: %s",
		extraContext, manualContext, tasksState, allowedText, string(commitJSON))

	taskTools := tools.NewTaskTools(options.RepoName, options.Date, currentTasks)
	agent := NewAgent(options, taskTools)
	_, _, err := agent.StreamChat([]OpenAIMessage{{Role: "user", Content: prompt}}, system)
	return taskTools.GetUpdatedTasks(), err
}

func RefineTasksWithPrompt(tasks []gitdiff.TaskChange, userPrompt string, options LLMOptions) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_refiner.txt")
	if system == "" {
		return tasks, nil
	}
	tasksJSON, _ := json.MarshalIndent(tasks, "", "  ")
	prompt := fmt.Sprintf("User request:\n%s\n\nCurrent Task List:\n%s", strings.TrimSpace(userPrompt), string(tasksJSON))
	var out []gitdiff.TaskChange
	agent := NewAgent(options, nil)
	err := agent.CallJSON(context.Background(), []OpenAIMessage{{Role: "user", Content: prompt}}, system, &out)
	if err != nil {
		return tasks, nil
	}
	return PruneTasks(out), nil
}

func SuggestNextActions(tasks []gitdiff.TaskChange, options LLMOptions) ([]string, error) {
	system := readPromptFile("next_actions.txt")
	if system == "" {
		return nil, errors.New("prompt file next_actions.txt not found")
	}
	var cleanTasks []map[string]string
	for _, t := range tasks {
		cleanTasks = append(cleanTasks, map[string]string{"intent": t.TaskIntent, "scope": t.Scope, "type": t.TaskType})
	}
	inputJSON, _ := json.MarshalIndent(cleanTasks, "", "  ")
	prompt := fmt.Sprintf("Tasks synthesized for today:\n%s", string(inputJSON))
	var suggestions []string
	agent := NewAgent(options, nil)
	err := agent.CallJSON(context.Background(), []OpenAIMessage{{Role: "user", Content: prompt}}, system, &suggestions)
	return suggestions, err
}

func GroupTasks(tasks []gitdiff.TaskChange, options LLMOptions) ([]gitdiff.GroupedTask, error) {
	// Simple grouping implementation or LLM-based
	// For now, let's keep it simple or use a default grouping logic if available in gitdiff
	return nil, nil // Placeholder or implement if needed
}

func FallbackTasksFromSummaries(summaries []gitdiff.CommitSummary) []gitdiff.TaskChange {
	var tasks []gitdiff.TaskChange
	for _, s := range summaries {
		if strings.TrimSpace(s.Summary) == "" {
			continue
		}
		tasks = append(tasks, gitdiff.TaskChange{
			TaskType:   "delivery",
			TaskIntent: s.Summary,
			Scope:      s.Area,
			Commits:    []string{s.CommitHash},
			Details:    s.Impact,
		})
	}
	return tasks
}

func RefineTasks(tasks []gitdiff.TaskChange, options LLMOptions) ([]gitdiff.TaskChange, error) {
	return RefineTasksWithPrompt(tasks, "", options)
}

func EditTasksWithAction(tasks []gitdiff.TaskChange, action string, selected []int, options LLMOptions) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_editor.txt")
	if system == "" {
		return tasks, errors.New("prompt file task_editor.txt not found")
	}
	tasksJSON, _ := json.MarshalIndent(tasks, "", "  ")
	selectedJSON, _ := json.MarshalIndent(selected, "", "  ")
	prompt := fmt.Sprintf("Action: %s\nSelected: %s\nTasks: %s", action, string(selectedJSON), string(tasksJSON))
	var out []gitdiff.TaskChange
	agent := NewAgent(options, nil)
	err := agent.CallJSON(context.Background(), []OpenAIMessage{{Role: "user", Content: prompt}}, system, &out)
	if err != nil {
		return tasks, err
	}

	normalized := normalizeSelected(selected, len(tasks))
	if len(normalized) == 0 {
		return tasks, nil
	}

	if len(out) == len(tasks) {
		return out, nil
	}

	// Legacy merge logic (simplified)
	merged := append([]gitdiff.TaskChange(nil), tasks...)
	if len(out) == len(normalized) {
		for i, idx := range normalized {
			merged[idx] = out[i]
		}
	}
	return merged, nil
}
