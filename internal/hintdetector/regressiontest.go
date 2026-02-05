package hintdetector

import "strings"

type RegressionTestDetector struct{}

func (d RegressionTestDetector) Detect(line, path string) (string, string, bool) {
	if strings.Contains(path, "test") &&
		strings.Contains(line, "expect") &&
		strings.Contains(line, "not.toBe") {
		return "regression_test", "added regression coverage", true
	}
	return "", "", false
}
