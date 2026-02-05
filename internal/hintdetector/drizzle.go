package hintdetector

import "strings"

type DrizzleDetector struct{}

func (d DrizzleDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(line, "drizzle-orm") ||
		strings.Contains(line, "pgTable") ||
		strings.Contains(line, "mysqlTable") ||
		strings.Contains(line, "sqliteTable") ||
		strings.Contains(line, "db.select(") ||
		strings.Contains(line, "db.insert(") ||
		strings.Contains(line, "db.update(") ||
		(strings.Contains(line, "eq(") && strings.Contains(line, "and(")) { // rough heuristic for query builders
		return "framework_change", "drizzle orm query/schema", true
	}
	return "", "", false
}
