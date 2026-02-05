package hintdetector

import "strings"

type AuthDetector struct{}

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
		return "auth_change", "authentication/authorization logic", true
	}
	return "", "", false
}
