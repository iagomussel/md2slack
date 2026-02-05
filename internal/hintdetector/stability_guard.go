package hintdetector

import (
	"fmt"
	"regexp"
	"strings"
)

type StabilityGuardDetector struct{}

var errorRegex = regexp.MustCompile(`catch\s*\(([^)]+)\)|console\.error\s*\(([^)]+)\)`)

func (d StabilityGuardDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(line, "catch (error") ||
		strings.Contains(line, "console.error") {

		matches := errorRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			errVar := matches[1]
			if errVar == "" {
				errVar = matches[2]
			}
			if errVar != "" {
				return "stability_guard", fmt.Sprintf("logged and isolated runtime failure (handled %s)", errVar), true
			}
		}

		return "stability_guard", "logged and isolated runtime failure", true
	}
	return "", "", false
}
