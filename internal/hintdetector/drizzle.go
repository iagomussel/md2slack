package hintdetector

import (
	"fmt"
	"regexp"
	"strings"
)

type DrizzleDetector struct{}

var drizzleRegex = regexp.MustCompile(`(pgTable|mysqlTable|sqliteTable|select|insert|update|delete)\s*\(?["']?([A-Za-z0-9_]+)?`)

func (d DrizzleDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(line, "drizzle-orm") ||
		strings.Contains(line, "pgTable") ||
		strings.Contains(line, "mysqlTable") ||
		strings.Contains(line, "sqliteTable") ||
		strings.Contains(line, "db.select(") ||
		strings.Contains(line, "db.insert(") ||
		strings.Contains(line, "db.update(") ||
		(strings.Contains(line, "eq(") && strings.Contains(line, "and(")) { // rough heuristic for query builders

		matches := drizzleRegex.FindStringSubmatch(line)
		if len(matches) > 2 && matches[2] != "" {
			return "framework_change", fmt.Sprintf("drizzle orm %s for %s", matches[1], matches[2]), true
		} else if len(matches) > 1 {
			return "framework_change", fmt.Sprintf("drizzle orm %s", matches[1]), true
		}

		return "framework_change", "drizzle orm query/schema", true
	}
	return "", "", false
}
