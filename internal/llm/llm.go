package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"md2slack/internal/gitdiff"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
)

type LLMOptions struct {
	Provider      string
	Temperature   float64
	TopP          float64
	RepeatPenalty float64
	ContextSize   int
	ModelName     string
	BaseUrl       string
	Token         string
	Debug         bool
	Quiet         bool
	OnToolLog     func(string)
	OnToolStatus  func(string)
	OnLLMLog      func(string)
	Timeout       time.Duration
	Heartbeat     time.Duration
}

// OpenAIMessage is now shared with webui, but we keep it here for internal use.
// We should probably move this to a shared types package, but for now we'll alias or duplicate if needed.
// IMPORTANT: webui.OpenAIMessage and llm.OpenAIMessage must be compatible in structure.
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolCall struct {
	Tool       string                 `json:"tool"`
	Parameters map[string]interface{} `json:"parameters"`
}

func getNativeTools() []llms.Tool {
	return []llms.Tool{
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "create_task",
				Description: "Create a new task summarized from commits or provided context.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"intent":          map[string]interface{}{"type": "string", "description": "What was done"},
						"scope":           map[string]interface{}{"type": "string", "description": "Component or area"},
						"type":            map[string]interface{}{"type": "string", "enum": []string{"delivery", "fix", "chore", "refactor", "meeting"}},
						"estimated_hours": map[string]interface{}{"type": "number"},
					},
					"required": []string{"intent", "score", "type", "estimated_hours"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "edit_task",
				Description: "Modify an existing task's properties.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index":           map[string]interface{}{"type": "integer", "description": "0-based index of the task to edit"},
						"intent":          map[string]interface{}{"type": "string"},
						"scope":           map[string]interface{}{"type": "string"},
						"estimated_hours": map[string]interface{}{"type": "number"},
					},
					"required": []string{"index"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "add_details",
				Description: "Add technical details or bullet points to a task.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index":         map[string]interface{}{"type": "integer"},
						"technical_why": map[string]interface{}{"type": "string", "description": "Technical details (markdown bullets preferred)"},
					},
					"required": []string{"index", "technical_why"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "add_time",
				Description: "Add additional hours to a task.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index": map[string]interface{}{"type": "integer"},
						"hours": map[string]interface{}{"type": "number"},
					},
					"required": []string{"index", "hours"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "add_commit_reference",
				Description: "Link a commit hash to a task.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"index": map[string]interface{}{"type": "integer"},
						"hash":  map[string]interface{}{"type": "string", "description": "Full commit hash"},
					},
					"required": []string{"index", "hash"},
				},
			},
		},
		{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "get_codebase_context",
				Description: "Search the codebase for context using ripgrep.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query":       map[string]interface{}{"type": "string"},
						"path":        map[string]interface{}{"type": "string", "description": "Optional subdirectory to search"},
						"max_results": map[string]interface{}{"type": "integer", "default": 20},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}

func (tc *ToolCall) UnmarshalJSON(data []byte) error {
	type Alias ToolCall
	var aux Alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// If we have a nested parameters field, use it
	if aux.Parameters != nil {
		tc.Tool = aux.Tool
		tc.Parameters = aux.Parameters
		return nil
	}

	// Otherwise, treat the entire object as parameters, excluding the "tool" key
	var flat map[string]interface{}
	if err := json.Unmarshal(data, &flat); err != nil {
		return err
	}

	tc.Tool = castString(flat["tool"])
	delete(flat, "tool")
	tc.Parameters = flat

	return nil
}

func readPromptFile(filename string) string {
	// Try local prompts directory first
	content, err := os.ReadFile(filepath.Join("prompts", filename))
	if err == nil {
		return string(content)
	}

	// Try ~/.md2slack/prompts/
	home, err := os.UserHomeDir()
	if err == nil {
		content, err = os.ReadFile(filepath.Join(home, ".md2slack", "prompts", filename))
		if err == nil {
			return string(content)
		}
	}

	return ""
}

func ExtractCommitIntent(change gitdiff.SemanticChange, commitMsg string, options LLMOptions) (*gitdiff.CommitChange, error) {
	system := readPromptFile("commit_intent_extractor.txt")
	if system == "" {
		return nil, errors.New("prompt file commit_intent_extractor.txt not found")
	}

	prompt := fmt.Sprintf("Commit: %s\nMessage: %s\nSignals: %v", change.CommitHash, commitMsg, change.Signals)

	var out gitdiff.CommitChange
	messages := []OpenAIMessage{{Role: "user", Content: prompt}}
	err := callJSON(messages, system, options, &out)
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
	messages := []OpenAIMessage{{Role: "user", Content: prompt}}
	err := callJSON(messages, system, options, &out)
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
		diff := diffMap[c.Hash]
		sem := semMap[c.Hash]
		summary, err := SummarizeCommit(c, diff, sem, options)
		if err != nil {
			return out, err
		}
		summary.CommitHash = c.Hash
		out = append(out, *summary)
	}
	return out, nil
}

