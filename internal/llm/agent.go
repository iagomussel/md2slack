package llm

import (
	"context"
	"fmt"
	"md2slack/internal/llm/tools"

	"github.com/tmc/langchaingo/llms"
)

// Agent handles interactions with the LLM using tools
type Agent struct {
	Options LLMOptions
	Tools   *tools.TaskTools
}

// NewAgent creates a new agent with the given options and tools
func NewAgent(opts LLMOptions, taskTools *tools.TaskTools) *Agent {
	return &Agent{
		Options: opts,
		Tools:   taskTools,
	}
}

// StreamChat runs a chat session with streaming and tools
func (a *Agent) StreamChat(history []OpenAIMessage, systemPrompt string) (string, error) {
	// Prepare messages
	messages := convertToLLMCMessages(history, systemPrompt)

	ctx := context.Background()

	callOpts := []llms.CallOption{
		llms.WithTools(tools.GetLLMDefinitions()),
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			if a.Options.OnStreamChunk != nil {
				a.Options.OnStreamChunk(string(chunk))
			}
			return nil
		}),
		llms.WithTemperature(a.Options.Temperature),
		llms.WithRepetitionPenalty(a.Options.RepeatPenalty),
	}

	adapter, err := createLLM(ctx, a.Options)
	if err != nil {
		return "", err
	}

	// Tool loop
	maxTurns := 5
	currentMessages := messages

	for i := 0; i < maxTurns; i++ {
		// Call LLM
		resp, err := adapter.GenerateContent(ctx, currentMessages, callOpts...)
		if err != nil {
			return "", err
		}

		choice := resp.Choices[0]

		// If there are tool calls
		if len(choice.ToolCalls) > 0 {
			// Add assistant message with tool calls to history
			currentMessages = append(currentMessages, llms.TextParts(llms.ChatMessageTypeAI, choice.Content))

			for _, tc := range choice.ToolCalls {
				// Notify tool start
				if a.Options.OnToolStart != nil {
					a.Options.OnToolStart(tc.FunctionCall.Name, tc.FunctionCall.Arguments)
				}

				// Execute tool
				var result string
				var toolErr error

				tool, found := a.Tools.Find(tc.FunctionCall.Name)
				if found {
					result, toolErr = tool.Call(ctx, tc.FunctionCall.Arguments)
				} else {
					result = "Error: Tool not found"
					toolErr = fmt.Errorf("tool not found: %s", tc.FunctionCall.Name)
				}

				if toolErr != nil {
					result = fmt.Sprintf("Error executing tool: %v", toolErr)
				}

				// Notify tool end
				if a.Options.OnToolEnd != nil {
					a.Options.OnToolEnd(tc.FunctionCall.Name, result)
				}

				// Add tool response to history
				currentMessages = append(currentMessages, llms.TextParts(llms.ChatMessageTypeSystem, fmt.Sprintf("Tool %s result: %s", tc.FunctionCall.Name, result)))
			}

			// Continue loop to let LLM respond to tool results
			continue
		}

		// No tool calls, we are done
		return choice.Content, nil
	}

	return "Max turns reached", nil
}
