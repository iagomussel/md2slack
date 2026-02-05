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