func FallbackTasksFromSummaries(summaries []gitdiff.CommitSummary) []gitdiff.TaskChange {
	var tasks []gitdiff.TaskChange
	for _, s := range summaries {
		intent := strings.TrimSpace(s.Summary)
		if intent == "" {
			continue
		}
		task := gitdiff.TaskChange{
			TaskType:   "delivery",
			TaskIntent: intent,
			Scope:      s.Area,
			Commits:    []string{s.CommitHash},
		}
		if s.Impact != "" {
			task.TechnicalWhy = s.Impact
		}
		tasks = append(tasks, task)
	}
	return tasks
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
	messages := []OpenAIMessage{{Role: "user", Content: prompt}}
	err := callJSON(messages, system, options, &out)
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
	messages := []OpenAIMessage{{Role: "user", Content: prompt}}

	var currentTasks []gitdiff.TaskChange
	// Iterative Loop
	for turn := 0; turn < 5; turn++ {
		var tools []ToolCall
		err := callJSON(messages, system, options, &tools, getNativeTools()...)
		if err != nil {
			return currentTasks, err
		}
		if len(tools) == 0 {
			break
		}

		// Apply tools
		var log string
		var status string
		currentTasks, log, status = ApplyTools(tools, currentTasks, nil)
		emitToolUpdates(options, log, status)
		for i := range currentTasks {
			currentTasks[i].IsManual = true
		}

		showStateDashboard("User Context", currentTasks, log, turn, options.Quiet)
		PrintMarkdownTasks(currentTasks, options.Quiet)

		// Update history for next turn
		toolsJSON, _ := json.Marshal(tools)
		messages = append(messages, OpenAIMessage{Role: "assistant", Content: string(toolsJSON)})
		var feedbackSB strings.Builder
		feedbackSB.WriteString("Tool Execution Log:\n")
		feedbackSB.WriteString(log)
		if errs := toolErrorSummary(log); errs != "" {
			feedbackSB.WriteString("\n\nTool Errors:\n")
			feedbackSB.WriteString(errs)
		}
		feedbackSB.WriteString("\n\nContinue if more tasks need to be created from the extra context, otherwise return [].")
		messages = append(messages, OpenAIMessage{Role: "user", Content: feedbackSB.String()})
	}

	return currentTasks, nil
}

