package gitdiff

import (
	"strings"
)

// 1. Structures
type DiffFile struct {
	Path      string
	IsNew     bool
	IsDeleted bool
	IsTest    bool
	Additions []string
	Deletions []string
}

type SignalType string

const (
	SignalNewFile       SignalType = "new_file"
	SignalTestAdded     SignalType = "test_added"
	SignalTestModified  SignalType = "test_modified"
	SignalTimeoutChange SignalType = "timeout_change"
	SignalErrorHandling SignalType = "error_handling"
	SignalRouteChange   SignalType = "route_change"
	SignalSchemaChange  SignalType = "schema_change"
	SignalLogicChange   SignalType = "logic_change"
)

type Signal struct {
	Type SignalType
	File string
	Hint string
}

type SemanticChange struct {
	CommitHash   string
	Signals      []Signal
	FilesTouched int
	TouchesTests bool
}

// 2. Detection Logic
func detectTimeout(line string) bool {
	return strings.Contains(line, "timeout") &&
		(strings.Contains(line, "waitFor") || strings.Contains(line, "setTimeout"))
}

func detectErrorHandling(line string) bool {
	return strings.Contains(line, "try {") ||
		strings.Contains(line, "catch") ||
		strings.Contains(line, "if err !=") ||
		strings.Contains(line, "throw new Error")
}

func isTestFile(path string) bool {
	return strings.Contains(path, "/test") ||
		strings.Contains(path, ".spec.") ||
		strings.Contains(path, ".test.")
}

// 3. Signal Extractor
func detectSchemaChange(line string, path string) bool {
	return strings.Contains(path, "migration") ||
		strings.Contains(path, "schema") ||
		strings.Contains(line, "CREATE TABLE") ||
		strings.Contains(line, "ALTER TABLE")
}

func detectLogicChange(line string) bool {
	return strings.Contains(line, "for ") ||
		strings.Contains(line, "if ") ||
		strings.Contains(line, "return") ||
		strings.Contains(line, "else")
}

func detectUIState(line string) bool {
	return strings.Contains(line, "useState") ||
		strings.Contains(line, "useEffect") ||
		strings.Contains(line, "useRef") ||
		strings.Contains(line, "loading")
}

func ExtractSignals(file DiffFile) []Signal {
	var signals []Signal

	if file.IsNew {
		signals = append(signals, Signal{
			Type: SignalNewFile,
			File: file.Path,
		})
	}

	if file.IsTest {
		if file.IsNew {
			signals = append(signals, Signal{
				Type: SignalTestAdded,
				File: file.Path,
			})
		} else {
			signals = append(signals, Signal{
				Type: SignalTestModified,
				File: file.Path,
			})
		}
	}

	for _, line := range file.Additions {
		switch {
		case detectTimeout(line):
			signals = append(signals, Signal{
				Type: SignalTimeoutChange,
				File: file.Path,
				Hint: "test execution timing adjusted",
			})
		case detectErrorHandling(line):
			signals = append(signals, Signal{
				Type: SignalErrorHandling,
				File: file.Path,
				Hint: "guarded failure path",
			})
		case detectSchemaChange(line, file.Path):
			signals = append(signals, Signal{
				Type: SignalSchemaChange,
				File: file.Path,
				Hint: "data model update",
			})
		case detectUIState(line):
			signals = append(signals, Signal{
				Type: SignalLogicChange, // Using generic type for now or add SignalUIChange
				File: file.Path,
				Hint: "ui state management",
			})
		case detectLogicChange(line):
			signals = append(signals, Signal{
				Type: SignalLogicChange,
				File: file.Path,
				Hint: "flow control logic",
			})
		}
	}

	return signals
}

// 4. Change Grouper
func domainKey(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		return parts[len(parts)-2]
	}
	return path
}

func GroupSignals(commitHash string, signals []Signal) []SemanticChange {
	groups := map[string]*SemanticChange{}

	for _, s := range signals {
		key := domainKey(s.File)

		if _, ok := groups[key]; !ok {
			groups[key] = &SemanticChange{
				CommitHash: commitHash,
			}
		}

		group := groups[key]
		group.Signals = append(group.Signals, s)
		group.FilesTouched++ // This is an approximation, ideally we track unique files

		if s.Type == SignalTestAdded || s.Type == SignalTestModified {
			group.TouchesTests = true
		}
	}

	var out []SemanticChange
	for _, g := range groups {
		out = append(out, *g)
	}

	return out
}
