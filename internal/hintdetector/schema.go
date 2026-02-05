package hintdetector

import "strings"

type SchemaDetector struct{}

func (d SchemaDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(path, "migration") ||
		strings.Contains(path, "schema") ||
		strings.Contains(line, "CREATE TABLE") ||
		strings.Contains(line, "ALTER TABLE") {
		return "schema_change", "data model update", true
	}
	return "", "", false
}
