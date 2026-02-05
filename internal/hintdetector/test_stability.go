package hintdetector

import "strings"

type TestStabilityDetector struct{}

func (d TestStabilityDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(path, "test") &&
		(strings.Contains(line, "timeout") ||
			strings.Contains(line, "waitFor") ||
			strings.Contains(line, "retry") ||
			strings.Contains(line, "poll(")) {
		return "test_stability", "stabilized flaky test behavior", true
	}
	return "", "", false
}
