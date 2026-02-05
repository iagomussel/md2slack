package hintdetector

import (
	"fmt"
	"regexp"
	"strings"
)

type ExpressDetector struct{}

var expressRegex = regexp.MustCompile(`router\.(get|post|put|delete|patch|use)\s*\(["']([^"']+)`)

func (d ExpressDetector) Detect(line string, path string) (string, string, bool) {
	// Focusing on router/middleware patterns
	if strings.Contains(line, "express()") ||
		strings.Contains(line, "router.get(") ||
		strings.Contains(line, "router.post(") ||
		strings.Contains(line, "router.put(") ||
		strings.Contains(line, "router.delete(") ||
		strings.Contains(line, "app.use(") ||
		(strings.Contains(line, "req") && strings.Contains(line, "res") && strings.Contains(line, "next")) {

		matches := expressRegex.FindStringSubmatch(line)
		if len(matches) > 2 {
			return "framework_change", fmt.Sprintf("express %s route for %s", strings.ToUpper(matches[1]), matches[2]), true
		}

		return "framework_change", "express middleware/route", true
	}
	return "", "", false
}
