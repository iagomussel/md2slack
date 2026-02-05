package storage

import (
	"encoding/json"
	"fmt"
	"md2slack/internal/gitdiff"
	"os"
	"path/filepath"
)

type HistoryRecord struct {
	Date   string                `json:"date"`
	Tasks  []gitdiff.TaskChange  `json:"tasks"`
	Groups []gitdiff.GroupedTask `json:"groups,omitempty"`
}

func getHistoryDir(repoName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".md2slack", "history", repoName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func SaveHistory(repoName string, date string, tasks []gitdiff.TaskChange, groups []gitdiff.GroupedTask) error {
	dir, err := getHistoryDir(repoName)
	if err != nil {
		return err
	}

	record := HistoryRecord{
		Date:   date,
		Tasks:  tasks,
		Groups: groups,
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, fmt.Sprintf("%s.json", date))
	return os.WriteFile(path, data, 0644)
}

func LoadHistory(repoName string, date string) (*HistoryRecord, error) {
	dir, err := getHistoryDir(repoName)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, fmt.Sprintf("%s.json", date))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil // No history for this date
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var record HistoryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}

	return &record, nil
}