func GenerateTasksFromContext(commits []gitdiff.Commit, summaries []gitdiff.CommitSummary, semantics []gitdiff.CommitSemantic, extraContext string, options LLMOptions, allowedCommits map[string]struct{}) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_tools_generate.txt")
	if system == "" {
		return nil, errors.New("prompt file task_tools_generate.txt not found")
	}

	commitsJSON, _ := json.MarshalIndent(commits, "", "  ")
	summaryJSON, _ := json.MarshalIndent(summaries, "", "  ")
	semanticJSON, _ := json.MarshalIndent(semantics, "", "  ")
	allowedList := sortedCommitList(allowedCommits)
	allowedText := "(none)"
	if len(allowedList) > 0 {
		allowedText = strings.Join(allowedList, ", ")
	}

	prompt := fmt.Sprintf("Extra Context: %s\nValid Phase 1 Commits: %s\nCommits (JSON): %s\nCommit Summaries (JSON): %s\nSemantic (JSON): %s",
		extraContext, allowedText, string(commitsJSON), string(summaryJSON), string(semanticJSON))

	var currentTasks []gitdiff.TaskChange
	messages := []OpenAIMessage{{Role: "user", Content: prompt}}

	for turn := 0; turn < 8; turn++ {
		var tools []ToolCall
		err := callJSON(messages, system, options, &tools, getNativeTools()...)
		if err != nil {
			return currentTasks, err
		}
		if len(tools) == 0 {
			break
		}

		var log string
		var status string
		currentTasks, log, status = ApplyTools(tools, currentTasks, allowedCommits)
		emitToolUpdates(options, log, status)

		showStateDashboard("Task Gen", currentTasks, log, turn, options.Quiet)
		PrintMarkdownTasks(currentTasks, options.Quiet)

		toolsJSON, _ := json.Marshal(tools)
		messages = append(messages, OpenAIMessage{Role: "assistant", Content: string(toolsJSON)})
		var feedbackSB strings.Builder
		feedbackSB.WriteString("Tool Execution Log:\n")
		feedbackSB.WriteString(log)
		if errs := toolErrorSummary(log); errs != "" {
			feedbackSB.WriteString("\n\nTool Errors:\n")
			feedbackSB.WriteString(errs)
		}
		feedbackSB.WriteString("\n\nCurrent Tasks (State):\n")
		for i, t := range currentTasks {
			feedbackSB.WriteString(fmt.Sprintf("[%d] %s (%s) [%s]\n", i, t.TaskIntent, t.Scope, t.TaskType))
			if t.TechnicalWhy != "" {
				feedbackSB.WriteString(fmt.Sprintf("    Details: %s\n", t.TechnicalWhy))
			}
			feedbackSB.WriteString(fmt.Sprintf("    Commits: %v\n", t.Commits))
		}
		feedbackSB.WriteString("\nContinue if more tasks need to be created, otherwise return [].")
		messages = append(messages, OpenAIMessage{Role: "user", Content: feedbackSB.String()})
	}

	return currentTasks, nil
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
	allowedList := sortedCommitList(allowedCommits)
	allowedText := "(none)"
	if len(allowedList) > 0 {
		allowedText = strings.Join(allowedList, ", ")
	}

	prompt := fmt.Sprintf("Extra Context: %s\nValid Phase 1 Commits: %s\nCommits (JSON): %s\nCommit Summaries (JSON): %s\nSemantic (JSON): %s\nCurrent Tasks (JSON): %s",
		extraContext, allowedText, string(commitsJSON), string(summaryJSON), string(semanticJSON), string(tasksJSON))

	messages := []OpenAIMessage{{Role: "user", Content: prompt}}

	for turn := 0; turn < 8; turn++ {
		var tools []ToolCall
		err := callJSON(messages, system, options, &tools, getNativeTools()...)
		if err != nil {
			return currentTasks, err
		}
		if len(tools) == 0 {
			break
		}

		var log string
		var status string
		currentTasks, log, status = ApplyTools(tools, currentTasks, allowedCommits)
		emitToolUpdates(options, log, status)

		showStateDashboard("Task Review", currentTasks, log, turn, options.Quiet)
		PrintMarkdownTasks(currentTasks, options.Quiet)

		toolsJSON, _ := json.Marshal(tools)
		messages = append(messages, OpenAIMessage{Role: "assistant", Content: string(toolsJSON)})
		var feedbackSB strings.Builder
		feedbackSB.WriteString("Tool Execution Log:\n")
		feedbackSB.WriteString(log)
		if errs := toolErrorSummary(log); errs != "" {
			feedbackSB.WriteString("\n\nTool Errors:\n")
			feedbackSB.WriteString(errs)
		}
		feedbackSB.WriteString("\n\nCurrent Tasks (State):\n")
		for i, t := range currentTasks {
			feedbackSB.WriteString(fmt.Sprintf("[%d] %s (%s) [%s]\n", i, t.TaskIntent, t.Scope, t.TaskType))
			if t.TechnicalWhy != "" {
				feedbackSB.WriteString(fmt.Sprintf("    Details: %s\n", t.TechnicalWhy))
			}
			feedbackSB.WriteString(fmt.Sprintf("    Commits: %v\n", t.Commits))
		}
		feedbackSB.WriteString("\nContinue reviewing until duplicates and discrepancies are resolved; return [] only when done.")
		messages = append(messages, OpenAIMessage{Role: "user", Content: feedbackSB.String()})
	}

	return currentTasks, nil
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
		if t.TechnicalWhy != "" {
			parts := strings.Split(t.TechnicalWhy, "\n")
			for _, p := range parts {
				sb.WriteString(fmt.Sprintf("    - %s\n", p))
			}
		}
	}
	tasksState := sb.String()
	if tasksState == "" {
		tasksState = "(no commit tasks yet)"
	}

	var allowedList []string
	for hash := range allowedCommits {
		allowedList = append(allowedList, hash)
	}
	sort.Strings(allowedList)
	allowedText := "(none)"
	if len(allowedList) > 0 {
		allowedText = strings.Join(allowedList, ", ")
	}

	commitJSON, _ := json.MarshalIndent(commit, "", "  ")
	prompt := fmt.Sprintf("Extra Context: %s\nManual Tasks (Read-Only Context):\n%s\nCurrent Commit-Based Tasks (State):\n%s\nValid Phase 1 Commits: %s\nNew Commit to Incorporate: %s", extraContext, manualContext, tasksState, allowedText, string(commitJSON))

	messages := []OpenAIMessage{{Role: "user", Content: prompt}}

	// Iterative Loop: Allow the LLM to call tools and see results
	for turn := 0; turn < 8; turn++ {
		var tools []ToolCall
		err := callJSON(messages, system, options, &tools, getNativeTools()...)
		if err != nil {
			return currentTasks, err
		}

		// Apply tools and update state
		var log string
		var status string
		currentTasks, log, status = ApplyTools(tools, currentTasks, allowedCommits)
		emitToolUpdates(options, log, status)

		// REAL-TIME VISUALIZATION: Show the state dashboard to the user
		showStateDashboard(commit.CommitHash, currentTasks, log, turn, options.Quiet)
		PrintMarkdownTasks(currentTasks, options.Quiet)

		// VALIDATION: Check if we can stop
		isLinked := false
		for _, t := range currentTasks {
			for _, c := range t.Commits {
				if c == commit.CommitHash {
					isLinked = true
					break
				}
			}
		}

		missingDetails := false
		for i, t := range currentTasks {
			if t.TechnicalWhy == "" || strings.Contains(strings.ToLower(t.TechnicalWhy), "no details") {
				missingDetails = true
				log += fmt.Sprintf("\nError: Task %d is missing technical details.", i)
			}
		}

		// If LLM returned empty tools but validation failed, force a retry Turn
		if len(tools) == 0 {
			if !isLinked {
				log += fmt.Sprintf("\nCRITICAL: Commit %s is NOT linked to any task. You MUST call add_commit_reference.", commit.CommitHash)
			} else if !missingDetails {
				// All good, we can stop
				break
			}
		}

		// Prepare feedback for the LLM
		var feedbackSB strings.Builder
		feedbackSB.WriteString("Tool Execution Log:\n")
		feedbackSB.WriteString(log)
		if errs := toolErrorSummary(log); errs != "" {
			feedbackSB.WriteString("\n\nTool Errors:\n")
			feedbackSB.WriteString(errs)
		}
		feedbackSB.WriteString("\n\nExtra Context (Instructions):\n")
		feedbackSB.WriteString(extraContext)
		feedbackSB.WriteString("\n\nCurrent Tasks (State):\n")
		for i, t := range currentTasks {
			feedbackSB.WriteString(fmt.Sprintf("[%d] %s (%s) [%s]\n", i, t.TaskIntent, t.Scope, t.TaskType))
			if t.TechnicalWhy != "" {
				feedbackSB.WriteString(fmt.Sprintf("    Details: %s\n", t.TechnicalWhy))
			}
			feedbackSB.WriteString(fmt.Sprintf("    Commits: %v\n", t.Commits))
		}
		feedbackSB.WriteString("\nValid Phase 1 Commits:\n")
		if len(allowedList) == 0 {
			feedbackSB.WriteString("(none)\n")
		} else {
			feedbackSB.WriteString(strings.Join(allowedList, ", "))
			feedbackSB.WriteString("\n")
		}

		if !isLinked {
			feedbackSB.WriteString(fmt.Sprintf("\nWARNING: Current commit %s is NOT yet linked to any task. Use add_commit_reference to fix this.", commit.CommitHash))
		}

		feedbackSB.WriteString("\nContinue until all rules are met. Return [] ONLY when done and commit is fully integrated.")

		// Update history
		var toolsStr string
		if len(tools) > 0 {
			toolsJSON, _ := json.Marshal(tools)
			toolsStr = string(toolsJSON)
		} else {
			toolsStr = "[]"
		}
		messages = append(messages, OpenAIMessage{Role: "assistant", Content: toolsStr})
		messages = append(messages, OpenAIMessage{Role: "user", Content: feedbackSB.String()})
	}

	return currentTasks, nil
}

