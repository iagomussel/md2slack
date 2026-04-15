package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"md2slack/internal/config"
	"md2slack/internal/gitdiff"
	"md2slack/internal/jira"
	"md2slack/internal/llm"
	"md2slack/internal/renderer"
	"md2slack/internal/slack"
	"md2slack/internal/storage"
	"md2slack/internal/webui"
	"os"
	"strings"
	"time"
)

// UI defines the interaction with the user interface
type UI interface {
	StageStart(int, string)
	StageDone(int, string)
	Log(string)
	Error(string)
	Status(string)
	Stop()
}

// ReportProcessor handles the end-to-end report generation process
type ReportProcessor struct {
	Config     *config.Config
	LLMOpts    llm.LLMOptions
	WebServer  *webui.Server
	StageNames []string
	Debug      bool
}

// processCtx holds the state of a single report generation run
type processCtx struct {
	date           string
	repoPath       string
	repoName       string
	authorOverride string
	extraContext   string
	source         string // "git" or "jira"
	ui             UI
	llmOpts        llm.LLMOptions

	// Stage intermediate data
	gitFacts       *gitdiff.Output
	commitChanges  []gitdiff.CommitChange
	allTasks       []gitdiff.TaskChange
	nextActions    []string
	report         string
	allowedCommits map[string]struct{}
}

// Run starts the main loop, listening for requests from the web server
func (p *ReportProcessor) Run() {
	if p.WebServer == nil {
		fmt.Fprintln(os.Stderr, "Error: ReportProcessor started without a WebServer")
		return
	}

	log.Println("ReportProcessor: waiting for requests...")
	for req := range p.WebServer.RunChannel() {
		p.ProcessDate(req.Date, req.RepoPath, req.Author, "", req.Source)
	}
}

// ProcessDate executes the full report generation pipeline for a specific date.
// source is "git" (default) or "jira" for Jira-backed reports (see config [jira]).
func (p *ReportProcessor) ProcessDate(date string, repoPath string, authorOverride string, extraContext string, source string) {
	ctx := &processCtx{
		date:           strings.TrimSpace(date),
		repoPath:       repoPath,
		authorOverride: authorOverride,
		extraContext:   extraContext,
		source:         strings.ToLower(strings.TrimSpace(source)),
		llmOpts:        p.LLMOpts,
	}

	if ctx.source == "" {
		ctx.source = "git"
	}

	if ctx.date == "" {
		ctx.date = time.Now().Format("2006-01-02")
	}

	ctx.repoName = gitdiff.GetRepoNameAt(ctx.repoPath)
	if ctx.source == "jira" {
		ctx.repoName = config.JiraStorageKey(&p.Config.Jira)
		ctx.repoPath = ""
	}
	log.Printf("\n--- Date: %s (Repo: %s) source=%s autor %s ---\n", ctx.date, ctx.repoName, ctx.source, ctx.authorOverride)

	if p.WebServer != nil {
		p.WebServer.Reset(p.StageNames, ctx.date, ctx.repoName)
		p.WebServer.SetRepoDisplay(ctx.repoName)
		ctx.ui = p.WebServer
		p.loadSessionFromHistory(ctx)
	}

	p.configureLLMOpts(ctx)

	if ctx.source == "jira" {
		p.processJiraPipeline(ctx)
		return
	}

	// Execute stages (git)
	if !p.runStage(ctx, 0, "Preparing commit context", p.stagePrepareContext) {
		return
	}
	if !p.runStage(ctx, 1, "Summarizing commits", p.stageSummarizeCommits) {
		return
	}
	if !p.runStage(ctx, 2, "Generating tasks", p.stageGenerateTasks) {
		return
	}
	if !p.runStage(ctx, 3, "Reviewing tasks", p.stageReviewTasks) {
		return
	}
	if !p.runStage(ctx, 4, "Suggesting next actions", p.stageSuggestActions) {
		return
	}
	if !p.runStage(ctx, 5, "Rendering report", p.stageRenderReport) {
		return
	}

	p.finalizeReport(ctx)
}

