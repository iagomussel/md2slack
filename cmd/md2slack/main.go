package main

import (
	"flag"
	"fmt"
	"md2slack/internal/config"
	"md2slack/internal/gitdiff"
	"md2slack/internal/llm"
	"md2slack/internal/storage"
	"md2slack/internal/webui"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	var debug bool
	var install bool
	var webAddr string
	flag.BoolVar(&debug, "debug", false, "Enable debug mode")
	flag.BoolVar(&install, "install", false, "Install the binary")
	flag.StringVar(&webAddr, "web-addr", "127.0.0.1:8080", "Web UI address")
	flag.Parse()

	if install {
		runInstall()
		return
	}

	args := flag.Args()
	// Web UI is always enabled, dates are optional

	var dates []string
	extra := ""
	if len(args) > 0 {
		dates = []string{args[0]}
		if strings.Contains(args[0], ",") {
			dates = strings.Split(args[0], ",")
		}
		if len(args) > 1 {
			extra = strings.Join(args[1:], " ")
			// Clean terminal artifacts like bracketed paste markers
			re := regexp.MustCompile(`(?i)\x1b\[\d+~`)
			extra = re.ReplaceAllString(extra, "")
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	flagWebAddr := flag.Lookup("web-addr")
	webAddrDefault := "127.0.0.1:8080"
	if flagWebAddr != nil {
		if flagWebAddr.DefValue != "" {
			webAddrDefault = flagWebAddr.DefValue
		}
	}

	if webAddr == webAddrDefault {
		webAddr = net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
		resolved, err := resolveWebAddr(cfg.Server.Host, cfg.Server.Port, cfg.Server.AutoIncrementPort)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving web address: %v\n", err)
			os.Exit(1)
		}
		webAddr = resolved
	}

	stageNames := []string{
		"Preparing commit context",
		"Summarizing commits",
		"Generating tasks",
		"Reviewing tasks",
		"Suggesting next actions",
		"Rendering report",
	}

	// Always start web server
	webServer := webui.Start(webAddr, stageNames)

	processor := &ReportProcessor{
		Config: cfg,
		LLMOpts: llm.LLMOptions{
			Provider:      cfg.LLM.Provider,
			ModelName:     cfg.LLM.Model,
			Temperature:   cfg.LLM.Temperature,
			TopP:          cfg.LLM.TopP,
			RepeatPenalty: cfg.LLM.RepeatPenalty,
			ContextSize:   cfg.LLM.ContextSize,
			BaseUrl:       cfg.LLM.BaseURL,
			Token:         cfg.LLM.Token,
			Timeout:       2 * time.Minute,
		},
		WebServer:  webServer,
		Debug:      debug,
		StageNames: stageNames,
	}

	if len(dates) == 0 {
		// Register load/clear handlers immediately so they're available before any analysis runs
		webServer.SetLoadClearHandlers(
			func(repo string, date string) ([]gitdiff.TaskChange, string, error) {
				repoName := gitdiff.GetRepoNameAt(repo)
				hist, err := storage.LoadHistory(repoName, date)
				if err != nil {
					return nil, "", err
				}
				if hist == nil {
					return nil, "", nil
				}
				return hist.Tasks, hist.Report, nil
			},
			func(repo string, date string) error {
				repoName := gitdiff.GetRepoNameAt(repo)
				return storage.DeleteHistoryDB(repoName, date)
			},
		)

		// Register action handlers immediately so they're available before any analysis runs
		webServer.SetActionHandler(
			func(action string, selected []int, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
				return llm.EditTasksWithAction(tasks, action, selected, processor.LLMOpts)
			},
			func(index int, task gitdiff.TaskChange, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
				if index < 0 || index >= len(tasks) {
					return tasks, fmt.Errorf("index out of bounds")
				}
				tasks[index] = task
				return tasks, nil
			},
		)

		// Register chat handler with callbacks for streaming tool events
		webServer.SetChatWithCallbacks(
			func(history []webui.OpenAIMessage, tasks []gitdiff.TaskChange, callbacks webui.ChatCallbacks) ([]gitdiff.TaskChange, string, error) {
				var llmHistory []llm.OpenAIMessage
				for _, msg := range history {
					llmHistory = append(llmHistory, llm.OpenAIMessage{Role: msg.Role, Content: msg.Content})
				}
				// Create LLM options with callbacks
				opts := processor.LLMOpts
				opts.OnToolStart = callbacks.OnToolStart
				opts.OnToolEnd = callbacks.OnToolEnd
				opts.OnStreamChunk = callbacks.OnStreamChunk

				updated, text, err := llm.StreamChatWithRequests(llmHistory, tasks, opts, nil)
				return updated, text, err
			},
		)

		for req := range webServer.RunChannel() {
			processor.ProcessDate(req.Date, req.RepoPath, req.Author, "")
		}
		return
	}

	for _, date := range dates {
		processor.ProcessDate(date, "", "", extra)
	}
}