func showStateDashboard(commitHash string, tasks []gitdiff.TaskChange, lastLog string, turn int, quiet bool) {
	if quiet {
		return
	}
	fmt.Printf("\r  [Turn %d] Incorporating %s | Current Tasks: %d                     \n", turn+1, commitHash, len(tasks))
	// Print simple log if it contains errors
	if strings.Contains(strings.ToLower(lastLog), "error") || strings.Contains(strings.ToLower(lastLog), "critical") {
		fmt.Printf("    > %s\n", lastLog)
	}
}

func PrintMarkdownTasks(tasks []gitdiff.TaskChange, quiet bool) {
	if quiet {
		return
	}
	fmt.Println("\n--- DEBUG: Current Task List ---")
	for i, t := range tasks {
		fmt.Printf("[%d] **%s** (%s) [%s]\n", i, t.TaskIntent, t.Scope, t.TaskType)
		if t.TechnicalWhy != "" {
			lines := strings.Split(t.TechnicalWhy, "\n")
			for _, l := range lines {
				if strings.TrimSpace(l) != "" {
					fmt.Printf("    - %s\n", l)
				}
			}
		}
		if len(t.Commits) > 0 {
			fmt.Printf("    commits: `%s`\n", strings.Join(t.Commits, "`, `"))
		}
	}
	fmt.Println("--------------------------------")
}

func PruneTasks(tasks []gitdiff.TaskChange) []gitdiff.TaskChange {
	var out []gitdiff.TaskChange
	for _, t := range tasks {
		if t.IsHistorical {
			out = append(out, t)
			continue
		}
		// Preserve if it has a commit OR is a useful shell
		hasCommits := len(t.Commits) > 0
		if (hasCommits || t.TaskIntent != "") && t.TaskIntent != "" {
			out = append(out, t)
		}
	}
	return out
}

func RefineTasks(tasks []gitdiff.TaskChange, options LLMOptions) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_refiner.txt")
	if system == "" {
		return tasks, nil // Fallback
	}

	tasksJSON, _ := json.MarshalIndent(tasks, "", "  ")
	prompt := fmt.Sprintf("Current Task List:\n%s", string(tasksJSON))

	var out []gitdiff.TaskChange
	messages := []OpenAIMessage{{Role: "user", Content: prompt}}
	err := callJSON(messages, system, options, &out)
	if err != nil {
		fmt.Printf("Warning: task refinement failed: %v. Using unrefined list.\n", err)
		return tasks, nil
	}

	// Double prune after refinement to remove any junk the LLM might have introduced
	out = PruneTasks(out)

	if len(out) == 0 && len(tasks) > 0 {
		return tasks, nil // Don't return an empty list if refinement wiped it out
	}
	return out, nil
}

func RefineTasksWithPrompt(tasks []gitdiff.TaskChange, userPrompt string, options LLMOptions) ([]gitdiff.TaskChange, error) {
	if strings.TrimSpace(userPrompt) == "" {
		return RefineTasks(tasks, options)
	}

	system := readPromptFile("task_refiner.txt")
	if system == "" {
		return tasks, nil // Fallback
	}

	tasksJSON, _ := json.MarshalIndent(tasks, "", "  ")
	prompt := fmt.Sprintf("User request:\n%s\n\nCurrent Task List:\n%s", strings.TrimSpace(userPrompt), string(tasksJSON))

	var out []gitdiff.TaskChange
	messages := []OpenAIMessage{{Role: "user", Content: prompt}}
	err := callJSON(messages, system, options, &out)
	if err != nil {
		fmt.Printf("Warning: task refinement (with user prompt) failed: %v. Using unrefined list.\n", err)
		return tasks, nil
	}

	out = PruneTasks(out)
	if len(out) == 0 && len(tasks) > 0 {
		return tasks, nil
	}
	return out, nil
}

