package llm

import (
	"fmt"
	"md2slack/internal/gitdiff"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

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

func getCodebaseContext(query string, path string, maxResults int) (string, error) {
	if path == "" {
		return "", nil
	}
	// Simple grep/search for context
	cmd := exec.Command("grep", "-r", "-l", query, path)
	out, err := cmd.Output()
	if err != nil {
		return "", nil // No matches
	}
	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(files) > maxResults {
		files = files[:maxResults]
	}

	var sb strings.Builder
	for _, f := range files {
		content, _ := os.ReadFile(f)
		sb.WriteString(fmt.Sprintf("\nFile: %s\n```\n%s\n```\n", f, string(content)))
	}
	return sb.String(), nil
}

func toolErrorSummary(log string) string {
	var errors []string
	lines := strings.Split(log, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "error") || strings.Contains(strings.ToLower(line), "critical") {
			errors = append(errors, strings.TrimSpace(line))
		}
	}
	return strings.Join(errors, "\n")
}

func emitToolUpdates(options LLMOptions, log string, status string) {
	if options.OnToolLog != nil {
		options.OnToolLog(log)
	}
	if options.OnToolStatus != nil {
		options.OnToolStatus(status)
	}
}

func emitLLMLog(options LLMOptions, label string, content string) {
	if options.OnLLMLog != nil {
		options.OnLLMLog(fmt.Sprintf("[%s]\n%s", label, content))
	}
}

func formatMessages(messages []OpenAIMessage) string {
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", strings.ToUpper(m.Role), m.Content))
	}
	return sb.String()
}

func truncateLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func sortedCommitList(allowedCommits map[string]struct{}) []string {
	var list []string
	if allowedCommits == nil {
		return list
	}
	for h := range allowedCommits {
		list = append(list, h)
	}
	sort.Strings(list)
	return list
}

func convertToLLMCMessages(messages []OpenAIMessage, system string) []llms.MessageContent {
	var out []llms.MessageContent
	if system != "" {
		out = append(out, llms.TextParts(llms.ChatMessageTypeSystem, system))
	}
	for _, m := range messages {
		role := llms.ChatMessageTypeGeneric
		switch m.Role {
		case "system":
			role = llms.ChatMessageTypeSystem
		case "user":
			role = llms.ChatMessageTypeHuman
		case "assistant":
			role = llms.ChatMessageTypeAI
		}
		out = append(out, llms.TextParts(role, m.Content))
	}
	return out
}

