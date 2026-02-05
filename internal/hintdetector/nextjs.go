package hintdetector

import "strings"

type NextJSDetector struct{}

func (d NextJSDetector) Detect(line string, path string) (string, string, bool) {
	if strings.Contains(line, "NextResponse") ||
		strings.Contains(line, "NextRequest") ||
		strings.Contains(line, "getServerSideProps") ||
		strings.Contains(line, "getStaticProps") ||
		strings.Contains(line, "useRouter") ||
		strings.Contains(line, "usePathname") ||
		strings.Contains(line, "useSearchParams") ||
		(strings.Contains(path, "/app/") && (strings.Contains(path, "page.tsx") || strings.Contains(path, "layout.tsx"))) {
		return "framework_change", "nextjs routing/data flow", true
	}
	return "", "", false
}