func EditTasksWithAction(tasks []gitdiff.TaskChange, action string, selected []int, options LLMOptions) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_editor.txt")
	if system == "" {
		return tasks, errors.New("prompt file task_editor.txt not found")
	}

	tasksJSON, _ := json.MarshalIndent(tasks, "", "  ")
	selectedJSON, _ := json.MarshalIndent(selected, "", "  ")
	prompt := fmt.Sprintf("Action: %s\nSelected Indices: %s\nTasks: %s", action, string(selectedJSON), string(tasksJSON))

	var out []gitdiff.TaskChange
	messages := []OpenAIMessage{{Role: "user", Content: prompt}}
	err := callJSON(messages, system, options, &out)
	if err != nil {
		return tasks, err
	}

	normalized := normalizeSelected(selected, len(tasks))
	if len(normalized) == 0 {
		return tasks, nil
	}

	// If the model returned a full list, accept it as-is.
	if len(out) == len(tasks) {
		return out, nil
	}

	// If the model returned only the selected tasks, merge them back.
	switch action {
	case "make_longer", "make_shorter", "improve_text":
		if len(out) == len(normalized) {
			merged := append([]gitdiff.TaskChange(nil), tasks...)
			for i, idx := range normalized {
				merged[idx] = out[i]
			}
			return merged, nil
		}
		if len(out) == 1 && len(normalized) == 1 {
			merged := append([]gitdiff.TaskChange(nil), tasks...)
			merged[normalized[0]] = out[0]
			return merged, nil
		}
	case "split_task":
		if len(normalized) == 1 && len(out) >= 2 {
			idx := normalized[0]
			merged := append([]gitdiff.TaskChange(nil), tasks[:idx]...)
			merged = append(merged, out...)
			merged = append(merged, tasks[idx+1:]...)
			return merged, nil
		}
	case "merge_tasks":
		if len(normalized) >= 2 && len(out) == 1 {
			first := normalized[0]
			keep := make([]gitdiff.TaskChange, 0, len(tasks)-len(normalized)+1)
			for i, t := range tasks {
				if i == first {
					keep = append(keep, out[0])
				}
				if !indexInSorted(i, normalized) {
					keep = append(keep, t)
				}
			}
			return keep, nil
		}
	}

	// Fallback to original tasks if output shape is unexpected.
	return tasks, nil
}

func SuggestNextActions(tasks []gitdiff.TaskChange, options LLMOptions) ([]string, error) {
	system := readPromptFile("next_actions.txt")
	if system == "" {
		return nil, errors.New("prompt file next_actions.txt not found")
	}

	// Create a clean JSON representing only the essential task info
	type cleanTask struct {
		Intent string `json:"intent"`
		Scope  string `json:"scope"`
		Type   string `json:"type"`
	}
	var cleanTasks []cleanTask
	for _, t := range tasks {
		cleanTasks = append(cleanTasks, cleanTask{
			Intent: t.TaskIntent,
			Scope:  t.Scope,
			Type:   t.TaskType,
		})
	}

	inputJSON, _ := json.MarshalIndent(cleanTasks, "", "  ")
	messages := []OpenAIMessage{
		{Role: "user", Content: fmt.Sprintf("Tasks synthesized for today:\n%s", string(inputJSON))},
	}

	var suggestions []string
	err := callJSON(messages, system, options, &suggestions)
	if err != nil {
		return nil, err
	}

	if len(suggestions) == 0 {
		fmt.Printf("Debug: Stage 3 (Next Actions) returned empty array. Tasks count: %d\n", len(tasks))
	}

	return suggestions, nil
}

func normalizeSelected(selected []int, max int) []int {
	if len(selected) == 0 || max <= 0 {
		return nil
	}
	uniq := make(map[int]struct{}, len(selected))
	for _, idx := range selected {
		if idx < 0 || idx >= max {
			continue
		}
		uniq[idx] = struct{}{}
	}
	out := make([]int, 0, len(uniq))
	for idx := range uniq {
		out = append(out, idx)
	}
	sort.Ints(out)
	return out
}

func indexInSorted(val int, sorted []int) bool {
	i := sort.SearchInts(sorted, val)
	return i < len(sorted) && sorted[i] == val
}

