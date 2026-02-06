package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"md2slack/internal/config"
	"md2slack/internal/gitdiff"
	"md2slack/internal/llm"
	"md2slack/internal/renderer"
	"md2slack/internal/slack"
	"md2slack/internal/storage"
	"md2slack/internal/tui"
	"md2slack/internal/webui"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	var debug bool
	var install bool
	var web bool
	var webAddr string
	flag.BoolVar(&debug, "debug", false, "Enable debug mode")
	flag.BoolVar(&install, "install", false, "Install the binary")
	flag.BoolVar(&web, "web", false, "Enable web UI")
	flag.StringVar(&webAddr, "web-addr", "127.0.0.1:8080", "Web UI address")
	flag.Parse()

	if install {
		fmt.Println("Installing in user mode (no sudo required)...")

		// 1. Link Project Directory to ~/.md2slack
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting user home directory: %v\n", err)
			return
		}

		targetLink := filepath.Join(home, ".md2slack")

		// Use current directory as the source for the link
		absCwd, err := filepath.Abs(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error determining current directory: %v\n", err)
			return
		}

		// Remove existing symlink or directory
		if stats, err := os.Lstat(targetLink); err == nil {
			if stats.Mode()&os.ModeSymlink != 0 {
				fmt.Println("Removing existing ~/.md2slack symlink...")
				os.Remove(targetLink)
			} else if stats.IsDir() {
				fmt.Println("Backing up existing ~/.md2slack directory to ~/.md2slack.bak")
				os.Rename(targetLink, targetLink+".bak")
			}
		}

		if err := os.Symlink(absCwd, targetLink); err != nil {
			fmt.Fprintf(os.Stderr, "Error linking ~/.md2slack: %v\n", err)
			return
		} else {
			fmt.Printf("Linked ~/.md2slack -> %s\n", absCwd)
		}

		// 2. Advise on PATH
		fmt.Printf("\nInstallation successful!\n")
		fmt.Printf("Please add the following to your ~/.bashrc or ~/.zshrc:\n\n")
		fmt.Printf("export PATH=$PATH:$HOME/.md2slack\n\n")
		fmt.Printf("Then run: source ~/.bashrc\n")

		return
	}

	args := flag.Args()
	if len(args) < 1 && !web {
		fmt.Println("usage: ssbot [--debug] [--web] [--web-addr host:port] [--install] [<MM-DD-YYYY>] [extra context]")
		os.Exit(1)
	}

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
		if web {
			resolved, err := resolveWebAddr(cfg.Server.Host, cfg.Server.Port, cfg.Server.AutoIncrementPort)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error resolving web address: %v\n", err)
				os.Exit(1)
			}
			webAddr = resolved
		}
	}

	baseUrl := cfg.LLM.BaseURL

	llmOpts := llm.LLMOptions{
		Provider:      cfg.LLM.Provider,
		ModelName:     cfg.LLM.Model,
		Temperature:   cfg.LLM.Temperature,
		TopP:          cfg.LLM.TopP,
		RepeatPenalty: cfg.LLM.RepeatPenalty,
		ContextSize:   cfg.LLM.ContextSize,
		BaseUrl:       baseUrl,
		Token:         cfg.LLM.Token,
		Debug:         debug,
		Timeout:       2 * time.Minute,
		Heartbeat:     5 * time.Second,
	}

	stageNames := []string{
		"Preparing commit context",
		"Summarizing commits",
		"Generating tasks",
		"Reviewing tasks",
		"Suggesting next actions",
		"Rendering report",
	}

	var webServer *webui.Server
	if web {
		webServer = webui.Start(webAddr, stageNames)
	}

	processDate := func(date string, repoPath string, authorOverride string) {
		date = strings.TrimSpace(date)
		if date == "" {
			return
		}
		repoName := gitdiff.GetRepoNameAt(repoPath)
		fmt.Printf("\n--- Processing Date: %s (Repo: %s) ---\n", date, repoName)
		runStart := time.Now()
		type uiController interface {
			StageStart(int, string)
			StageDone(int, string)
			Log(string)
			Error(string)
			Status(string)
			Stop()
		}

		var ui uiController
		if webServer != nil {
			webServer.Reset(stageNames, date, repoName)
			ui = webServer
		} else if !debug {
			ui = tui.Start(stageNames)
			defer func() {
				if ui != nil {
					ui.Stop()
				}
			}()
		}

		llmOpts.Quiet = ui != nil
		if ui != nil {
			llmOpts.OnToolLog = ui.Log
			llmOpts.OnToolStatus = ui.Status
			llmOpts.OnLLMLog = ui.Log
		} else {
			llmOpts.OnToolLog = nil
			llmOpts.OnToolStatus = nil
			llmOpts.OnLLMLog = nil
		}

		logf := func(format string, args ...interface{}) {
			msg := fmt.Sprintf(format, args...)
			if ui != nil {
				ui.Log(msg)
				return
			}
			fmt.Println(msg)
		}
		errf := func(format string, args ...interface{}) {
			msg := fmt.Sprintf(format, args...)
			if ui != nil {
				ui.Error(msg)
				return
			}
			fmt.Println(msg)
		}

		// --- STAGE 0: Preparing commit context ---
		stageStart := time.Now()
		if ui != nil {
			ui.StageStart(0, "")
		}
		output, err := gitdiff.GenerateFactsWithOptions(date, extra, repoPath, authorOverride)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating facts for %s: %v\n", date, err)
			if ui != nil {
				ui.Error(err.Error())
			}
			return
		}
		if ui != nil {
			ui.StageDone(0, fmt.Sprintf("%d commits found", len(output.Commits)))
		}
		logf("Stage 0 done in %s", time.Since(stageStart).Truncate(time.Millisecond))

		// --- STAGE 1: Summarizing commits (Parallel) ---
		stageStart = time.Now()
		if ui != nil {
			ui.StageStart(1, "")
		}
		logf("Summarizing %d commits in parallel...", len(output.Commits))

		type commitResult struct {
			index int
			cc    *gitdiff.CommitChange
			err   error
		}
		results := make(chan commitResult, len(output.Commits))
		for i, commit := range output.Commits {
			go func(idx int, c gitdiff.Commit) {
				var semantic gitdiff.CommitSemantic
				for _, s := range output.Semantic {
					if s.CommitHash == c.Hash {
						semantic = s
						break
					}
				}

				cc, err := llm.ExtractCommitIntent(gitdiff.SemanticChange{
					CommitHash: c.Hash,
					Signals:    semantic.Signals,
				}, c.Message, llmOpts)
				if err != nil {
					results <- commitResult{index: idx, err: err}
					return
				}
				results <- commitResult{index: idx, cc: cc}
			}(i, commit)
		}

		commitChanges := make([]gitdiff.CommitChange, len(output.Commits))
		for i := 0; i < len(output.Commits); i++ {
			res := <-results
			if res.err != nil {
				errf("Error analyzing commit: %v", res.err)
				continue
			}
			commitChanges[res.index] = *res.cc
		}

		if ui != nil {
			ui.StageDone(1, fmt.Sprintf("%d analyzed", len(commitChanges)))
		}
		logf("Stage 1 done in %s", time.Since(stageStart).Truncate(time.Millisecond))

		// --- STAGE 2: Generating tasks ---
		stageStart = time.Now()
		if ui != nil {
			ui.StageStart(2, "")
		}
		logf("Generating tasks (Manual first, then Commits)...")

		manualTasks, _ := llm.IncorporateExtraContext(output.Extra, llmOpts)
		var commitTasks []gitdiff.TaskChange

		allowedCommits := make(map[string]struct{})
		for _, c := range output.Commits {
			allowedCommits[c.Hash] = struct{}{}
		}

		for i, cc := range commitChanges {
			if cc.CommitHash == "" {
				continue
			}
			logf("  [%d/%d] Incorporating commit %s...", i+1, len(commitChanges), cc.CommitHash)
			updated, err := llm.IncorporateCommit(cc, commitTasks, manualTasks, output.Extra, llmOpts, allowedCommits)
			if err != nil {
				errf("Error incorporating commit %s: %v", cc.CommitHash, err)
				continue
			}
			commitTasks = updated
		}

		allTasks := append([]gitdiff.TaskChange{}, manualTasks...)
		allTasks = append(allTasks, commitTasks...)

		if ui != nil {
			ui.StageDone(2, fmt.Sprintf("%d tasks", len(allTasks)))
		}
		logf("Stage 2 done in %s", time.Since(stageStart).Truncate(time.Millisecond))

		// --- STAGE 3: Reviewing tasks ---
		stageStart = time.Now()
		if ui != nil {
			ui.StageStart(3, "")
		}
		logf("Reviewing and refining tasks...")
		allTasks, err = llm.RefineTasks(allTasks, llmOpts)
		if err != nil {
			errf("Warning: task refinement failed: %v", err)
		}
		if ui != nil {
			ui.StageDone(3, "Refined")
		}
		logf("Stage 3 done in %s", time.Since(stageStart).Truncate(time.Millisecond))

		// --- STAGE 4: Suggesting next actions ---
		stageStart = time.Now()
		if ui != nil {
			ui.StageStart(4, "")
		}
		nextActions, err := llm.SuggestNextActions(allTasks, llmOpts)
		if err != nil {
			errf("Warning: failed to suggest next actions: %v", err)
		}
		if ui != nil {
			ui.StageDone(4, fmt.Sprintf("%d actions", len(nextActions)))
		}
		logf("Stage 4 done in %s", time.Since(stageStart).Truncate(time.Millisecond))

		// --- STAGE 5: Rendering report ---
		stageStart = time.Now()
		if ui != nil {
			ui.StageStart(5, "")
		}
		report := renderer.RenderReport(date, nil, allTasks, nextActions)
		if webServer != nil {
			webServer.SetTasks(allTasks, nextActions)
			webServer.SetReport(report)
			webServer.SetHandlers(
				func(report string) error {
					if debug {
						logf("Debug: send requested; skipping Slack send")
						return nil
					}
					return slack.SendMarkdown(&cfg.Slack, report)
				},
				func(prompt string, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
					return llm.RefineTasksWithPrompt(tasks, prompt, llmOpts)
				},
				func(date string, tasks []gitdiff.TaskChange, report string) error {
					return storage.SaveHistory(repoName, date, tasks, nil, nil, report)
				},
			)
			webServer.SetActionHandler(func(action string, selected []int, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
				return llm.EditTasksWithAction(tasks, action, selected, llmOpts)
			})
		}

		if ui != nil {
			ui.StageDone(5, "ready")
			if webServer == nil {
				ui.Stop()
				ui = nil
			}
		}
		logf("Stage 5 done in %s", time.Since(stageStart).Truncate(time.Millisecond))
		fmt.Println("\n--- FINAL REPORT ---")
		fmt.Println(report)

		// Save History
		if err := storage.SaveHistory(repoName, date, allTasks, nil, nil, report); err != nil {
			errf("Warning: failed to save history for %s: %v", date, err)
		}

		if debug {
			fmt.Println("--- LLM Report ---")
			fmt.Println(report)
			fmt.Println("--- Slack Blocks ---")
			blocks, err := slack.ConvertToBlocks(report)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error converting to blocks: %v\n", err)
				return
			}
			b, _ := json.MarshalIndent(blocks, "", "  ")
			fmt.Println(string(b))
		} else if webServer == nil {
			fmt.Println("Sending to Slack...")
			if err := slack.SendMarkdown(&cfg.Slack, report); err != nil {
				fmt.Fprintf(os.Stderr, "Error sending to Slack for %s: %v\n", date, err)
				return
			}
			fmt.Printf("Daily Status Report for %s sent successfully!\n", date)
		} else {
			fmt.Println("Web UI enabled: report ready; use the Send button to post to Slack.")
		}
		logf("Total elapsed: %s", time.Since(runStart).Truncate(time.Millisecond))
	}

	if len(dates) == 0 && webServer != nil {
		for req := range webServer.RunChannel() {
			processDate(req.Date, req.RepoPath, req.Author)
		}
		return
	}

	for _, date := range dates {
		processDate(date, "", "")
	}
}

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
