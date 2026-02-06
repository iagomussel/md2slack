package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"md2slack/internal/gitdiff"
)

func LoadTasks(repoName string, date string) ([]gitdiff.TaskChange, error) {
	if err := initDB(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, task_json FROM tasks WHERE repo_name = ? AND date = ? ORDER BY created_at ASC`, repoName, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []gitdiff.TaskChange
	for rows.Next() {
		var id string
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var t gitdiff.TaskChange
		if err := json.Unmarshal([]byte(raw), &t); err != nil {
			// If JSON fails, skip or log? For now return error to be safe
			return nil, err
		}
		// Ensure ID matches DB ID
		if t.ID == "" {
			t.ID = id
		}
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
	// Generate ID if missing (simple random string for now or UUID if import allowed, sticking to pseudo-random based on time/intent to avoid imports if not needed, but uuid is better. User provided example "hasdasdlkqoi22sda")
	if task.ID == "" {
		task.ID = fmt.Sprintf("task-%d", time.Now().UnixNano()) // Simple fallback
	}

	raw, err := json.Marshal(task)
	if err != nil {
		return "", nil, err
	}
	_, err = db.Exec(`INSERT INTO tasks (id, repo_name, date, task_json) VALUES (?, ?, ?, ?)`, task.ID, repoName, date, string(raw))
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
	// Ensure task ID matches
	task.ID = taskID

	raw, err := json.Marshal(task)
	if err != nil {
		return nil, err
	}
	res, err := db.Exec(`UPDATE tasks SET task_json = ?, updated_at = CURRENT_TIMESTAMP WHERE repo_name = ? AND date = ? AND id = ?`, string(raw), repoName, date, taskID)
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
	stmt, err := tx.Prepare(`INSERT INTO tasks (id, repo_name, date, task_json) VALUES (?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, task := range tasks {
		// Ensure ID
		if task.ID == "" {
			task.ID = fmt.Sprintf("%s-%d", "task", time.Now().UnixNano()) // Simple ID generation
		}
		raw, mErr := json.Marshal(task)
		if mErr != nil {
			_ = tx.Rollback()
			return mErr
		}
		if _, err = stmt.Exec(task.ID, repoName, date, string(raw)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}
