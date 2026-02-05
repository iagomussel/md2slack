package gitdiff

import (
	"md2slack/internal/hintdetector"
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
	File  string       `json:"file"`
	Types []SignalType `json:"types"`
	Hints []string     `json:"hints,omitempty"`
}

type SemanticChange struct {
	CommitHash   string
	Signals      []Signal
	FilesTouched int
	TouchesTests bool
}

// Pipeline Stage 1 Output
type CommitChange struct {
	CommitHash string   `json:"commit"`
	ChangeType string   `json:"change_type"`
	Intent     string   `json:"intent"`
	Scope      string   `json:"scope"`
	Signals    []string `json:"signals"`
	Confidence float64  `json:"confidence"`
}

// Pipeline Stage 2 Output
type TaskChange struct {
	TaskType       string   `json:"task_type"`
	TaskIntent     string   `json:"task_intent"`
	Scope          string   `json:"scope"`
	Commits        []string `json:"commits"`
	Confidence     float64  `json:"confidence"`
	EstimatedHours *int     `json:"estimated_hours,omitempty"`
	TechnicalWhy   string   `json:"technical_why,omitempty"`
}

// Pipeline Stage 3 Output
type GroupedTask struct {
	Epic       string  `json:"epic"`
	Tasks      []int   `json:"tasks"`
	Confidence float64 `json:"confidence"`
}

// 2. Detection Logic
func isTestFile(path string) bool {
	return strings.Contains(path, "/test") ||
		strings.Contains(path, ".spec.") ||
		strings.Contains(path, ".test.")
}

var detectors = []hintdetector.Detector{
	hintdetector.TimeoutDetector{},
	hintdetector.ErrorHandlingDetector{},
	hintdetector.SchemaDetector{},
	hintdetector.UIDetector{},
	hintdetector.LogicDetector{},
	hintdetector.NextJSDetector{},
	hintdetector.ExpressDetector{},
	hintdetector.DrizzleDetector{},
	hintdetector.MigrationDetector{},
	hintdetector.TypeScriptDetector{},
	hintdetector.AuthDetector{},
	hintdetector.RefactorDetector{},
	hintdetector.TestStabilityDetector{},
	hintdetector.RetryDetector{},
	hintdetector.StateGuardDetector{},
	hintdetector.UXBugFixDetector{},
	hintdetector.ORMMigrationDetector{},
	hintdetector.CompletionFlowDetector{},
	hintdetector.RegressionTestDetector{},
	hintdetector.StyleAdjustmentDetector{},
	hintdetector.StabilityGuardDetector{},
}

func ExtractSignals(file DiffFile) Signal {
	s := Signal{
		File: file.Path,
	}

	seenTypes := make(map[SignalType]bool)
	addType := func(t SignalType) {
		if !seenTypes[t] {
			s.Types = append(s.Types, t)
			seenTypes[t] = true
		}
	}

	seenHints := make(map[string]bool)
	addHint := func(h string) {
		if h != "" && !seenHints[h] {
			s.Hints = append(s.Hints, h)
			seenHints[h] = true
		}
	}

	if file.IsNew {
		addType(SignalNewFile)
	}

	if file.IsTest {
		if file.IsNew {
			addType(SignalTestAdded)
		} else {
			addType(SignalTestModified)
		}
	}

	for _, line := range file.Additions {
		for _, d := range detectors {
			sigType, hint, found := d.Detect(line, file.Path)
			if found {
				addType(SignalType(sigType))
				addHint(hint)
			}
		}
	}

	return s
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
		// Only include signals that have interesting types
		if len(s.Types) == 0 {
			continue
		}

		key := domainKey(s.File)

		if _, ok := groups[key]; !ok {
			groups[key] = &SemanticChange{
				CommitHash: commitHash,
			}
		}

		group := groups[key]
		group.Signals = append(group.Signals, s)
		group.FilesTouched++

		for _, t := range s.Types {
			if t == SignalTestAdded || t == SignalTestModified {
				group.TouchesTests = true
			}
		}
	}

	var out []SemanticChange
	for _, g := range groups {
		out = append(out, *g)
	}

	return out
}
