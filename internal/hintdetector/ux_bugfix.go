package hintdetector

import "strings"

type UXBugFixDetector struct{}

func (d UXBugFixDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(path, "client") &&
		(strings.Contains(line, "setDisabled") ||
			strings.Contains(line, "stopwatch") ||
			strings.Contains(line, "input")) {
		return "ux_bugfix", "fixed user-visible interaction issue", true
	}
	return "", "", false
}