func (p *ReportProcessor) processJiraPipeline(ctx *processCtx) {
	if err := p.Config.Jira.Validate(); err != nil {
		p.errf(ctx, "Jira is not configured: %v", err)
		return
	}

	if !p.runStage(ctx, 0, "Fetching Jira issues", func(ctx *processCtx) error {
		tasks, err := jira.DailyIssues(&p.Config.Jira, ctx.date)
		if err != nil {
			return err
		}
		ctx.allTasks = tasks
		ctx.gitFacts = &gitdiff.Output{
			RepoName: ctx.repoName,
			Date:     ctx.date,
			Commits:  nil,
		}
		ctx.allowedCommits = nil
		ctx.commitChanges = nil
		if p.WebServer != nil {
			p.WebServer.SetTasks(ctx.allTasks, nil)
		}
		if err := storage.ReplaceTasks(ctx.repoName, ctx.date, ctx.allTasks); err != nil {
			p.errf(ctx, "Warning: failed to persist tasks: %v", err)
		}
		return nil
	}) {
		return
	}

	if !p.runStage(ctx, 1, "Summarizing commits", func(*processCtx) error {
		ctx.commitChanges = nil
		return nil
	}) {
		return
	}
	if !p.runStage(ctx, 2, "Generating tasks", func(*processCtx) error {
		return nil
	}) {
		return
	}
	if !p.runStage(ctx, 3, "Reviewing tasks", p.stageReviewTasks) {
		return
	}
	if !p.runStage(ctx, 4, "Suggesting next actions", p.stageSuggestActions) {
		return
	}
	if !p.runStage(ctx, 5, "Rendering report", p.stageRenderReport) {
		return
	}
	p.finalizeReport(ctx)
}

func (p *ReportProcessor) runStage(ctx *processCtx, stage int, logMsg string, action func(*processCtx) error) bool {
	log.Printf("<=-------- Run Stage (%d) %s --------=>\n\n...\n", stage, logMsg)
	start := time.Now()
	if ctx.ui != nil {
		ctx.ui.StageStart(stage, "")
	}
	p.logf(ctx, "%s...", logMsg)

	err := action(ctx)
	if err != nil {
		p.errf(ctx, "Stage %d failed: %v", stage, err)
		return false
	}

	if ctx.ui != nil {
		status := ""
		switch stage {
		case 0:
			if ctx.source == "jira" {
				status = fmt.Sprintf("%d issues", len(ctx.allTasks))
			} else if ctx.gitFacts != nil {
				status = fmt.Sprintf("%d commits found", len(ctx.gitFacts.Commits))
			}
		case 1:
			if ctx.source == "jira" {
				status = "skipped (Jira)"
			} else {
				status = fmt.Sprintf("%d analyzed", len(ctx.commitChanges))
			}
		case 2:
			status = fmt.Sprintf("%d tasks", len(ctx.allTasks))
		case 3:
			status = "Refined"
		case 4:
			status = fmt.Sprintf("%d actions", len(ctx.nextActions))
		case 5:
			status = "ready"
		}
		ctx.ui.StageDone(stage, status)
	}

	p.logf(ctx, "Stage %d done in %s", stage, time.Since(start).Truncate(time.Millisecond))
	log.Printf("Stage %d done in %s", stage, time.Since(start).Truncate(time.Millisecond))
	return true
}

func (p *ReportProcessor) loadSessionFromHistory(ctx *processCtx) {
	hist, err := storage.LoadHistory(ctx.repoName, ctx.date)
	if err != nil || hist == nil {
		return
	}

	if hist.Message != "" {
		p.WebServer.SetReport(hist.Message)
		for i := 0; i < len(p.StageNames); i++ {
			p.WebServer.StageDone(i, "Loaded from history")
		}
	}

	if tasks, err := storage.LoadTasks(ctx.repoName, ctx.date); err == nil {
		p.WebServer.SetTasks(tasks, nil)
	}
}

func (p *ReportProcessor) configureLLMOpts(ctx *processCtx) {
	ctx.llmOpts.RepoName = ctx.repoName
	ctx.llmOpts.Date = ctx.date
	ctx.llmOpts.RepoPath = ctx.repoPath
	ctx.llmOpts.Quiet = ctx.ui != nil
	if ctx.ui != nil {
		ctx.llmOpts.OnToolLog = ctx.ui.Log
		ctx.llmOpts.OnToolStatus = ctx.ui.Status
		ctx.llmOpts.OnLLMLog = ctx.ui.Log
	}
}

