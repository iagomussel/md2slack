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

	fmt.Println("ReportProcessor: waiting for requests...")
	for req := range p.WebServer.RunChannel() {
		p.ProcessDate(req.Date, req.RepoPath, req.Author, "")
	}
}

// ProcessDate executes the full report generation pipeline for a specific date
func (p *ReportProcessor) ProcessDate(date string, repoPath string, authorOverride string, extraContext string) {
	ctx := &processCtx{
		date:           strings.TrimSpace(date),
		repoPath:       repoPath,
		authorOverride: authorOverride,
		extraContext:   extraContext,
		llmOpts:        p.LLMOpts,
	}

	if ctx.date == "" {
		ctx.date = time.Now().Format("01-02-2006")
	}

	ctx.repoName = gitdiff.GetRepoNameAt(ctx.repoPath)
	fmt.Printf("\n--- Date: %s (Repo: %s) autor %s ---\n", ctx.date, ctx.repoName, ctx.authorOverride)

	if p.WebServer != nil {
		p.WebServer.Reset(p.StageNames, ctx.date, ctx.repoName)
		ctx.ui = p.WebServer
		p.loadSessionFromHistory(ctx)
	}

	p.configureLLMOpts(ctx)

	// Execute stages
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

func (p *ReportProcessor) runStage(ctx *processCtx, stage int, logMsg string, action func(*processCtx) error) bool {
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
			if ctx.gitFacts != nil {
				status = fmt.Sprintf("%d commits found", len(ctx.gitFacts.Commits))
			}
		case 1:
			status = fmt.Sprintf("%d analyzed", len(ctx.commitChanges))
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
	type commitResult struct {
		index int
		cc    *gitdiff.CommitChange
		err   error
	}

	commits := ctx.gitFacts.Commits
	results := make(chan commitResult, len(commits))
	for i, commit := range commits {
		go func(idx int, c gitdiff.Commit) {
			var semantic gitdiff.CommitSemantic
			for _, s := range ctx.gitFacts.Semantic {
				if s.CommitHash == c.Hash {
					semantic = s
					break
				}
			}

			cc, err := llm.ExtractCommitIntent(gitdiff.SemanticChange{
				CommitHash: c.Hash,
				Signals:    semantic.Signals,
			}, c.Message, ctx.llmOpts)
			results <- commitResult{index: idx, cc: cc, err: err}
		}(i, commit)
	}

	ctx.commitChanges = make([]gitdiff.CommitChange, len(commits))
	for i := 0; i < len(commits); i++ {
		res := <-results
		if res.err != nil {
			p.errf(ctx, "Error analyzing commit: %v", res.err)
			continue
		}
		ctx.commitChanges[res.index] = *res.cc
	}
	return nil
}

func (p *ReportProcessor) stageGenerateTasks(ctx *processCtx) error {
	if p.WebServer != nil {
		ctx.allTasks = p.WebServer.GetTasks()
	}

	manualTasks, _ := llm.IncorporateExtraContext(ctx.gitFacts.Extra, ctx.llmOpts)

	for i, cc := range ctx.commitChanges {
		if cc.CommitHash == "" {
			continue
		}
		p.logf(ctx, "  [%d/%d] Incorporating commit %s...", i+1, len(ctx.commitChanges), cc.CommitHash)
		updated, err := llm.IncorporateCommit(cc, ctx.allTasks, manualTasks, ctx.gitFacts.Extra, ctx.llmOpts, ctx.allowedCommits)
		if err != nil {
			p.errf(ctx, "Error incorporating commit %s: %v", cc.CommitHash, err)
			continue
		}
		ctx.allTasks = updated
		if p.WebServer != nil {
			p.WebServer.SetTasks(ctx.allTasks, nil)
		}
	}

	ctx.allTasks = append(ctx.allTasks, manualTasks...)
	return nil
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

	fmt.Println("\n--- FINAL REPORT ---")
	fmt.Println(ctx.report)

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
		fmt.Println("Web UI enabled: report ready; use the Send button to post to Slack.")
	}
}

func (p *ReportProcessor) printDebugInfo(ctx *processCtx) {
	fmt.Println("--- LLM Report ---")
	fmt.Println(ctx.report)
	fmt.Println("--- Slack Blocks ---")
	blocks, err := slack.ConvertToBlocks(ctx.report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error converting to blocks: %v\n", err)
		return
	}
	b, _ := json.MarshalIndent(blocks, "", "  ")
	fmt.Println(string(b))
}

func (p *ReportProcessor) sendToSlack(ctx *processCtx) {
	fmt.Println("Sending to Slack...")
	if err := slack.SendMarkdown(&p.Config.Slack, ctx.report); err != nil {
		fmt.Fprintf(os.Stderr, "Error sending to Slack: %v\n", err)
		return
	}
	fmt.Printf("Daily Status Report for %s sent successfully!\n", ctx.date)
}

func (p *ReportProcessor) logf(ctx *processCtx, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if ctx.ui != nil {
		ctx.ui.Log(msg)
	} else {
		fmt.Println(msg)
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
