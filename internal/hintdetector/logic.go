package hintdetector

import (
	"fmt"
	"regexp"
	"strings"
)

type LogicDetector struct{}

var logicRegex = regexp.MustCompile(`(if|for|while)\s*\(([^)]+)\)`)

func (d LogicDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(line, "for ") ||
		strings.Contains(line, "if ") ||
		strings.Contains(line, "return") ||
		strings.Contains(line, "else") {

		matches := logicRegex.FindStringSubmatch(line)
		if len(matches) > 2 {
			condition := matches[2]
			if len(condition) > 20 {
				condition = condition[:20] + "..."
			}
			return "logic_change", fmt.Sprintf("flow control logic involving '%s'", condition), true
		}

		return "logic_change", "flow control logic", true
	}
	return "", "", false
}
