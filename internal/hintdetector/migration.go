package hintdetector

import (
	"fmt"
	"regexp"
	"strings"
)

type MigrationDetector struct{}

var migrationRegex = regexp.MustCompile(`(CREATE TABLE|DROP TABLE|ALTER COLUMN|CREATE INDEX)\s+([A-Za-z0-9_.]+)`)

func (d MigrationDetector) Detect(line, path string) (string, string, bool) {
	// Focusing on file paths mostly, or specific SQL content
	if (strings.Contains(path, "migration") || strings.Contains(path, "migrations")) &&
		(strings.HasSuffix(path, ".sql") || strings.HasSuffix(path, ".ts")) {
		return "migration", "database migration file", true
	}
	// Fallback for SQL statements often found in migrations
	if strings.Contains(line, "CREATE INDEX") ||
		strings.Contains(line, "DROP TABLE") ||
		strings.Contains(line, "ALTER COLUMN") {

		matches := migrationRegex.FindStringSubmatch(line)
		if len(matches) > 2 {
			return "migration", fmt.Sprintf("database schema migration (%s %s)", strings.ToLower(matches[1]), matches[2]), true
		}

		return "migration", "database schema migration", true
	}
	return "", "", false
}