func ApplyTools(tools []ToolCall, tasks []gitdiff.TaskChange, allowedCommits map[string]struct{}) ([]gitdiff.TaskChange, string, string) {
	var logs []string
	var status string
	for _, toolCall := range tools {
		toolCall.Tool = strings.ToLower(toolCall.Tool)
		params := normalizeToolParams(toolCall.Tool, toolCall.Parameters)
		switch toolCall.Tool {
		case "create_task":
			intent := castString(params["intent"])
			if intent == "" {
				logs = append(logs, "Error: attempt to create task with empty intent")
				continue
			}
			newTask := gitdiff.TaskChange{
				TaskIntent: intent,
				Title:      castString(params["title"]),
				Details:    castString(params["details"]),
				Scope:      castString(params["scope"]),
				TaskType:   castString(params["type"]),
			}
			if newTask.TaskType == "" {
				newTask.TaskType = "delivery"
			}
			if h, ok := castInt(params["estimated_hours"]); ok {
				newTask.EstimatedHours = float64(h)
			}
			tasks = append(tasks, newTask)
			logs = append(logs, fmt.Sprintf("Success: created task with index %d", len(tasks)-1))
			status = fmt.Sprintf("Created task #%d: %s", len(tasks)-1, intent)

		case "edit_task", "update_task":
			idx, ok := castInt(params["index"])
			if !ok || idx < 0 || idx >= len(tasks) {
				logs = append(logs, fmt.Sprintf("Error: index %v is out of bounds (current max: %d)", params["index"], len(tasks)-1))
				continue
			}
			if intent := castString(params["intent"]); intent != "" {
				tasks[idx].TaskIntent = intent
			}
			if title := castString(params["title"]); title != "" {
				tasks[idx].Title = title
			}
			if details := castString(params["details"]); details != "" {
				tasks[idx].Details = details
			}
			if scope := castString(params["scope"]); scope != "" {
				tasks[idx].Scope = scope
			}
			if tType := castString(params["type"]); tType != "" {
				tasks[idx].TaskType = tType
			}
			if h, ok := castInt(params["estimated_hours"]); ok {
				tasks[idx].EstimatedHours = float64(h)
			}
			logs = append(logs, fmt.Sprintf("Success: updated task %d", idx))
			status = fmt.Sprintf("Updated task #%d", idx)

		case "delete_task", "remove_task":
			idx, ok := castInt(params["index"])
			if !ok || idx < 0 || idx >= len(tasks) {
				logs = append(logs, fmt.Sprintf("Error: index %v is out of bounds", params["index"]))
				continue
			}
			intent := tasks[idx].TaskIntent
			tasks = append(tasks[:idx], tasks[idx+1:]...)
			logs = append(logs, fmt.Sprintf("Success: deleted task %d", idx))
			status = fmt.Sprintf("Deleted task #%d: %s", idx, intent)

		case "add_commit_reference":
			idx, ok := castInt(params["index"])
			if !ok || idx < 0 || idx >= len(tasks) {
				logs = append(logs, "Error: invalid task index for commit reference")
				continue
			}
			hash := castString(params["hash"])
			if hash == "" {
				logs = append(logs, "Error: empty commit hash")
				continue
			}
			// Check if allowed
			if allowedCommits != nil {
				if _, ok := allowedCommits[hash]; !ok {
					logs = append(logs, fmt.Sprintf("Error: commit %s is not allowed for this day/repo", hash))
					continue
				}
			}
			// Add if not exists
			exists := false
			for _, c := range tasks[idx].Commits {
				if c == hash {
					exists = true
					break
				}
			}
			if !exists {
				tasks[idx].Commits = append(tasks[idx].Commits, hash)
				logs = append(logs, fmt.Sprintf("Success: added commit %s to task %d", hash, idx))
			} else {
				logs = append(logs, fmt.Sprintf("Info: commit %s already linked to task %d", hash, idx))
			}
			status = fmt.Sprintf("Linked commit %s to task #%d", hash[:7], idx)

		case "add_details":
			idx, ok := castInt(params["index"])
			if !ok || idx < 0 || idx >= len(tasks) {
				continue
			}
			newDetails := castString(params["details"])
			if newDetails != "" {
				if tasks[idx].Details != "" {
					tasks[idx].Details += "\n" + newDetails
				} else {
					tasks[idx].Details = newDetails
				}
				logs = append(logs, fmt.Sprintf("Success: added details to task %d", idx))
			}
			status = fmt.Sprintf("Added details to task #%d", idx)

		case "add_time":
			idx, ok := castInt(params["index"])
			if !ok || idx < 0 || idx >= len(tasks) {
				continue
			}
			hours, ok := castInt(params["hours"])
			if ok {
				tasks[idx].EstimatedHours += float64(hours)
				logs = append(logs, fmt.Sprintf("Success: added %d hours to task %d", hours, idx))
			}
			status = fmt.Sprintf("Added %d hours to task #%d", hours, idx)
		}
	}
	return tasks, strings.Join(logs, "\n"), status
}

func castString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func castInt(v interface{}) (int, bool) {
	if v == nil {
		return 0, false
	}
	switch val := v.(type) {
	case int:
		return val, true
	case float64:
		return int(val), true
	case string:
		i, err := strconv.Atoi(val)
		if err == nil {
			return i, true
		}
	}
	return 0, false
}

func castIntSlice(v interface{}) []int {
	if v == nil {
		return nil
	}
	var out []int
	switch slice := v.(type) {
	case []interface{}:
		for _, item := range slice {
			if i, ok := castInt(item); ok {
				out = append(out, i)
			}
		}
	case []int:
		return slice
	}
	return out
}

func castStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	var out []string
	switch slice := v.(type) {
	case []interface{}:
		for _, item := range slice {
			out = append(out, castString(item))
		}
	case []string:
		return slice
	}
	return out
}

func showStateDashboard(commitHash string, tasks []gitdiff.TaskChange, lastLog string, turn int, quiet bool) {
	if quiet {
		return
	}
	fmt.Printf("\r  [Turn %d] Incorporating %s | Current Tasks: %d                     \n", turn+1, commitHash, len(tasks))
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
		if t.Details != "" {
			lines := strings.Split(t.Details, "\n")
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
		hasCommits := len(t.Commits) > 0
		if (hasCommits || t.TaskIntent != "") && t.TaskIntent != "" {
			out = append(out, t)
		}
	}
	return out
}

func stripCodeFences(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "```") {
		lines := strings.Split(input, "\n")
		if len(lines) > 2 {
			// Remove first and last line
			return strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	return input
}

func extractJSONPayload(input string) string {
	input = strings.TrimSpace(input)
	start := strings.Index(input, "{")
	if start == -1 {
		start = strings.Index(input, "[")
	}
	if start == -1 {
		return input
	}
	end := strings.LastIndex(input, "}")
	if end == -1 {
		end = strings.LastIndex(input, "]")
	}
	if end == -1 || end <= start {
		return input
	}
	return input[start : end+1]
}

func findMatchingEnd(input string, start int, open, close rune) int {
	depth := 0
	for i, r := range input[start:] {
		if r == open {
			depth++
		} else if r == close {
			depth--
			if depth == 0 {
				return start + i
			}
		}
	}
	return -1
}
