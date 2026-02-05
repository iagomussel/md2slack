package hintdetector

import (
	"fmt"
	"regexp"
	"strings"
)

type RefactorDetector struct{}

var refactorRegex = regexp.MustCompile(`(extract[A-Za-z0-9_]+|refactor[A-Za-z0-9_]+)`)

func (d RefactorDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(line, "extract") ||
		strings.Contains(line, "refactor") ||
		strings.Contains(line, "helper") ||
		strings.Contains(line, "utils/") ||
		strings.Contains(line, "shared/") {

		matches := refactorRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			return "refactor", fmt.Sprintf("extracted reusable logic in %s", matches[1]), true
		}

		return "refactor", "extracted reusable logic", true
	}
	return "", "", false
}
