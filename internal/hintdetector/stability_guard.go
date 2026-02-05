package hintdetector

import "strings"

type StabilityGuardDetector struct{}

func (d StabilityGuardDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(line, "catch (error") ||
		strings.Contains(line, "console.error") {
		return "stability_guard", "logged and isolated runtime failure", true
	}
	return "", "", false
}
