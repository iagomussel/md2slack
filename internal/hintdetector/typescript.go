package hintdetector

import (
	"fmt"
	"regexp"
	"strings"
)

type TypeScriptDetector struct{}

var tsRegex = regexp.MustCompile(`(interface|type|enum|class)\s+([A-Za-z0-9_]+)`)

func (d TypeScriptDetector) Detect(line string, path string) (string, string, bool) {
	// Focusing on type definitions and type manipulations
	if strings.Contains(line, "interface ") ||
		strings.Contains(line, "type ") ||
		strings.Contains(line, "enum ") ||
		strings.Contains(line, " as ") ||
		(strings.Contains(line, ": ") && (strings.Contains(line, "string") || strings.Contains(line, "number"))) ||
		strings.Contains(line, "Omit<") {

		matches := tsRegex.FindStringSubmatch(line)
		if len(matches) > 2 {
			return "type_change", fmt.Sprintf("typescript definition/fix for %s", matches[2]), true
		}

		return "type_change", "typescript definition/fix", true
	}
	return "", "", false
}
