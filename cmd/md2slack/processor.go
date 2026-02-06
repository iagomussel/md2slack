package main

import (
	"encoding/json"
	"fmt"
	"md2slack/internal/config"
	"md2slack/internal/gitdiff"
	"md2slack/internal/llm"
	"md2slack/internal/renderer"
	"md2slack/internal/slack"
	"md2slack/internal/storage"
	"md2slack/internal/tui"
	"md2slack/internal/webui"
	"os"
	"strings"
	"time"
)

type ReportProcessor struct {
	Config     *config.Config
	LLMOpts    llm.LLMOptions
	WebServer  *webui.Server
	Debug      bool
	StageNames []string
}

func (p *ReportProcessor) ProcessDate(date string, repoPath string, authorOverride string, extraContext string) {
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
	if p.WebServer != nil {
		p.WebServer.Reset(p.StageNames, date, repoName)
		ui = p.WebServer

		// Re-load previous session if it exists
		if hist, err := storage.LoadHistory(repoName, date); err == nil && hist != nil {
			p.WebServer.SetTasks(hist.Tasks, nil)
			if hist.Report != "" {
				p.WebServer.SetReport(hist.Report)
			}
			// Mark stages as done if we have a report (simple heuristic)
			if hist.Report != "" {
				for i := 0; i < len(p.StageNames); i++ {
					p.WebServer.StageDone(i, "Loaded from history")
				}
			}
		}
	} else if !p.Debug {
		ui = tui.Start(p.StageNames)
		defer func() {
			if ui != nil {
				ui.Stop()
			}
		}()
	}

	localLLMOpts := p.LLMOpts
	localLLMOpts.Quiet = ui != nil
	if ui != nil {
		localLLMOpts.OnToolLog = ui.Log
		localLLMOpts.OnToolStatus = ui.Status
		localLLMOpts.OnLLMLog = ui.Log
	} else {
		localLLMOpts.OnToolLog = nil
		localLLMOpts.OnToolStatus = nil
		localLLMOpts.OnLLMLog = nil
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
	output, err := gitdiff.GenerateFactsWithOptions(date, extraContext, repoPath, authorOverride)
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
			}, c.Message, localLLMOpts)
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

	manualTasks, _ := llm.IncorporateExtraContext(output.Extra, localLLMOpts)
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
		updated, err := llm.IncorporateCommit(cc, commitTasks, manualTasks, output.Extra, localLLMOpts, allowedCommits)
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
	allTasks, err = llm.RefineTasks(allTasks, localLLMOpts)
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
	nextActions, err := llm.SuggestNextActions(allTasks, localLLMOpts)
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
	if p.WebServer != nil {
		p.WebServer.SetTasks(allTasks, nextActions)
		p.WebServer.SetReport(report)
		p.WebServer.SetHandlers(
			func(report string) error {
				if p.Debug {
					logf("Debug: send requested; skipping Slack send")
					return nil
				}
				return slack.SendMarkdown(&p.Config.Slack, report)
			},
			func(prompt string, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
				return llm.RefineTasksWithPrompt(tasks, prompt, localLLMOpts)
			},
			func(date string, tasks []gitdiff.TaskChange, report string) error {
				return storage.SaveHistory(repoName, date, tasks, nil, nil, report)
			},
		)
		p.WebServer.SetActionHandler(
			func(action string, selected []int, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
				return llm.EditTasksWithAction(tasks, action, selected, localLLMOpts)
			},
			func(history []webui.OpenAIMessage, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, string, error) {
				var llmHistory []llm.OpenAIMessage
				for _, msg := range history {
					llmHistory = append(llmHistory, llm.OpenAIMessage{Role: msg.Role, Content: msg.Content})
				}
				updated, text, err := llm.ChatWithRequests(llmHistory, tasks, localLLMOpts, allowedCommits)
				return updated, text, err
			},
			func(index int, task gitdiff.TaskChange, tasks []gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
				if index < 0 || index >= len(tasks) {
					return tasks, fmt.Errorf("index out of bounds")
				}
				tasks[index] = task
				return tasks, nil
			},
		)
	}

	if ui != nil {
		ui.StageDone(5, "ready")
		if p.WebServer == nil {
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

	if p.Debug {
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
	} else if p.WebServer == nil {
		fmt.Println("Sending to Slack...")
		if err := slack.SendMarkdown(&p.Config.Slack, report); err != nil {
			fmt.Fprintf(os.Stderr, "Error sending to Slack for %s: %v\n", date, err)
			return
		}
		fmt.Printf("Daily Status Report for %s sent successfully!\n", date)
	} else {
		fmt.Println("Web UI enabled: report ready; use the Send button to post to Slack.")
	}
	logf("Total elapsed: %s", time.Since(runStart).Truncate(time.Millisecond))
}
