package storage

import (
	"database/sql"
	"encoding/json"
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
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			repo_name TEXT,
			date TEXT,
			data TEXT,
			report TEXT,
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
			task_json TEXT NOT NULL,
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

func SaveHistoryDB(repoName string, date string, report string) error {
	if err := initDB(); err != nil {
		return err
	}

	record := HistoryRecord{
		Date:   date,
		Report: report,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO history (repo_name, date, data, report)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(repo_name, date) DO UPDATE SET
			data = excluded.data,
			report = excluded.report;
	`, repoName, date, string(data), report)

	return err
}

func LoadHistoryDB(repoName string, date string) (*HistoryRecord, error) {
	if err := initDB(); err != nil {
		return nil, err
	}

	var data, report string
	err := db.QueryRow("SELECT data, report FROM history WHERE repo_name = ? AND date = ?", repoName, date).Scan(&data, &report)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var record HistoryRecord
	if err := json.Unmarshal([]byte(data), &record); err != nil {
		return nil, err
	}
	if record.Report == "" {
		record.Report = report
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
