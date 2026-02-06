package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"md2slack/internal/gitdiff"
	"strings"
)

func ChatWithRequests(history []OpenAIMessage, currentTasks []gitdiff.TaskChange, options LLMOptions, allowedCommits map[string]struct{}) ([]gitdiff.TaskChange, string, error) {
	system := readPromptFile("task_chat.txt")
	if system == "" {
		return currentTasks, "System error: prompt file task_chat.txt not found", errors.New("prompt file task_chat.txt not found")
	}

	tasksJSON, _ := json.MarshalIndent(currentTasks, "", "  ")
	// Replace placeholder in system prompt
	system = strings.Replace(system, "{{TASKS_JSON}}", string(tasksJSON), 1)

	type ChatOutput struct {
		Text  string     `json:"text,omitempty"`
		Tools []ToolCall `json:"tools,omitempty"`
	}
	var out ChatOutput
	// We call callJSON directly with history
	err := callJSON(history, system, options, &out, getNativeTools()...)

	if err != nil {
		fmt.Printf("Chat LLM Error: %v\n", err)
		return currentTasks, "I'm sorry, I couldn't process that request. (LLM Error)", nil
	}

	responseText := ""
	if out.Text != "" {
		responseText = out.Text
	}

	// Apply tools
	if len(out.Tools) > 0 {
		var status string
		var log string
		var updatedTasks []gitdiff.TaskChange
		updatedTasks, log, status = ApplyTools(out.Tools, currentTasks, allowedCommits)

		// Emit logs if requested in options
		emitToolUpdates(options, log, status)

		// If the response text is empty, provide a summary of actions
		if responseText == "" {
			responseText = fmt.Sprintf("Executed actions: %s", status)
		} else if status != "" {
			responseText = fmt.Sprintf("%s\n\n(Action: %s)", responseText, status)
		}
		return updatedTasks, responseText, nil
	}

	return currentTasks, responseText, nil
}
