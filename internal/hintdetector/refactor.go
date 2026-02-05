package hintdetector

import "strings"

type RefactorDetector struct{}

func (d RefactorDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(line, "extract") ||
		strings.Contains(line, "refactor") ||
		strings.Contains(line, "helper") ||
		strings.Contains(line, "utils/") ||
		strings.Contains(line, "shared/") {
		return "refactor", "extracted reusable logic", true
	}
	return "", "", false
}
