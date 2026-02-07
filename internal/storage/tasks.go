package storage

import (
	"fmt"
	"strings"
	"time"

	"md2slack/internal/gitdiff"
)

func LoadTasks(repoName string, date string) ([]gitdiff.TaskChange, error) {
	if err := initDB(); err != nil {
		return nil, err
	}
	// Select all columns
	rows, err := db.Query(`
		SELECT id, repo_name, date, title, details, status, intents, usernames, commits, scope, estimated_time, type 
		FROM tasks 
		WHERE repo_name = ? AND date = ? 
		ORDER BY created_at ASC`, repoName, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []gitdiff.TaskChange
	for rows.Next() {
		var (
			id, repo, d, title, desc, status, intent, users, commits, scope, taskType string
			estTime                                                                   int
		)
		if err := rows.Scan(&id, &repo, &d, &title, &desc, &status, &intent, &users, &commits, &scope, &estTime, &taskType); err != nil {
			return nil, err
		}

		t := gitdiff.TaskChange{
			ID:             id,
			RepoName:       repo,
			Date:           d,
			Title:          title,
			Details:        desc,
			Status:         status,
			TaskIntent:     intent,
			Usernames:      splitComma(users),
			Commits:        splitComma(commits),
			Scope:          scope,
			EstimatedHours: float64(estTime),
			TaskType:       taskType,
		}
		// Legacy field mapping
		t.Intent = t.TaskIntent

		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func CreateTask(repoName string, date string, task gitdiff.TaskChange) (string, []gitdiff.TaskChange, error) {
	if err := initDB(); err != nil {
		return "", nil, err
	}
	if task.ID == "" {
		task.ID = fmt.Sprintf("task-%d", time.Now().UnixNano())
	}

	_, err := db.Exec(`
		INSERT INTO tasks (
			id, repo_name, date, title, details, status, intents, usernames, commits, scope, estimated_time, type
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, repoName, date,
		task.Title, task.Details, task.Status, task.TaskIntent,
		joinComma(task.Usernames), joinComma(task.Commits), task.Scope, int(task.EstimatedHours), task.TaskType,
	)
	if err != nil {
		return "", nil, err
	}

	updated, err := LoadTasks(repoName, date)
	return task.ID, updated, err
}

func UpdateTask(repoName string, date string, taskID string, task gitdiff.TaskChange) ([]gitdiff.TaskChange, error) {
	if err := initDB(); err != nil {
		return nil, err
	}
	task.ID = taskID

	res, err := db.Exec(`
		UPDATE tasks SET 
			title = ?, details = ?, status = ?, intents = ?, usernames = ?, commits = ?, scope = ?, estimated_time = ?, type = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE repo_name = ? AND date = ? AND id = ?`,
		task.Title, task.Details, task.Status, task.TaskIntent,
		joinComma(task.Usernames), joinComma(task.Commits), task.Scope, int(task.EstimatedHours), task.TaskType,
		repoName, date, taskID,
	)
	if err != nil {
		return nil, err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return nil, fmt.Errorf("task_id %s not found", taskID)
	}
	return LoadTasks(repoName, date)
}

func DeleteTasks(repoName string, date string, ids []string) ([]gitdiff.TaskChange, error) {
	if err := initDB(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return LoadTasks(repoName, date)
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+2)
	args = append(args, repoName, date)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`DELETE FROM tasks WHERE repo_name = ? AND date = ? AND id IN (%s)`, strings.Join(placeholders, ","))
	_, err := db.Exec(query, args...)
	if err != nil {
		return nil, err
	}
	return LoadTasks(repoName, date)
}

func DeleteAllTasks(repoName string, date string) error {
	if err := initDB(); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM tasks WHERE repo_name = ? AND date = ?`, repoName, date)
	return err
}

func ReplaceTasks(repoName string, date string, tasks []gitdiff.TaskChange) error {
	if err := initDB(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM tasks WHERE repo_name = ? AND date = ?`, repoName, date); err != nil {
		_ = tx.Rollback()
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO tasks (
			id, repo_name, date, title, Details, status, intents, usernames, commits, scope, estimated_time, type
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, task := range tasks {
		if task.ID == "" {
			task.ID = fmt.Sprintf("%s-%d", "task", time.Now().UnixNano())
		}
		if _, err = stmt.Exec(
			task.ID, repoName, date,
			task.Title, task.Details, task.Status, task.TaskIntent,
			joinComma(task.Usernames), joinComma(task.Commits), task.Scope, int(task.EstimatedHours), task.TaskType,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// Helpers
func splitComma(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

func joinComma(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.Join(s, ",")
}
