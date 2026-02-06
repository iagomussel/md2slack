package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"md2slack/internal/llm/tools"
	"strings"

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

// StreamChat runs a chat session with streaming and tools.
// Returns the response text and whether any tools were executed.
func (a *Agent) StreamChat(history []OpenAIMessage, systemPrompt string) (string, bool, error) {
	// Prepare messages
	messages := convertToLLMCMessages(history, systemPrompt)

	ctx := context.Background()
	toolUsed := false

	var streamBuf strings.Builder
	callOpts := []llms.CallOption{
		llms.WithTools(tools.GetLLMDefinitions()),
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			streamBuf.Write(chunk)
			if a.Options.OnStreamChunk != nil {
				a.Options.OnStreamChunk(string(chunk))
			}
			return nil
		}),
		llms.WithRepetitionPenalty(a.Options.RepeatPenalty),
	}
	provider := strings.ToLower(a.Options.Provider)
	if provider == "anthropic" {
		if a.Options.Temperature > 0 {
			callOpts = append(callOpts, llms.WithTemperature(a.Options.Temperature))
		} else if a.Options.TopP > 0 && a.Options.TopP < 1.0 {
			callOpts = append(callOpts, llms.WithTopP(a.Options.TopP))
		}
	} else {
		callOpts = append(callOpts, llms.WithTemperature(a.Options.Temperature))
		if a.Options.TopP > 0 && a.Options.TopP < 1.0 {
			callOpts = append(callOpts, llms.WithTopP(a.Options.TopP))
		}
	}

	adapter, err := createLLM(ctx, a.Options)
	if err != nil {
		return "", toolUsed, err
	}

	// Tool loop
	maxTurns := 5
	currentMessages := messages

	for i := 0; i < maxTurns; i++ {
		streamBuf.Reset()
		// Call LLM
		resp, err := adapter.GenerateContent(ctx, currentMessages, callOpts...)
		if err != nil {
			return "", toolUsed, err
		}

		choice := resp.Choices[0]
		responseText := choice.Content
		if responseText == "" && streamBuf.Len() > 0 {
			responseText = streamBuf.String()
		}
		log.Printf("[llm.StreamChat] turn=%d toolCalls=%d contentLen=%d streamLen=%d",
			i+1, len(choice.ToolCalls), len(choice.Content), streamBuf.Len())

		// If there are tool calls
		if len(choice.ToolCalls) > 0 {
			toolUsed = true
			// Add assistant message with tool calls to history (no empty text blocks)
			var parts []llms.ContentPart
			if strings.TrimSpace(responseText) != "" {
				parts = append(parts, llms.TextContent{Text: responseText})
			}
			for _, tc := range choice.ToolCalls {
				parts = append(parts, llms.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					FunctionCall: &llms.FunctionCall{
						Name:      tc.FunctionCall.Name,
						Arguments: tc.FunctionCall.Arguments,
					},
				})
			}
			currentMessages = append(currentMessages, llms.MessageContent{
				Role:  llms.ChatMessageTypeAI,
				Parts: parts,
			})

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

				// Add tool response to history (proper tool message)
				currentMessages = append(currentMessages, llms.MessageContent{
					Role: llms.ChatMessageTypeTool,
					Parts: []llms.ContentPart{
						llms.ToolCallResponse{
							ToolCallID: tc.ID,
							Name:       tc.FunctionCall.Name,
							Content:    result,
						},
					},
				})
			}

			// Continue loop to let LLM respond to tool results
			continue
		}

		// No tool calls, we are done
		return responseText, toolUsed, nil
	}

	return "Max turns reached", toolUsed, nil
}

// ForceToolCalls asks the model to respond only with tool calls.
// Returns parsed tool calls and the raw response text.
func (a *Agent) ForceToolCalls(history []OpenAIMessage, systemPrompt string) ([]ToolCall, string, error) {
	forcedSystem := systemPrompt + "\n\nIMPORTANT: Respond ONLY with tool calls. Do not include any prose."
	messages := convertToLLMCMessages(history, forcedSystem)

	ctx := context.Background()
	var streamBuf strings.Builder

	callOpts := []llms.CallOption{
		llms.WithTools(tools.GetLLMDefinitions()),
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			streamBuf.Write(chunk)
			return nil
		}),
		llms.WithRepetitionPenalty(a.Options.RepeatPenalty),
	}

	provider := strings.ToLower(a.Options.Provider)
	if provider == "anthropic" {
		if a.Options.Temperature > 0 {
			callOpts = append(callOpts, llms.WithTemperature(a.Options.Temperature))
		} else if a.Options.TopP > 0 && a.Options.TopP < 1.0 {
			callOpts = append(callOpts, llms.WithTopP(a.Options.TopP))
		}
	} else {
		callOpts = append(callOpts, llms.WithTemperature(a.Options.Temperature))
		if a.Options.TopP > 0 && a.Options.TopP < 1.0 {
			callOpts = append(callOpts, llms.WithTopP(a.Options.TopP))
		}
	}

	adapter, err := createLLM(ctx, a.Options)
	if err != nil {
		return nil, "", err
	}

	resp, err := adapter.GenerateContent(ctx, messages, callOpts...)
	if err != nil {
		return nil, "", err
	}

	choice := resp.Choices[0]
	responseText := choice.Content
	if responseText == "" && streamBuf.Len() > 0 {
		responseText = streamBuf.String()
	}

	if len(choice.ToolCalls) > 0 {
		return convertToolCalls(choice.ToolCalls), responseText, nil
	}

	return parseToolCallsFromText(responseText), responseText, nil
}

func convertToolCalls(calls []llms.ToolCall) []ToolCall {
	var out []ToolCall
	for _, tc := range calls {
		var params map[string]interface{}
		_ = json.Unmarshal([]byte(tc.FunctionCall.Arguments), &params)
		out = append(out, ToolCall{
			Tool:       tc.FunctionCall.Name,
			Parameters: params,
		})
	}
	return out
}