func ApplyTools(tools []ToolCall, tasks []gitdiff.TaskChange, allowedCommits map[string]struct{}) ([]gitdiff.TaskChange, string, string) {
	var logs []string
	var status string
	for _, tc := range tools {
		params := tc.Parameters
		switch tc.Tool {
		case "create_task":
			intent := castString(params["intent"])
			if intent == "" {
				logs = append(logs, "Error: attempt to create task with empty intent")
				continue
			}
			newTask := gitdiff.TaskChange{
				TaskIntent: intent,
				Scope:      castString(params["scope"]),
				TaskType:   castString(params["type"]),
			}
			if newTask.TaskType == "" {
				newTask.TaskType = "delivery"
			}
			if h, ok := castInt(params["estimated_hours"]); ok {
				newTask.EstimatedHours = &h
			}
			tasks = append(tasks, newTask)
			logs = append(logs, fmt.Sprintf("Success: created task with index %d", len(tasks)-1))
			status = fmt.Sprintf("Created task #%d: %s", len(tasks)-1, intent)

		case "edit_task":
			idx, ok := castInt(params["index"])
			if !ok || idx < 0 || idx >= len(tasks) {
				logs = append(logs, fmt.Sprintf("Error: index %v is out of bounds (current max: %d)", params["index"], len(tasks)-1))
				continue
			}
			if intent := castString(params["intent"]); intent != "" {
				tasks[idx].TaskIntent = intent
			}
			if scope := castString(params["scope"]); scope != "" {
				tasks[idx].Scope = scope
			}
			if h, ok := castInt(params["estimated_hours"]); ok {
				tasks[idx].EstimatedHours = &h
			}
			logs = append(logs, fmt.Sprintf("Success: edited task %d", idx))
			status = fmt.Sprintf("Edited task #%d", idx)

		case "add_details":
			idx, ok := castInt(params["index"])
			if !ok || idx < 0 || idx >= len(tasks) {
				logs = append(logs, fmt.Sprintf("Error: index %v is out of bounds (current max: %d)", params["index"], len(tasks)-1))
				continue
			}
			detail := castString(params["technical_why"])
			if detail != "" && !strings.Contains(detail, "...") {
				if tasks[idx].TechnicalWhy == "" {
					tasks[idx].TechnicalWhy = detail
				} else {
					if !strings.Contains(tasks[idx].TechnicalWhy, detail) {
						tasks[idx].TechnicalWhy += "\n" + detail
					}
				}
				logs = append(logs, fmt.Sprintf("Success: added detail to task %d", idx))
				status = fmt.Sprintf("Updated details for task #%d", idx)
			}

		case "add_time":
			idx, ok := castInt(params["index"])
			if !ok || idx < 0 || idx >= len(tasks) {
				logs = append(logs, fmt.Sprintf("Error: index %v is out of bounds (current max: %d)", params["index"], len(tasks)-1))
				continue
			}
			if h, ok := castInt(params["hours"]); ok {
				if tasks[idx].EstimatedHours == nil {
					tasks[idx].EstimatedHours = &h
				} else {
					newH := *tasks[idx].EstimatedHours + h
					tasks[idx].EstimatedHours = &newH
				}
				logs = append(logs, fmt.Sprintf("Success: added %d hours to task %d", h, idx))
				status = fmt.Sprintf("Updated time for task #%d", idx)
			}

		case "add_commit_reference":
			idx, ok := castInt(params["index"])
			if !ok || idx < 0 || idx >= len(tasks) {
				logs = append(logs, fmt.Sprintf("Error: index %v is out of bounds (current max: %d)", params["index"], len(tasks)-1))
				continue
			}
			hash := castString(params["hash"])
			if hash != "" && (allowedCommits == nil || len(allowedCommits) == 0) {
				logs = append(logs, fmt.Sprintf("Error: commit %s is not a valid Phase 1 commit (no allowed list available)", hash))
				continue
			}
			if hash != "" {
				if _, ok := allowedCommits[hash]; !ok {
					logs = append(logs, fmt.Sprintf("Error: commit %s is not a valid Phase 1 commit", hash))
					continue
				}
			}
			if hash != "" {
				found := false
				for _, c := range tasks[idx].Commits {
					if c == hash {
						found = true
						break
					}
				}
				if !found {
					tasks[idx].Commits = append(tasks[idx].Commits, hash)
				}
				logs = append(logs, fmt.Sprintf("Success: added commit %s to task %d", hash, idx))
				status = fmt.Sprintf("Linked commit %s to task #%d", hash, idx)
			}

		case "get_codebase_context":
			query := castString(params["query"])
			path := castString(params["path"])
			maxResults, ok := castInt(params["max_results"])
			if !ok || maxResults <= 0 {
				maxResults = 20
			}
			if query == "" {
				logs = append(logs, "Error: get_codebase_context requires non-empty query")
				continue
			}
			out, err := getCodebaseContext(query, path, maxResults)
			if err != nil {
				logs = append(logs, fmt.Sprintf("Error: get_codebase_context failed: %v", err))
				continue
			}
			if out == "" {
				logs = append(logs, fmt.Sprintf("Context: no matches for query %q", query))
				continue
			}
			logs = append(logs, fmt.Sprintf("Context (query %q):\n%s", query, out))
		}
	}
	return tasks, strings.Join(logs, "\n"), status
}

func getCodebaseContext(query string, path string, maxResults int) (string, error) {
	if path == "" {
		path = "."
	}
	if !strings.HasPrefix(path, ".") && !strings.HasPrefix(path, "/") {
		path = "./" + path
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("invalid path %q: %v", path, err)
	}
	args := []string{"--no-heading", "--line-number", "--max-count", strconv.Itoa(maxResults), query, path}
	cmd := exec.Command("rg", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		// Treat "no matches" as empty output without error.
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return "", nil
			}
			return "", fmt.Errorf("rg error (exit %d): %s", exitErr.ExitCode(), strings.TrimSpace(out.String()))
		}
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func toolErrorSummary(log string) string {
	if log == "" {
		return ""
	}
	var errs []string
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, "Error:") || strings.Contains(line, "CRITICAL:") {
			errs = append(errs, line)
		}
	}
	if len(errs) == 0 {
		return ""
	}
	return strings.Join(errs, "\n")
}

