package hintdetector

import "strings"

type ErrorHandlingDetector struct{}

func (d ErrorHandlingDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(line, "try {") ||
		strings.Contains(line, "catch") ||
		strings.Contains(line, "if err !=") ||
		strings.Contains(line, "throw new Error") {
		return "error_handling", "guarded failure path", true
	}
	return "", "", false
}
