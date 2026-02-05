package hintdetector

import "strings"

type UIDetector struct{}

func (d UIDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(line, "useState") ||
		strings.Contains(line, "useEffect") ||
		strings.Contains(line, "useRef") ||
		strings.Contains(line, "loading") {
		return "logic_change", "ui state management", true
	}
	return "", "", false
}