func emitToolUpdates(options LLMOptions, log string, status string) {
	if options.OnToolLog != nil && log != "" {
		for _, line := range strings.Split(log, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			options.OnToolLog(line)
		}
	}
	if options.OnToolStatus != nil && strings.TrimSpace(status) != "" {
		options.OnToolStatus(status)
	}
}

func emitLLMLog(options LLMOptions, label string, content string) {
	if options.OnLLMLog == nil {
		return
	}
	msg := fmt.Sprintf("%s:\n%s", label, truncateLog(content, 4000))
	for _, line := range strings.Split(msg, "\n") {
		options.OnLLMLog(line)
	}
}

func formatMessages(messages []OpenAIMessage) string {
	var sb strings.Builder
	for i, m := range messages {
		sb.WriteString(fmt.Sprintf("[%d] %s:\n", i, strings.ToUpper(m.Role)))
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}

func truncateLog(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n…(truncated)"
}

func sortedCommitList(allowedCommits map[string]struct{}) []string {
	if len(allowedCommits) == 0 {
		return nil
	}
	var out []string
	for hash := range allowedCommits {
		out = append(out, hash)
	}
	sort.Strings(out)
	return out
}

// Helpers for casting (aliased from gitdiff to avoid circular dependency if needed,
// but here we can just use those that are already in gitdiff if they are exported.
// Wait, they are not exported. I'll re-implement them or move them.
// For now, I'll implement simple versions here to avoid breaking things.

func castString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []interface{}:
		var parts []string
		for _, part := range val {
			parts = append(parts, fmt.Sprint(part))
		}
		return strings.Join(parts, "\n")
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func castInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case float64:
		return int(val), true
	case int:
		return val, true
	case string:
		if i, err := strconv.Atoi(val); err == nil {
			return i, true
		}
	case nil:
		return 0, false
	}
	return 0, false
}

func GroupTasks(tasks []gitdiff.TaskChange, options LLMOptions) ([]gitdiff.GroupedTask, error) {
	system := readPromptFile("task_grouper.txt")
	if system == "" {
		return nil, errors.New("prompt file task_grouper.txt not found")
	}

	tasksJSON, _ := json.MarshalIndent(tasks, "", "  ")
	prompt := fmt.Sprintf("Tasks: %s", string(tasksJSON))

	var out []gitdiff.GroupedTask
	messages := []OpenAIMessage{{Role: "user", Content: prompt}}
	err := callJSON(messages, system, options, &out)
	return out, err
}

func getAdapter(options LLMOptions) (LLMAdapter, error) {
	provider := strings.ToLower(options.Provider)
	switch provider {
	case "ollama":
		return NewOllamaAdapter(options.ModelName, options.BaseUrl)
	case "openai", "codex":
		return NewOpenAIAdapter(options.ModelName, options.Token, options.BaseUrl)
	case "anthropic":
		return NewAnthropicAdapter(options.ModelName, options.Token, options.BaseUrl)
	default:
		// Default to Ollama if unknown
		return NewOllamaAdapter(options.ModelName, options.BaseUrl)
	}
}

func convertToLLMCMessages(messages []OpenAIMessage, system string) []llms.MessageContent {
	var result []llms.MessageContent
	if system != "" {
		result = append(result, llms.TextParts(llms.ChatMessageTypeSystem, system))
	}
	for _, msg := range messages {
		role := strings.ToLower(msg.Role)
		switch role {
		case "system":
			result = append(result, llms.TextParts(llms.ChatMessageTypeSystem, msg.Content))
		case "user":
			result = append(result, llms.TextParts(llms.ChatMessageTypeHuman, msg.Content))
		case "assistant", "ai":
			result = append(result, llms.TextParts(llms.ChatMessageTypeAI, msg.Content))
		default:
			result = append(result, llms.TextParts(llms.ChatMessageTypeHuman, msg.Content))
		}
	}
	return result
}

