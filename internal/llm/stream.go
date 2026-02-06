package llm

import (
	"encoding/json"
	"fmt"
	"log"
	"md2slack/internal/gitdiff"
	"md2slack/internal/llm/tools"
	"strings"
)

// StreamChatWithRequests handles streaming chat with tool calls
func StreamChatWithRequests(history []OpenAIMessage, currentTasks []gitdiff.TaskChange, options LLMOptions, allowedCommits map[string]struct{}) ([]gitdiff.TaskChange, string, error) {
	system := readPromptFile("task_chat.txt")
	if system == "" {
		return currentTasks, "System error: prompt file task_chat.txt not found", fmt.Errorf("prompt file task_chat.txt not found")
	}

	tasksJSON, _ := json.MarshalIndent(currentTasks, "", "  ")
	system = strings.Replace(system, "{{TASKS_JSON}}", string(tasksJSON), 1)

	// Create Task tools
	taskTools := tools.NewTaskTools(options.RepoName, options.Date, currentTasks)
	agent := NewAgent(options, taskTools)

	responseText, toolUsed, err := agent.StreamChat(history, system)
	if err != nil {
		return currentTasks, "", err
	}
	if toolUsed {
		return taskTools.GetUpdatedTasks(), responseText, nil
	}

	parsedTools := parseToolCallsFromText(responseText)
	log.Printf("[llm.StreamChatWithRequests] toolUsed=false parsedTools=%d responseLen=%d response=%q",
		len(parsedTools),
		len(responseText),
		truncateForLog(responseText, 200),
	)
	if len(parsedTools) == 0 {
		forcedTools, forcedText, err := agent.ForceToolCalls(history, system)
		if err != nil {
			return taskTools.GetUpdatedTasks(), responseText, nil
		}
		log.Printf("[llm.StreamChatWithRequests] forcedTools=%d forcedLen=%d forced=%q",
			len(forcedTools),
			len(forcedText),
			truncateForLog(forcedText, 200),
		)
		parsedTools = forcedTools
		if len(parsedTools) == 0 {
			return taskTools.GetUpdatedTasks(), responseText, nil
		}
	}

	if options.OnToolStart != nil {
		for _, tool := range parsedTools {
			paramsJSON, _ := json.Marshal(tool.Parameters)
			options.OnToolStart(tool.Tool, string(paramsJSON))
		}
	}

	updatedTasks, log, status := ApplyTools(parsedTools, currentTasks, allowedCommits)

	if options.OnToolEnd != nil {
		resultData := map[string]interface{}{
			"log":    log,
			"status": status,
		}
		resultJSON, _ := json.Marshal(resultData)
		for _, tool := range parsedTools {
			options.OnToolEnd(tool.Tool, string(resultJSON))
		}
	}
	emitToolUpdates(options, log, status)

	if responseText == "" {
		responseText = status
	} else if status != "" {
		responseText = fmt.Sprintf("%s\n\n(Action: %s)", responseText, status)
	}

	return updatedTasks, responseText, nil
}

func truncateForLog(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
