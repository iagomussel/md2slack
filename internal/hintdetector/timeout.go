package hintdetector

import "strings"

type TimeoutDetector struct{}

func (d TimeoutDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(line, "timeout") &&
		(strings.Contains(line, "waitFor") || strings.Contains(line, "setTimeout")) {
		return "timeout_change", "test execution timing adjusted", true
	}
	return "", "", false
}
