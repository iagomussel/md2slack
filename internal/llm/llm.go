package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"md2slack/internal/gitdiff"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type LLMOptions struct {
	Provider      string
	Temperature   float64
	TopP          float64
	RepeatPenalty float64
	ContextSize   int
	ModelName     string
	BaseUrl       string
	Debug         bool
}

type OpenAIRequest struct {
	Model    string          `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	Choices []struct {
		Message OpenAIMessage `json:"message"`
	} `json:"choices"`
}

type OllamaChatRequest struct {
	Model    string                 `json:"model"`
	Messages []OpenAIMessage        `json:"messages"`
	Format   string                 `json:"format,omitempty"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options"`
}

type OllamaChatResponse struct {
	Message OpenAIMessage `json:"message"`
	Done    bool          `json:"done"`
}

type ToolCall struct {
	Tool       string                 `json:"tool"`
	Parameters map[string]interface{} `json:"parameters"`
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

	system := readPromptFile("task_tools.txt")
	if system == "" {
		return nil, errors.New("prompt file task_tools.txt not found")
	}

	prompt := fmt.Sprintf("USER EXTRA CONTEXT:\n%s\n\nPlease create initial tasks based ON THIS CONTEXT. Use the tools provided.", extraContext)
	messages := []OpenAIMessage{{Role: "user", Content: prompt}}

	var currentTasks []gitdiff.TaskChange
	// Iterative Loop
	for turn := 0; turn < 5; turn++ {
		var tools []ToolCall
		err := callJSON(messages, system, options, &tools)
		if err != nil {
			return currentTasks, err
		}
		if len(tools) == 0 {
			break
		}

		// Apply tools
		var log string
		currentTasks, log = ApplyTools(tools, currentTasks, nil)
		for i := range currentTasks {
			currentTasks[i].IsManual = true
		}

		showStateDashboard("User Context", currentTasks, log, turn)
		PrintMarkdownTasks(currentTasks)

		// Update history for next turn
		toolsJSON, _ := json.Marshal(tools)
		messages = append(messages, OpenAIMessage{Role: "assistant", Content: string(toolsJSON)})
		messages = append(messages, OpenAIMessage{Role: "user", Content: "Tool Execution Log:\n" + log + "\n\nContinue if more tasks need to be created from the extra context, otherwise return []."})
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
		err := callJSON(messages, system, options, &tools)
		if err != nil {
			return currentTasks, err
		}

		// Apply tools and update state
		var log string
		currentTasks, log = ApplyTools(tools, currentTasks, allowedCommits)

		// REAL-TIME VISUALIZATION: Show the state dashboard to the user
		showStateDashboard(commit.CommitHash, currentTasks, log, turn)
		PrintMarkdownTasks(currentTasks)

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

func showStateDashboard(commitHash string, tasks []gitdiff.TaskChange, lastLog string, turn int) {
	fmt.Printf("\r  [Turn %d] Incorporating %s | Current Tasks: %d                     \n", turn+1, commitHash, len(tasks))
	// Print simple log if it contains errors
	if strings.Contains(strings.ToLower(lastLog), "error") || strings.Contains(strings.ToLower(lastLog), "critical") {
		fmt.Printf("    > %s\n", lastLog)
	}
}

func PrintMarkdownTasks(tasks []gitdiff.TaskChange) {
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
	fmt.Println("--------------------------------\n")
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

func ApplyTools(tools []ToolCall, tasks []gitdiff.TaskChange, allowedCommits map[string]struct{}) ([]gitdiff.TaskChange, string) {
	var logs []string
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
			tasks = append(tasks, newTask)
			logs = append(logs, fmt.Sprintf("Success: created task with index %d", len(tasks)-1))

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
			logs = append(logs, fmt.Sprintf("Success: edited task %d", idx))

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
			}
		}
	}
	return tasks, strings.Join(logs, "\n")
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

func callJSON(messages []OpenAIMessage, system string, options LLMOptions, target interface{}) error {
	var body []byte
	var url string

	// Ensure system prompt is at the beginning
	fullMessages := []OpenAIMessage{{Role: "system", Content: system}}
	fullMessages = append(fullMessages, messages...)

	switch strings.ToLower(options.Provider) {
	case "codex", "openai":
		url = options.BaseUrl
		if url == "" {
			url = "https://api.openai.com/v1/chat/completions"
		}
		if !strings.HasSuffix(url, "/chat/completions") && !strings.HasSuffix(url, "/completions") {
			url = strings.TrimSuffix(url, "/") + "/chat/completions"
		}

		req := OpenAIRequest{
			Model:    options.ModelName,
			Messages: fullMessages,
		}
		body, _ = json.Marshal(req)

	default: // ollama
		url = options.BaseUrl
		if url == "" {
			url = "http://localhost:11434"
		}
		url = strings.TrimSuffix(url, "/")
		url = strings.TrimSuffix(url, "/api/generate")
		url = strings.TrimSuffix(url, "/api/chat")
		url = url + "/api/chat"

		req := OllamaChatRequest{
			Model:    options.ModelName,
			Messages: fullMessages,
			Format:   "json",
			Stream:   false,
			Options: map[string]interface{}{
				"temperature":    0.1,
				"top_p":          options.TopP,
				"repeat_penalty": options.RepeatPenalty,
				"num_ctx":        options.ContextSize,
			},
		}
		body, _ = json.Marshal(req)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s status %d", options.Provider, resp.StatusCode)
	}

	var responseText string
	if strings.ToLower(options.Provider) == "codex" || strings.ToLower(options.Provider) == "openai" {
		var oai OpenAIResponse
		if err := json.NewDecoder(resp.Body).Decode(&oai); err != nil {
			return err
		}
		if len(oai.Choices) > 0 {
			responseText = oai.Choices[0].Message.Content
		}
	} else {
		var ollama OllamaChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&ollama); err != nil {
			return err
		}
		responseText = ollama.Message.Content
	}

	if options.Debug {
		fmt.Printf("\n--- RAW LLM RESPONSE (%s) ---\n%s\n--- END RAW RESPONSE ---\n\n", url, responseText)
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
