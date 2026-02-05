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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func main() {
	var debug bool
	var install bool
	flag.BoolVar(&debug, "debug", false, "Enable debug mode (don't send to Slack, print JSON)")
	flag.BoolVar(&install, "install", false, "Install the binary to /usr/bin/md2slack")
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
	if len(args) < 1 {
		fmt.Println("usage: md2slack [--debug] [--install] <MM-DD-YYYY> [extra context]")
		os.Exit(1)
	}

	dates := []string{args[0]}
	if strings.Contains(args[0], ",") {
		dates = strings.Split(args[0], ",")
	}

	extra := ""
	if len(args) > 1 {
		extra = strings.Join(args[1:], " ")
		// Clean terminal artifacts like bracketed paste markers
		re := regexp.MustCompile(`(?i)\x1b\[\d+~`)
		extra = re.ReplaceAllString(extra, "")
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	baseUrl := cfg.LLM.BaseURL
	if baseUrl == "" {
		baseUrl = "http://127.0.0.1:11434"
	}

	llmOpts := llm.LLMOptions{
		Provider:      cfg.LLM.Provider,
		ModelName:     cfg.LLM.Model,
		Temperature:   cfg.LLM.Temperature,
		TopP:          cfg.LLM.TopP,
		RepeatPenalty: cfg.LLM.RepeatPenalty,
		ContextSize:   cfg.LLM.ContextSize,
		BaseUrl:       baseUrl,
		Debug:         debug,
		Timeout:       2 * time.Minute,
		Heartbeat:     5 * time.Second,
	}

	repoName := gitdiff.GetRepoName()

	for _, date := range dates {
		date = strings.TrimSpace(date)
		fmt.Printf("\n--- Processing Date: %s (Repo: %s) ---\n", date, repoName)
		runStart := time.Now()
		var ui *tui.UI
		if !debug {
			ui = tui.Start([]string{
				"Preparing commit context",
				"Summarizing commits",
				"Generating tasks",
				"Reviewing tasks",
				"Suggesting next actions",
				"Rendering report",
			})
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

		output, err := gitdiff.GenerateFacts(date, extra)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating facts for %s: %v\n", date, err)
			continue
		}

		if debug {
			fmt.Println("--- Git Diff Facts ---")
			b, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(b))
		}

		stageStart := time.Now()
		if ui != nil {
			ui.StageStart(0, "")
		}
		allowedCommits := make(map[string]struct{})
		for _, c := range output.Commits {
			allowedCommits[c.Hash] = struct{}{}
		}
		if ui != nil {
			ui.StageDone(0, fmt.Sprintf("%d commits", len(output.Commits)))
		}
		logf("Stage 1 done in %s", time.Since(stageStart).Truncate(time.Millisecond))

		stageStart = time.Now()
		if ui != nil {
			ui.StageStart(1, "")
		}
		summaries, err := llm.SummarizeCommits(output.Commits, output.Diffs, output.Semantic, llmOpts)
		if err != nil {
			errf("Warning: failed to summarize commits: %v", err)
		}
		output.Summaries = summaries
		if ui != nil {
			ui.StageDone(1, fmt.Sprintf("%d summaries", len(summaries)))
		}
		logf("Stage 2 done in %s", time.Since(stageStart).Truncate(time.Millisecond))

		stageStart = time.Now()
		if ui != nil {
			ui.StageStart(2, "")
		}
		allTasks, err := llm.GenerateTasksFromContext(output.Commits, output.Summaries, output.Semantic, output.Extra, llmOpts, allowedCommits)
		if err != nil {
			errf("Warning: failed to generate tasks: %v", err)
		}
		if ui != nil {
			ui.StageDone(2, fmt.Sprintf("%d tasks", len(allTasks)))
		}
		logf("Stage 3 done in %s", time.Since(stageStart).Truncate(time.Millisecond))

		stageStart = time.Now()
		if ui != nil {
			ui.StageStart(3, "")
		}
		allTasks, err = llm.ReviewTasks(allTasks, output.Commits, output.Summaries, output.Semantic, output.Extra, llmOpts, allowedCommits)
		if err != nil {
			errf("Warning: failed to review tasks: %v", err)
		}
		if ui != nil {
			ui.StageDone(3, fmt.Sprintf("%d tasks", len(allTasks)))
		}
		logf("Stage 4 done in %s", time.Since(stageStart).Truncate(time.Millisecond))

		if len(allTasks) == 0 && len(output.Summaries) > 0 {
			errf("Warning: no tasks synthesized, falling back to summary-based tasks")
			allTasks = llm.FallbackTasksFromSummaries(output.Summaries)
		}

		if len(allTasks) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no tasks synthesized for %s\n", date)
			continue
		}

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
		logf("Stage 5 done in %s", time.Since(stageStart).Truncate(time.Millisecond))

		stageStart = time.Now()
		if ui != nil {
			ui.StageStart(5, "")
		}
		report := renderer.RenderReport(date, nil, allTasks, nextActions)
		if ui != nil {
			ui.StageDone(5, "ready")
			ui.Stop()
			ui = nil
		}
		logf("Stage 6 done in %s", time.Since(stageStart).Truncate(time.Millisecond))
		fmt.Println("\n--- FINAL REPORT ---")
		fmt.Println(report)

		// Save History for the NEXT run
		if err := storage.SaveHistory(repoName, date, allTasks, nil, output.Summaries); err != nil {
			errf("Warning: failed to save history for %s: %v", date, err)
		}
		if debug {
			fmt.Println("--- LLM Report ---")
			fmt.Println(report)
			fmt.Println("--- Slack Blocks ---")
			blocks, err := slack.ConvertToBlocks(report)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error converting to blocks: %v\n", err)
				continue
			}
			b, _ := json.MarshalIndent(blocks, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println("Sending to Slack...")
			if err := slack.SendMarkdown(&cfg.Slack, report); err != nil {
				fmt.Fprintf(os.Stderr, "Error sending to Slack for %s: %v\n", date, err)
				continue
			}
			fmt.Printf("Daily Status Report for %s sent successfully!\n", date)
		}
		logf("Total elapsed: %s", time.Since(runStart).Truncate(time.Millisecond))
	}
}
