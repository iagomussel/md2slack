package hintdetector

import (
	"fmt"
	"regexp"
	"strings"
)

type UIDetector struct{}

var uiRegex = regexp.MustCompile(`(useState|useRef|useEffect)\s*\(?([^)]+)?`)

func (d UIDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(line, "useState") ||
		strings.Contains(line, "useEffect") ||
		strings.Contains(line, "useRef") ||
		strings.Contains(line, "loading") {

		matches := uiRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			return "logic_change", fmt.Sprintf("ui state management (%s)", matches[1]), true
		}

		return "logic_change", "ui state management", true
	}
	return "", "", false
}
