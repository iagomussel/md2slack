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
	}

	repoName := gitdiff.GetRepoName()

	for _, date := range dates {
		date = strings.TrimSpace(date)
		fmt.Printf("\n--- Processing Date: %s (Repo: %s) ---\n", date, repoName)

		// 1. Get Previous Day's History
		var prevTasks []gitdiff.TaskChange
		t, err := time.Parse("01-02-2006", date)
		if err == nil {
			prevDate := t.AddDate(0, 0, -1).Format("01-02-2006")
			hist, _ := storage.LoadHistory(repoName, prevDate)
			if hist != nil {
				fmt.Printf("Loaded history from %s/%s (%d tasks)\n", repoName, prevDate, len(hist.Tasks))
				for _, t := range hist.Tasks {
					t.IsHistorical = true
					prevTasks = append(prevTasks, t)
				}
			}
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

		fmt.Printf("Stage 1: Extracting commit intents for %d changes...\n", len(output.Changes))
		var commitChanges []gitdiff.CommitChange
		commitMap := make(map[string]gitdiff.Commit)
		for _, c := range output.Commits {
			commitMap[c.Hash] = c
		}

		for i, change := range output.Changes {
			fmt.Printf("  [%d/%d] Analyzing %s...\n", i+1, len(output.Changes), change.CommitHash)
			commit, ok := commitMap[change.CommitHash]
			msg := ""
			if ok {
				msg = commit.Message
			}
			cc, err := llm.ExtractCommitIntent(change, msg, llmOpts)
			if err != nil {
				fmt.Printf("Warning: failed to extract intent for %s: %v\n", change.CommitHash, err)
				continue
			}
			cc.CommitHash = change.CommitHash // Preserve factual hash
			commitChanges = append(commitChanges, *cc)
		}

		fmt.Printf("Stage 2: Synthesizing tasks from %d analyzed commits (incremental)...\n", len(commitChanges))
		var currentTasks []gitdiff.TaskChange
		currentTasks = append(currentTasks, prevTasks...) // Start with previous tasks for continuity

		for i, cc := range commitChanges {
			fmt.Printf("  [%d/%d] Incorporating commit %s...\n", i+1, len(commitChanges), cc.CommitHash)
			updatedTasks, err := llm.IncorporateCommit(cc, currentTasks, output.Extra, llmOpts)
			if err != nil {
				fmt.Printf("Warning: failed to incorporate commit %s: %v\n", cc.CommitHash, err)
				continue
			}
			currentTasks = updatedTasks
		}

		if len(currentTasks) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no tasks synthesized for %s\n", date)
			continue
		}

		fmt.Printf("\nStage 2.5: Refining and Deduplicating tasks...\n")
		currentTasks = llm.PruneTasks(currentTasks)
		currentTasks, _ = llm.RefineTasks(currentTasks, llmOpts)

		fmt.Printf("Stage 3: Grouping %d synthesized tasks into Epics...\n", len(currentTasks))
		groups, err := llm.GroupTasks(currentTasks, llmOpts)
		if err != nil {
			fmt.Printf("Warning: failed to group tasks for %s: %v\n", date, err)
		}

		// Save History for the NEXT run
		if err := storage.SaveHistory(repoName, date, currentTasks, groups); err != nil {
			fmt.Printf("Warning: failed to save history for %s: %v\n", date, err)
		}

		fmt.Println("Stage 4: Rendering report and preparing Slack blocks...")
		report := renderer.RenderReport(date, groups, currentTasks)
		fmt.Println("\n--- FINAL REPORT ---")
		fmt.Println(report)

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
	}
}