func callJSON(messages []OpenAIMessage, system string, options LLMOptions, target interface{}, tools ...llms.Tool) error {
	adapter, err := getAdapter(options)
	if err != nil {
		return err
	}

	llmsMessages := convertToLLMCMessages(messages, system)
	payload := formatMessages(messages) // system is already in fullMessages in original code but here we keep it separate for convert
	if system != "" {
		payload = "SYSTEM: " + system + "\n" + payload
	}
	emitLLMLog(options, "LLM INPUT", payload)

	reqStart := time.Now()
	emitLLMLog(options, "LLM STATUS", fmt.Sprintf("request queued (provider=%s model=%s)", options.Provider, options.ModelName))

	timeout := options.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	callOpts := []llms.CallOption{
		llms.WithRepetitionPenalty(options.RepeatPenalty),
	}

	provider := strings.ToLower(options.Provider)
	if provider == "anthropic" {
		// Anthropic does not allow both. Prioritize temperature if set.
		if options.Temperature > 0 {
			callOpts = append(callOpts, llms.WithTemperature(options.Temperature))
		} else if options.TopP > 0 && options.TopP < 1.0 {
			callOpts = append(callOpts, llms.WithTopP(options.TopP))
		}
	} else {
		callOpts = append(callOpts, llms.WithTemperature(options.Temperature))
		callOpts = append(callOpts, llms.WithTopP(options.TopP))
	}

	if len(tools) > 0 {
		callOpts = append(callOpts, llms.WithTools(tools))
	}

	if strings.ToLower(options.Provider) == "ollama" {
		// Ollama in langchaingo supports JSON mode via options if the model supports it
		// but specifically here the original code used "format": "json"
		// langchaingo's ollama implementation might handle this differently.
	}

	resp, err := adapter.GenerateContent(ctx, llmsMessages, callOpts...)
	if err != nil {
		emitLLMLog(options, "LLM STATUS", fmt.Sprintf("request error after %s: %v", time.Since(reqStart).Truncate(time.Millisecond), err))
		return err
	}

	if len(resp.Choices) == 0 {
		return errors.New("empty LLM response")
	}

	responseText := resp.Choices[0].Content
	toolCalls := resp.Choices[0].ToolCalls
	emitLLMLog(options, "LLM OUTPUT", responseText)
	if len(toolCalls) > 0 {
		emitLLMLog(options, "LLM TOOL CALLS", fmt.Sprintf("%d calls", len(toolCalls)))
	}

	// Handle native tool calls if they exist and target can accept them
	if len(toolCalls) > 0 {
		// Try to find if target has a field named Tools of type []ToolCall
		// This is a bit hacky but it avoids changing all call sites immediately
		// if we only use it for ChatOutput.
		// For now, let's just check if it's a pointer to ChatOutput (defined in chat.go)
		// Wait, ChatOutput is private to chat.go.
		// Let's use reflection or just assume the target can be unmarshaled into.

		var nativeTools []ToolCall
		for _, tc := range toolCalls {
			var params map[string]interface{}
			_ = json.Unmarshal([]byte(tc.FunctionCall.Arguments), &params)
			nativeTools = append(nativeTools, ToolCall{
				Tool:       tc.FunctionCall.Name,
				Parameters: params,
			})
		}

		// If the LLM returned a text response AND tool calls, we want both.
		// We'll try to marshal them into a JSON object that matches ChatOutput.
		type tempOutput struct {
			Text  string     `json:"text"`
			Tools []ToolCall `json:"tools"`
		}
		tmp := tempOutput{
			Text:  responseText,
			Tools: nativeTools,
		}
		raw, _ := json.Marshal(tmp)
		return json.Unmarshal(raw, target)
	}

	clean := strings.TrimSpace(responseText)
	clean = stripCodeFences(clean)
	clean = extractJSONPayload(clean)
	raw := []byte(strings.TrimSpace(clean))

	// Robust unmarshal: if target is a slice but response is an empty object {},
	// treat it as an empty array.
	if clean == "{}" || clean == "{ }" {
		// Check if target is a pointer to a slice
		if strings.HasPrefix(fmt.Sprintf("%T", target), "*[]") {
			return nil
		}
	}

	err = json.Unmarshal(raw, target)
	if err != nil {
		// Try to see if it's an object containing an array (wrapper key case)
		if strings.HasPrefix(fmt.Sprintf("%T", target), "*[]") {
			var m map[string]interface{}
			if err2 := json.Unmarshal(raw, &m); err2 == nil {
				for _, v := range m {
					if b, err3 := json.Marshal(v); err3 == nil {
						if err4 := json.Unmarshal(b, target); err4 == nil {
							// If we got some items, return success
							return nil
						}
					}
				}
			}
		}

		// Try to see if it's a single object that should have been an array
		if strings.HasPrefix(string(raw), "{") {
			wrapped := append([]byte("["), raw...)
			wrapped = append(wrapped, ']')
			if err2 := json.Unmarshal(wrapped, target); err2 == nil {
				return nil
			}
		}
		return fmt.Errorf("unmarshal error: %v (response: %s)", err, responseText)
	}

	return nil
}

func stripCodeFences(input string) string {
	if strings.HasPrefix(input, "```") {
		parts := strings.SplitN(input, "\n", 2)
		if len(parts) == 2 {
			input = parts[1]
		}
		if idx := strings.LastIndex(input, "```"); idx >= 0 {
			input = input[:idx]
		}
	}
	return strings.TrimSpace(input)
}

func extractJSONPayload(input string) string {
	if input == "" {
		return input
	}

	firstObj := strings.Index(input, "{")
	firstArr := strings.Index(input, "[")

	start := -1
	end := -1

	if firstArr >= 0 && (firstObj == -1 || firstArr < firstObj) {
		start = firstArr
		end = findMatchingEnd(input, start, '[', ']')
	} else if firstObj >= 0 {
		start = firstObj
		end = findMatchingEnd(input, start, '{', '}')
	}

	if start >= 0 && end > start {
		return input[start : end+1]
	}

	return input
}

func findMatchingEnd(input string, start int, open, close rune) int {
	depth := 0
	for i, r := range input[start:] {
		switch r {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return start + i
			}
		}
	}
	return -1
}
