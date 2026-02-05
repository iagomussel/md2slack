package hintdetector

import "strings"

type ORMMigrationDetector struct{}

func (d ORMMigrationDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(line, "drizzle") ||
		strings.Contains(line, "query.") ||
		strings.Contains(line, "update(") && strings.Contains(path, "service") {
		return "orm_migration", "migrated data access to ORM abstraction", true
	}
	return "", "", false
}