func (p *ReportProcessor) stagePrepareContext(ctx *processCtx) error {
	var err error
	ctx.gitFacts, err = gitdiff.GenerateFactsWithOptions(ctx.date, ctx.extraContext, ctx.repoPath, ctx.authorOverride)
	if err != nil {
		return err
	}

	ctx.allowedCommits = make(map[string]struct{})
	for _, c := range ctx.gitFacts.Commits {
		ctx.allowedCommits[c.Hash] = struct{}{}
	}
	return nil
}

func (p *ReportProcessor) stageSummarizeCommits(ctx *processCtx) error {
	commits := ctx.gitFacts.Commits
	ctx.commitChanges = make([]gitdiff.CommitChange, len(commits))
	for i, commit := range commits {
		log.Printf("Analyzing commit %s...", commit.Hash)
		var semantic gitdiff.CommitSemantic
		for _, s := range ctx.gitFacts.Semantic {
			if s.CommitHash == commit.Hash {
				semantic = s
				break
			}
		}

		cc, err := llm.ExtractCommitIntent(gitdiff.SemanticChange{
			CommitHash: commit.Hash,
			Signals:    semantic.Signals,
		}, commit.Message, ctx.llmOpts)
		if err != nil {
			p.errf(ctx, "Error analyzing commit %s: %v", commit.Hash, err)
			continue
		}
		ctx.commitChanges[i] = *cc
		log.Printf("Commit %s analyzed: %+v", cc.CommitHash, ctx.commitChanges[i])
	}
	return nil
}

func (p *ReportProcessor) stageGenerateTasks(ctx *processCtx) error {
	log.Println("Generating tasks...")
	if p.WebServer != nil {
		ctx.allTasks = p.WebServer.GetTasks()
	}

	manualTasks, _ := llm.IncorporateExtraContext(ctx.gitFacts.Extra, ctx.llmOpts)

	for i, cc := range ctx.commitChanges {
		log.Printf("Incorporating commit %s...\n", cc.CommitHash)
		if cc.CommitHash == "" {
			continue
		}
		p.logf(ctx, "  [%d/%d] Incorporating commit %s...", i+1, len(ctx.commitChanges), cc.CommitHash)
		if err := p.processCommitLoop(ctx, cc, i, len(ctx.commitChanges), manualTasks); err != nil {
			p.errf(ctx, "Commit %s failed after retries: %v", cc.CommitHash, err)
			continue
		}
	}

	ctx.allTasks = append(ctx.allTasks, manualTasks...)
	return nil
}

const (
	commitRetryAttempts = 3
	commitRetryBackoff  = 300 * time.Millisecond
	defaultLLMTimeout   = 2 * time.Minute
)

