package hintdetector

import "strings"

type MigrationDetector struct{}

func (d MigrationDetector) Detect(line string, path string) (string, string, bool) {
	// Focusing on file paths mostly, or specific SQL content
	if (strings.Contains(path, "migration") || strings.Contains(path, "migrations")) &&
		(strings.HasSuffix(path, ".sql") || strings.HasSuffix(path, ".ts")) {
		return "migration", "database migration file", true
	}
	// Fallback for SQL statements often found in migrations
	if strings.Contains(line, "CREATE INDEX") ||
		strings.Contains(line, "DROP TABLE") ||
		strings.Contains(line, "ALTER COLUMN") {
		return "migration", "database schema migration", true
	}
	return "", "", false
}
