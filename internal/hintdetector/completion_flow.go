package hintdetector

import "strings"

type CompletionFlowDetector struct{}

func (d CompletionFlowDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(line, "finish") &&
		(strings.Contains(line, "try") ||
			strings.Contains(line, "catch") ||
			strings.Contains(line, "finally")) {
		return "completion_flow", "hardened completion flow against failures", true
	}
	return "", "", false
}