func (p *ReportProcessor) processCommitLoop(ctx *processCtx, commit gitdiff.CommitChange, idx int, total int, manualTasks []gitdiff.TaskChange) error {
	timeout := ctx.llmOpts.Timeout
	if timeout <= 0 {
		timeout = defaultLLMTimeout
	}

	var lastErr error
	if p.WebServer != nil {
		p.WebServer.StartCommitRun(commit.CommitHash, idx, total)
	}
	for attempt := 1; attempt <= commitRetryAttempts; attempt++ {
		if ctx.ui != nil {
			ctx.ui.Status(fmt.Sprintf("Commit %d/%d — tentativa %d/%d", idx+1, total, attempt, commitRetryAttempts))
		}
		p.logf(ctx, "Commit %d/%d — tentativa %d/%d", idx+1, total, attempt, commitRetryAttempts)
		if p.WebServer != nil {
			p.WebServer.UpdateCommitAttempt(commit.CommitHash, attempt, commitRetryAttempts)
			p.WebServer.AppendCommitEvent(commit.CommitHash, "status", fmt.Sprintf("Attempt %d/%d started", attempt, commitRetryAttempts), attempt)
		}

		callCtx, cancel := context.WithTimeout(context.Background(), timeout)
		opts := ctx.llmOpts
		previousOnTasksUpdate := opts.OnTasksUpdate
		previousOnToolStart := opts.OnToolStart
		previousOnToolEnd := opts.OnToolEnd
		previousOnLLMMessage := opts.OnLLMMessage
		opts.OnTasksUpdate = func(tasks []gitdiff.TaskChange) {
			ctx.allTasks = tasks
			if previousOnTasksUpdate != nil {
				previousOnTasksUpdate(tasks)
			}
			if p.WebServer != nil {
				p.WebServer.SetTasks(tasks, nil)
			}
			if err := storage.ReplaceTasks(ctx.repoName, ctx.date, tasks); err != nil {
				p.errf(ctx, "Warning: failed to persist tasks: %v", err)
			}
		}
		opts.OnLLMMessage = func(role string, content string) {
			if previousOnLLMMessage != nil {
				previousOnLLMMessage(role, content)
			}
			if p.WebServer != nil {
				p.WebServer.AppendCommitEvent(commit.CommitHash, role, content, attempt)
			}
		}
		opts.OnToolStart = func(toolName string, paramsJSON string) {
			if previousOnToolStart != nil {
				previousOnToolStart(toolName, paramsJSON)
			}
			if p.WebServer != nil {
				p.WebServer.AppendCommitEvent(commit.CommitHash, "tool_start", fmt.Sprintf("%s %s", toolName, paramsJSON), attempt)
			}
		}
		opts.OnToolEnd = func(toolName string, resultJSON string) {
			if previousOnToolEnd != nil {
				previousOnToolEnd(toolName, resultJSON)
			}
			if p.WebServer != nil {
				p.WebServer.AppendCommitEvent(commit.CommitHash, "tool_end", fmt.Sprintf("%s %s", toolName, resultJSON), attempt)
			}
		}

		updated, err := llm.IncorporateCommitWithContext(callCtx, commit, ctx.allTasks, manualTasks, ctx.gitFacts.Extra, opts, ctx.allowedCommits)
		cancel()
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				if errors.Is(err, llm.ErrNoToolCalls) {
					p.errf(ctx, "Commit %d/%d — resposta sem tool calls na tentativa %d/%d", idx+1, total, attempt, commitRetryAttempts)
					if p.WebServer != nil {
						p.WebServer.AppendCommitEvent(commit.CommitHash, "error", "Response without tool calls", attempt)
					}
				} else {
					p.errf(ctx, "Commit %d/%d — timeout na tentativa %d/%d", idx+1, total, attempt, commitRetryAttempts)
					if p.WebServer != nil {
						p.WebServer.AppendCommitEvent(commit.CommitHash, "error", "Timeout", attempt)
					}
				}
				if attempt < commitRetryAttempts {
					time.Sleep(commitRetryBackoff)
					continue
				}
				if p.WebServer != nil {
					p.WebServer.FinishCommitRun(commit.CommitHash, "error", "Failed after retries")
				}
				return fmt.Errorf("commit %s falhou após %d tentativas (sem tool calls ou timeout)", commit.CommitHash, commitRetryAttempts)
			}
			if p.WebServer != nil {
				p.WebServer.FinishCommitRun(commit.CommitHash, "error", err.Error())
			}
			return fmt.Errorf("commit %s falhou: %w", commit.CommitHash, err)
		}

		ctx.allTasks = updated
		if p.WebServer != nil {
			p.WebServer.SetTasks(ctx.allTasks, nil)
			p.WebServer.AppendCommitEvent(commit.CommitHash, "status", fmt.Sprintf("Attempt %d/%d succeeded", attempt, commitRetryAttempts), attempt)
			p.WebServer.FinishCommitRun(commit.CommitHash, "success", "")
		}
		if err := storage.ReplaceTasks(ctx.repoName, ctx.date, ctx.allTasks); err != nil {
			p.errf(ctx, "Warning: failed to persist tasks: %v", err)
		}
		p.logf(ctx, "Commit %d/%d — tasks atualizadas (%d)", idx+1, total, len(ctx.allTasks))
		if ctx.ui != nil {
			ctx.ui.Status(fmt.Sprintf("Commit %d/%d — tasks atualizadas (%d)", idx+1, total, len(ctx.allTasks)))
		}
		return nil
	}

	if lastErr != nil {
		if p.WebServer != nil {
			p.WebServer.FinishCommitRun(commit.CommitHash, "error", lastErr.Error())
		}
		return fmt.Errorf("commit %s falhou: %w", commit.CommitHash, lastErr)
	}
	return nil
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded")
}

