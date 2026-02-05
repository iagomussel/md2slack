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
	Temperature   float64
	TopP          float64
	RepeatPenalty float64
	ContextSize   int
	ModelName     string
	BaseUrl       string
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

func SynthesizeTasks(commits []gitdiff.CommitChange, extraContext string, options LLMOptions) ([]gitdiff.TaskChange, error) {
	system := readPromptFile("task_synthesizer.txt")
	if system == "" {
		return nil, errors.New("prompt file task_synthesizer.txt not found")
	}

	prompt := fmt.Sprintf("Extra Context: %s\nCommits: %v", extraContext, commits)

	var out []gitdiff.TaskChange
	err := callJSON(prompt, system, options, &out)
	return out, err
}

func GroupTasks(tasks []gitdiff.TaskChange, options LLMOptions) ([]gitdiff.GroupedTask, error) {
	system := readPromptFile("task_grouper.txt")
	if system == "" {
		return nil, errors.New("prompt file task_grouper.txt not found")
	}

	prompt := fmt.Sprintf("Tasks: %v", tasks)

	var out []gitdiff.GroupedTask
	err := callJSON(prompt, system, options, &out)
	return out, err
}

func callJSON(prompt, system string, options LLMOptions, target interface{}) error {
	reqBody, _ := json.Marshal(OllamaRequest{
		Model:  options.ModelName,
		Prompt: prompt,
		System: system,
		Format: "json",
		Stream: false,
		Options: map[string]interface{}{
			"temperature":    0.1, // Fixed low temperature for determinism
			"top_p":          options.TopP,
			"repeat_penalty": options.RepeatPenalty,
			"num_ctx":        options.ContextSize,
		},
	})

	resp, err := http.Post(
		options.BaseUrl,
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama status %d", resp.StatusCode)
	}

	var out OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}

	raw := []byte(strings.TrimSpace(out.Response))

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
		return fmt.Errorf("unmarshal error: %v (response: %s)", err, out.Response)
	}

	return nil
}
