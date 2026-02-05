package hintdetector

import "strings"

type StateGuardDetector struct{}

func (d StateGuardDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(line, "useRef") &&
		strings.Contains(line, "initialized") {
		return "state_guard", "prevented duplicate initialization", true
	}
	if strings.Contains(line, "if (") &&
		strings.Contains(line, "return;") &&
		strings.Contains(line, "already") {
		return "state_guard", "guarded repeated execution", true
	}
	return "", "", false
}
