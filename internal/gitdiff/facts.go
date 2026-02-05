package gitdiff

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func sh(cmd string) (string, error) {
	c := exec.Command("bash", "-c", cmd)
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

type Output struct {
	RepoName string           `json:"repo_name"`
	Date     string           `json:"date"`
	Author   string           `json:"author"`
	Extra    string           `json:"extra_context,omitempty"`
	Commits  []Commit         `json:"raw_commits,omitempty"`
	Diffs    []CommitDiff     `json:"raw_diffs,omitempty"`
	Summaries []CommitSummary `json:"summaries,omitempty"`
	Semantic []CommitSemantic `json:"semantic,omitempty"`
}

func GetRepoName() string {
	repo, _ := sh(`basename $(git rev-parse --show-toplevel 2>/dev/null) 2>/dev/null`)
	if repo == "" {
		return "unknown"
	}
	return repo
}

func GenerateFacts(date string, extra string) (*Output, error) {
	// 1. Get user and repo info
	fullAuthor, _ := sh(`git config user.name`)
	repo := GetRepoName()

	// 2. Format date for Git (it strictly needs YYYY-MM-DD)
	isoDate := toISODate(date)

	// Broaden author search: use first part of name and case-insensitivity
	// This handles variations like "iago.mussel" vs "IagoMussel"
	authorPart := fullAuthor
	if idx := strings.Index(fullAuthor, " "); idx > 0 {
		authorPart = fullAuthor[:idx]
	}
	if idx := strings.Index(authorPart, "."); idx > 0 {
		authorPart = authorPart[:idx]
	}

	raw, err := sh(fmt.Sprintf(`
git log --author="%s" --regexp-ignore-case \
--since="%s 00:00:00" \
--until="%s 23:59:59" \
--no-merges \
--format="commit %%h%%n%%s" \
-p -U1 --all
`, authorPart, isoDate, isoDate))
	if err != nil {
		return nil, err
	}

	// 3. Parse and Analyze
	commits := ParseGitLog(raw)
	var diffs []CommitDiff
	var semantics []CommitSemantic

	for _, commit := range commits {
		diffText, err := sh(buildRawDiffCommand(commit.Hash))
		if err == nil {
			diffs = append(diffs, CommitDiff{CommitHash: commit.Hash, Diff: diffText})
		} else {
			diffs = append(diffs, CommitDiff{CommitHash: commit.Hash, Diff: ""})
		}

		// Parallelize signal extraction for files in this commit
		commitSignals := make([]Signal, len(commit.Files))
		var wg sync.WaitGroup
		for i, file := range commit.Files {
			wg.Add(1)
			go func(idx int, f DiffFile) {
				defer wg.Done()
				commitSignals[idx] = ExtractSignals(f)
			}(i, file)
		}
		wg.Wait()

		var signals []Signal
		touchesTests := false
		for i, s := range commitSignals {
			if commit.Files[i].IsTest {
				touchesTests = true
			}
			if len(s.Types) == 0 && len(s.Hints) == 0 {
				continue
			}
			signals = append(signals, s)
		}

		semantics = append(semantics, CommitSemantic{
			CommitHash:   commit.Hash,
			Signals:      signals,
			FilesTouched: len(commit.Files),
			TouchesTests: touchesTests,
		})
	}

	// 3. Return Output
	out := &Output{
		RepoName: repo,
		Date:     date,
		Author:   fullAuthor,
		Extra:    extra,
		Commits:  commits,
		Diffs:    diffs,
		Semantic: semantics,
	}

	return out, nil
}

func toISODate(date string) string {
	t, err := time.Parse("01-02-2006", date)
	if err == nil {
		return t.Format("2006-01-02")
	}
	return date // Return it as-is if parsing fails
}

func buildRawDiffCommand(hash string) string {
	excludes := []string{
		"node_modules", "dist", "build", "vendor", ".next", ".turbo",
		".cache", "coverage", "tmp", "tmp/*", ".git", ".idea", ".vscode",
	}
	var sb strings.Builder
	sb.WriteString("git show ")
	sb.WriteString(hash)
	sb.WriteString(" --format= --unified=0 --minimal --ignore-all-space --no-color -- .")
	for _, p := range excludes {
		sb.WriteString(" ':(exclude)")
		sb.WriteString(p)
		sb.WriteString("'")
	}
	return sb.String()
}