func isRetryableError(err error) bool {
	return isTimeoutError(err) || errors.Is(err, llm.ErrNoToolCalls) || isToolUseMismatchError(err)
}

func isToolUseMismatchError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tool_use_id") && strings.Contains(msg, "tool_result")
}

func (p *ReportProcessor) stageReviewTasks(ctx *processCtx) error {
	var err error
	ctx.allTasks, err = llm.ReviewTasks(ctx.allTasks, ctx.gitFacts.Commits, ctx.gitFacts.Summaries, ctx.gitFacts.Semantic, ctx.gitFacts.Extra, ctx.llmOpts, ctx.allowedCommits)
	if err == nil && p.WebServer != nil {
		p.WebServer.SetTasks(ctx.allTasks, nil)
	}
	return err
}

func (p *ReportProcessor) stageSuggestActions(ctx *processCtx) error {
	var err error
	ctx.nextActions, err = llm.SuggestNextActions(ctx.allTasks, ctx.llmOpts)
	return err
}

func (p *ReportProcessor) stageRenderReport(ctx *processCtx) error {
	ctx.report = renderer.RenderReport(ctx.date, nil, ctx.allTasks, ctx.nextActions)

	if err := storage.ReplaceTasks(ctx.repoName, ctx.date, ctx.allTasks); err != nil {
		p.errf(ctx, "Warning: failed to persist tasks: %v", err)
	} else if loaded, err := storage.LoadTasks(ctx.repoName, ctx.date); err == nil {
		ctx.allTasks = loaded
	}

	if p.WebServer != nil {
		p.WebServer.SetTasks(ctx.allTasks, ctx.nextActions)
		p.WebServer.SetReport(ctx.report)
	}
	return nil
}

func (p *ReportProcessor) finalizeReport(ctx *processCtx) {
	if ctx.ui != nil && p.WebServer == nil {
		ctx.ui.Stop()
	}

	log.Println("\n--- FINAL REPORT ---")
	log.Println(ctx.report)

	if err := storage.SaveHistory(ctx.repoName, ctx.date, ctx.report, "assistant"); err != nil {
		p.errf(ctx, "Warning: failed to save history: %v", err)
	}

	p.handleOutput(ctx)
}

func (p *ReportProcessor) handleOutput(ctx *processCtx) {
	if p.Debug {
		p.printDebugInfo(ctx)
	} else if p.WebServer == nil {
		p.sendToSlack(ctx)
	} else {
		log.Println("Web UI enabled: report ready; use the Send button to post to Slack.")
	}
}

func (p *ReportProcessor) printDebugInfo(ctx *processCtx) {
	log.Println("--- LLM Report ---")
	log.Println(ctx.report)
	log.Println("--- Slack Blocks ---")
	blocks, err := slack.ConvertToBlocks(ctx.report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error converting to blocks: %v\n", err)
		return
	}
	b, _ := json.MarshalIndent(blocks, "", "  ")
	log.Println(string(b))
}

func (p *ReportProcessor) sendToSlack(ctx *processCtx) {
	log.Println("Sending to Slack...")
	if err := slack.SendMarkdown(&p.Config.Slack, ctx.report); err != nil {
		fmt.Fprintf(os.Stderr, "Error sending to Slack: %v\n", err)
		return
	}
	log.Printf("Daily Status Report for %s sent successfully!\n", ctx.date)
}

func (p *ReportProcessor) logf(ctx *processCtx, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if ctx.ui != nil {
		ctx.ui.Log(msg)
	} else {
		log.Println(msg)
	}
}

func (p *ReportProcessor) errf(ctx *processCtx, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if ctx.ui != nil {
		ctx.ui.Error(msg)
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", msg)
	}
}
