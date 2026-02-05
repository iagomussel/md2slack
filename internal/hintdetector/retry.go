package hintdetector

import "strings"

type RetryDetector struct{}

func (d RetryDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(line, "for (let attempt") ||
		strings.Contains(line, "retry") ||
		strings.Contains(line, "attempt <") {
		return "retry_logic", "added retry mechanism", true
	}
	return "", "", false
}
