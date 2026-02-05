package hintdetector

import "strings"

type TypeScriptDetector struct{}

func (d TypeScriptDetector) Detect(line string, path string) (string, string, bool) {
	// Focusing on type definitions and type manipulations
	if strings.Contains(line, "interface ") ||
		strings.Contains(line, "type ") ||
		strings.Contains(line, "enum ") ||
		strings.Contains(line, " as ") || // Careful with this one, might be noisy
		(strings.Contains(line, ": ") && (strings.Contains(line, "string") || strings.Contains(line, "number") || strings.Contains(line, "boolean") || strings.Contains(line, "Promise<"))) ||
		strings.Contains(line, "Check<") ||
		strings.Contains(line, "Pick<") ||
		strings.Contains(line, "Omit<") {
		return "type_change", "typescript definition/fix", true
	}
	return "", "", false
}
