package hintdetector

import "strings"

type ExpressDetector struct{}

func (d ExpressDetector) Detect(line string, path string) (string, string, bool) {
	// Focusing on router/middleware patterns
	if strings.Contains(line, "express()") ||
		strings.Contains(line, "router.get(") ||
		strings.Contains(line, "router.post(") ||
		strings.Contains(line, "router.put(") ||
		strings.Contains(line, "router.delete(") ||
		strings.Contains(line, "app.use(") ||
		(strings.Contains(line, "req") && strings.Contains(line, "res") && strings.Contains(line, "next")) {
		return "framework_change", "express middleware/route", true
	}
	return "", "", false
}
