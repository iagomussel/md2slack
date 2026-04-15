package storage

import (
	"strings"

	"md2slack/internal/gitdiff"
)

// RepoKey returns the SQLite repo_name key: git toplevel basename, or a literal jira-* namespace.
func RepoKey(repoPathOrKey string) string {
	s := strings.TrimSpace(repoPathOrKey)
	if s == "" {
		return "unknown"
	}
	if strings.HasPrefix(strings.ToLower(s), "jira-") {
		return s
	}
	return gitdiff.GetRepoNameAt(s)
}
