package hintdetector

import (
	"fmt"
	"regexp"
	"strings"
)

type AuthDetector struct{}

var authRegex = regexp.MustCompile(`(?i)(clerk|nextauth|headers|authorization|jwt|session)`)

func (d AuthDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(line, "NextAuth") ||
		strings.Contains(line, "getServerSession") ||
		strings.Contains(line, "useSession") ||
		strings.Contains(line, "authorize") ||
		strings.Contains(line, "credentials") ||
		strings.Contains(line, "jwt") ||
		strings.Contains(line, "JWT") ||
		strings.Contains(line, "Bearer") ||
		strings.Contains(line, "clerk") ||
		strings.Contains(line, "Clerk") ||
		(strings.Contains(line, "headers") && strings.Contains(line, "authorization")) {

		matches := authRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			return "auth_change", fmt.Sprintf("authentication/authorization logic (%s)", strings.ToLower(matches[1])), true
		}

		return "auth_change", "authentication/authorization logic", true
	}
	return "", "", false
}
