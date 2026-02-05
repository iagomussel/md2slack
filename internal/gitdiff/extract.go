package gitdiff

import (
	"regexp"
	"strings"
)

func extractFacts(diff string) []CommitFacts {
	var results []CommitFacts

	commitRe := regexp.MustCompile(`^commit ([a-f0-9]{7,})`)
	lines := strings.Split(diff, "\n")

	var current *CommitFacts

	for _, l := range lines {
		if m := commitRe.FindStringSubmatch(l); m != nil {
			if current != nil {
				results = append(results, *current)
			}
			current = &CommitFacts{Hash: m[1]}
			continue
		}

		if current == nil {
			continue
		}

		// Heurísticas simples (expandível)
		switch {
			case strings.Contains(l, "test.describe"):
				current.Facts = append(current.Facts, "Added end-to-end test coverage")

			case strings.Contains(l, "timeout"):
				current.Facts = append(current.Facts, "Increased test execution timeout")

			case strings.Contains(l, "correctAnswer"):
				current.Facts = append(current.Facts, "Correct answer now derived from problem data")

			case strings.Contains(l, "placeholder"):
				current.Facts = append(current.Facts, "Removed placeholder-based logic")

			case strings.Contains(l, "try {") || strings.Contains(l, "catch"):
				current.Facts = append(current.Facts, "Added error handling to prevent runtime failures")
		}
	}

	if current != nil {
		results = append(results, *current)
	}

	return results
}
