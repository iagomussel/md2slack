package main

import (
	"fmt"
	"md2slack/internal/config"
	"md2slack/internal/gitdiff"
	"md2slack/internal/llm"
	"md2slack/internal/slack"
	"md2slack/internal/storage"
	"md2slack/internal/webui"
	"net"
	"os"
	"strconv"
)

// startWebServer handles the initialization and startup of the web server
func startWebServer(cfg *config.Config, stageNames []string) *webui.Server {
	addr, err := resolveWebAddr(cfg.Server.Host, cfg.Server.Port, cfg.Server.AutoIncrementPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving web address: %v\n", err)
		os.Exit(1)
	}

	return webui.Start(addr, stageNames)
}

// SetupServerHandlers wires the processor's logic into the web server's callbacks
func SetupServerHandlers(server *webui.Server, p *ReportProcessor) {
	if server == nil || p == nil {
		return
	}

	server.SetHandlers(
		func(report string) error {
			if p.Debug {
				fmt.Println("Debug mode: skipping Slack send")
				return nil
			}
			return slack.SendMarkdown(&p.Config.Slack, report)
		},
		func(prompt string, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
			return llm.RefineTasksWithPrompt(tasks, prompt, p.LLMOpts)
		},
		func(repo string, date string, tasks []gitdiff.TaskChange, report string) error {
			if repo == "" {
				repo = "unknown"
			}
			if err := storage.ReplaceTasks(repo, date, tasks); err != nil {
				return err
			}
			return storage.SaveHistory(repo, date, report, "assistant")
		},
	)

	server.SetLoadClearHandlers(
		func(repo string, date string) ([]gitdiff.TaskChange, string, error) {
			repoName := gitdiff.GetRepoNameAt(repo)
			hist, err := storage.LoadHistory(repoName, date)
			if err != nil {
				return nil, "", err
			}
			if hist == nil {
				return nil, "", nil
			}
			tasks, err := storage.LoadTasks(repoName, date)
			if err != nil {
				tasks = nil
			}
			return tasks, hist.Message, nil
		},
		func(repo string, date string) error {
			repoName := gitdiff.GetRepoNameAt(repo)
			return storage.DeleteHistoryDB(repoName, date)
		},
	)

	server.SetActionHandler(
		func(action string, selected []int, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
			return llm.EditTasksWithAction(tasks, action, selected, p.LLMOpts)
		},
		func(index int, task gitdiff.TaskChange, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
			if index < 0 || index >= len(tasks) {
				return tasks, fmt.Errorf("index out of bounds")
			}
			tasks[index] = task
			return tasks, nil
		},
	)

	server.SetChatWithCallbacks(
		func(history []webui.OpenAIMessage, tasks []gitdiff.TaskChange, callbacks webui.ChatCallbacks) ([]gitdiff.TaskChange, string, error) {
			var llmHistory []llm.OpenAIMessage
			for _, msg := range history {
				llmHistory = append(llmHistory, llm.OpenAIMessage{Role: msg.Role, Content: msg.Content})
			}
			opts := p.LLMOpts
			opts.OnToolStart = callbacks.OnToolStart
			opts.OnToolEnd = callbacks.OnToolEnd
			opts.OnStreamChunk = callbacks.OnStreamChunk
			opts.OnTasksUpdate = callbacks.OnTasksUpdate

			return llm.StreamChatWithRequests(llmHistory, tasks, opts, nil)
		},
	)
}

// resolveWebAddr finds an available port to bind the server to
func resolveWebAddr(host string, port int, autoIncrement bool) (string, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if port <= 0 {
		port = 8080
	}

	maxTries := 20
	for i := 0; i < maxTries; i++ {
		candidate := port + i
		addr := net.JoinHostPort(host, strconv.Itoa(candidate))
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			_ = ln.Close()
			return addr, nil
		}
		if !autoIncrement {
			return "", fmt.Errorf("port %d unavailable", port)
		}
	}
	return "", fmt.Errorf("no available port found after %d attempts", maxTries)
}
