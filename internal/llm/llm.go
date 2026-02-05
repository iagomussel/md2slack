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

type OllamaRequest struct {
	Model   string                 `json:"model"`
	Prompt  string                 `json:"prompt"`
	System  string                 `json:"system"`
	Format  string                 `json:"format,omitempty"`
	Stream  bool                   `json:"stream"`
	Options map[string]interface{} `json:"options"`
}

type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
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
	err := callJSON(prompt, system, options, &out)
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
	err := callJSON(prompt, system, options, &out)
	return out, err
}

func IncorporateCommit(commit gitdiff.CommitChange, currentTasks []gitdiff.TaskChange, extraContext string, options LLMOptions) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_tools.txt")
	if system == "" {
		return nil, errors.New("prompt file task_tools.txt not found")
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
		tasksState = "(no tasks yet)"
	}

	commitJSON, _ := json.MarshalIndent(commit, "", "  ")

	prompt := fmt.Sprintf("Extra Context: %s\nCurrent Tasks (State):\n%s\nNew Commit: %s", extraContext, tasksState, string(commitJSON))

	var tools []ToolCall
	err := callJSON(prompt, system, options, &tools)
	if err != nil {
		return nil, err
	}

	return ApplyTools(tools, currentTasks), nil
}

func ApplyTools(tools []ToolCall, tasks []gitdiff.TaskChange) []gitdiff.TaskChange {
	for _, tc := range tools {
		params := tc.Parameters
		switch tc.Tool {
		case "create_task":
			intent := castString(params["intent"])
			if intent == "" {
				continue // Skip empty tasks
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

		case "edit_task":
			idx, ok := castInt(params["index"])
			if ok && idx >= 0 && idx < len(tasks) {
				if intent := castString(params["intent"]); intent != "" {
					tasks[idx].TaskIntent = intent
				}
				if scope := castString(params["scope"]); scope != "" {
					tasks[idx].Scope = scope
				}
			}

		case "add_details":
			idx, ok := castInt(params["index"])
			if ok && idx >= 0 && idx < len(tasks) {
				detail := castString(params["technical_why"])
				if detail != "" && !strings.Contains(detail, "...") {
					if tasks[idx].TechnicalWhy == "" {
						tasks[idx].TechnicalWhy = detail
					} else {
						// Avoid duplicate details
						if !strings.Contains(tasks[idx].TechnicalWhy, detail) {
							tasks[idx].TechnicalWhy += "\n" + detail
						}
					}
				}
			}

		case "add_time":
			idx, ok := castInt(params["index"])
			if ok && idx >= 0 && idx < len(tasks) {
				if h, ok := castInt(params["hours"]); ok {
					if tasks[idx].EstimatedHours == nil {
						tasks[idx].EstimatedHours = &h
					} else {
						newH := *tasks[idx].EstimatedHours + h
						tasks[idx].EstimatedHours = &newH
					}
				}
			}

		case "add_commit_reference":
			idx, ok := castInt(params["index"])
			if ok && idx >= 0 && idx < len(tasks) {
				hash := castString(params["hash"])
				if hash != "" {
					// Avoid duplicate hashes
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
				}
			}
		}
	}
	return tasks
}

// Helpers for casting (aliased from gitdiff to avoid circular dependency if needed,
// but here we can just use those that are already in gitdiff if they are exported.
// Wait, they are not exported. I'll re-implement them or move them.
// For now, I'll implement simple versions here to avoid breaking things.

func castString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
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
	err := callJSON(prompt, system, options, &out)
	return out, err
}

func callJSON(prompt, system string, options LLMOptions, target interface{}) error {
	var body []byte
	var url string

	switch strings.ToLower(options.Provider) {
	case "codex", "openai":
		url = options.BaseUrl
		if url == "" {
			url = "https://api.openai.com/v1/chat/completions"
		}
		if !strings.HasSuffix(url, "/chat/completions") && !strings.HasSuffix(url, "/completions") {
			// Try to append if it's just a base URL
			url = strings.TrimSuffix(url, "/") + "/chat/completions"
		}

		req := OpenAIRequest{
			Model: options.ModelName,
			Messages: []OpenAIMessage{
				{Role: "system", Content: system},
				{Role: "user", Content: prompt},
			},
		}
		body, _ = json.Marshal(req)

	default: // ollama
		url = options.BaseUrl
		if url == "" {
			url = "http://localhost:11434/api/generate"
		}
		req := OllamaRequest{
			Model:  options.ModelName,
			Prompt: prompt,
			System: system,
			Format: "json",
			Stream: false,
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
		var ollama OllamaResponse
		if err := json.NewDecoder(resp.Body).Decode(&ollama); err != nil {
			return err
		}
		responseText = ollama.Response
	}

	if options.Debug {
		fmt.Printf("\n--- RAW LLM RESPONSE (%s) ---\n%s\n--- END RAW RESPONSE ---\n\n", url, responseText)
	}

	clean := strings.TrimSpace(responseText)
	clean = stripCodeFences(clean)
	clean = extractJSONPayload(clean)
	raw := []byte(strings.TrimSpace(clean))

	// Robust unmarshal: if target is a slice but response is an object, wrap it
	err = json.Unmarshal(raw, target)
	if err != nil {
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
