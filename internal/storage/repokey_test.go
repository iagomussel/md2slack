package storage

import "testing"

func TestRepoKey_JiraPrefixPassthrough(t *testing.T) {
	if got := RepoKey("jira-ABC"); got != "jira-ABC" {
		t.Fatalf("RepoKey = %q", got)
	}
}
