package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"md2slack/internal/gitdiff"
	"md2slack/internal/llm/tools"
	"strings"

	"github.com/tmc/langchaingo/llms"
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
	taskTools := tools.NewTaskTools(currentTasks)
	agent := NewAgent(options, taskTools)

	responseText, err := agent.StreamChat(history, system)
	if err != nil {
		return currentTasks, "", err
	}

	return taskTools.GetUpdatedTasks(), responseText, nil
}

// StreamAdapter wraps the LLM adapter to support streaming
type StreamAdapter interface {
	GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error)
	StreamContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (io.Reader, error)
}
