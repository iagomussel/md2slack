package hintdetector

import "strings"

type LogicDetector struct{}

func (d LogicDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(line, "for ") ||
		strings.Contains(line, "if ") ||
		strings.Contains(line, "return") ||
		strings.Contains(line, "else") {
		return "logic_change", "flow control logic", true
	}
	return "", "", false
}
