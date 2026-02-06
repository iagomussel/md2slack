package storage

import (
	"md2slack/internal/gitdiff"
)

type HistoryRecord struct {
	Date      string                  `json:"date"`
	Tasks     []gitdiff.TaskChange    `json:"tasks"`
	Groups    []gitdiff.GroupedTask   `json:"groups,omitempty"`
	Summaries []gitdiff.CommitSummary `json:"summaries,omitempty"`
	Report    string                  `json:"report,omitempty"`
}

func SaveHistory(repoName string, date string, tasks []gitdiff.TaskChange, groups []gitdiff.GroupedTask, summaries []gitdiff.CommitSummary, report string) error {
	return SaveHistoryDB(repoName, date, tasks, groups, summaries, report)
}

func LoadHistory(repoName string, date string) (*HistoryRecord, error) {
	return LoadHistoryDB(repoName, date)
}
