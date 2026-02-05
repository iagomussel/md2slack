package gitdiff

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
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
	Changes  []SemanticChange `json:"changes"`
	Commits  []Commit         `json:"raw_commits,omitempty"`
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
	author, _ := sh(`git config user.name`)
	repo := GetRepoName()

	raw, err := sh(fmt.Sprintf(`
git log --author="%s" \
--since="%s 00:00:00" \
--until="%s 23:59:59" \
--no-merges \
--format="commit %%h%%n%%s" \
-p -U1 --all
`, author, date, date))
	if err != nil {
		return nil, err
	}

	// 3. Parse and Analyze
	commits := ParseGitLog(raw)
	var allChanges []SemanticChange

	for _, commit := range commits {
		var commitSignals []Signal
		for _, file := range commit.Files {
			commitSignals = append(commitSignals, ExtractSignals(file))
		}
		// Group signals per commit
		changes := GroupSignals(commit.Hash, commitSignals)
		allChanges = append(allChanges, changes...)
	}

	// 4. Return Output
	out := &Output{
		RepoName: repo,
		Date:     date,
		Author:   author,
		Extra:    extra,
		Changes:  allChanges,
		Commits:  commits,
	}

	return out, nil
}
