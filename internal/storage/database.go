package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db     *sql.DB
	dbOnce sync.Once
)

func initDB() error {
	var err error
	dbOnce.Do(func() {
		home, derr := os.UserHomeDir()
		if derr != nil {
			err = derr
			return
		}
		dbPath := os.Getenv("MD2SLACK_DB_PATH")
		if dbPath == "" {
			dir := filepath.Join(home, ".md2slack")
			if derr := os.MkdirAll(dir, 0755); derr != nil {
				err = derr
				return
			}
			dbPath = filepath.Join(dir, "md2slack.db")
		} else {
			if derr := os.MkdirAll(filepath.Dir(dbPath), 0755); derr != nil {
				err = derr
				return
			}
		}
		db, err = sql.Open("sqlite", dbPath)
		if err != nil {
			return
		}

		schema := `
		CREATE TABLE IF NOT EXISTS history (
			id TEXT PRIMARY KEY,
			repo_name TEXT,
			date TEXT,
			message TEXT,
			role TEXT,
			UNIQUE(repo_name, date)
		);`
		_, err = db.Exec(schema)
		if err != nil {
			return
		}
		taskSchema := `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			repo_name TEXT NOT NULL,
			date TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			status TEXT NOT NULL,
			intents TEXT NOT NULL,
			usernames TEXT NOT NULL,
			commits TEXT NOT NULL,
			scope TEXT NOT NULL,
			estimated_time INT NOT NULL,
			type TEXT NOT NULL,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_tasks_repo_date ON tasks(repo_name, date);`
		_, err = db.Exec(taskSchema)
	})
	return err
}

// ResetForTest clears the db connection and init guard (for tests only).
func ResetForTest() {
	if db != nil {
		_ = db.Close()
	}
	db = nil
	dbOnce = sync.Once{}
}

func SaveHistoryDB(repoName string, date string, message string, role string) error {
	if err := initDB(); err != nil {
		return err
	}

	_, err := db.Exec(`
		INSERT INTO history (id, repo_name, date, message, role)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(repo_name, date) DO UPDATE SET
			message = excluded.message,
			role = excluded.role;

	`, fmt.Sprintf("%s-%s", repoName, date), repoName, date, message, role)

	return err
}

func LoadHistoryDB(repoName string, date string) (*HistoryRecord, error) {
	if err := initDB(); err != nil {
		return nil, err
	}

	var message, role string
	err := db.QueryRow("SELECT message, role FROM history WHERE repo_name = ? AND date = ?", repoName, date).Scan(&message, &role)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	record := HistoryRecord{
		Date:    date,
		Message: message,
		Role:    role,
	}

	return &record, nil
}
func DeleteHistoryDB(repoName string, date string) error {
	if err := initDB(); err != nil {
		return err
	}
	_, err := db.Exec("DELETE FROM history WHERE repo_name = ? AND date = ?", repoName, date)
	return err
}
