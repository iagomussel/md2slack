package hintdetector

import (
	"fmt"
	"regexp"
	"strings"
)

type TestStabilityDetector struct{}

var waitRegex = regexp.MustCompile(`(waitFor|waitUntil)\(([^)]+)`)

func (d TestStabilityDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(path, "test") &&
		(strings.Contains(line, "timeout") ||
			strings.Contains(line, "waitFor") ||
			strings.Contains(line, "retry") ||
			strings.Contains(line, "poll(")) {

		matches := waitRegex.FindStringSubmatch(line)
		if len(matches) > 2 {
			target := matches[2]
			if len(target) > 20 {
				target = target[:20] + "..."
			}
			return "test_stability", fmt.Sprintf("stabilized flaky test behavior (waiting for %s)", target), true
		}

		return "test_stability", "stabilized flaky test behavior", true
	}
	return "", "", false
}
