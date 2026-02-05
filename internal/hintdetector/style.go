package hintdetector

import "strings"

type StyleAdjustmentDetector struct{}

func (d StyleAdjustmentDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(path, "css") ||
		strings.Contains(line, "font-weight") ||
		strings.Contains(line, "color:") {
		return "ui_style", "adjusted visual styling", true
	}
	return "", "", false
}
