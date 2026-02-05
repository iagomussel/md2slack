package gitdiff

import (
	"encoding/json"
	"md2slack/internal/hintdetector"
	"strconv"
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

type CommitDiff struct {
	CommitHash string `json:"commit"`
	Diff       string `json:"diff"`
}

type CommitSummary struct {
	CommitHash string `json:"commit"`
	Summary    string `json:"summary"`
	Area       string `json:"area,omitempty"`
	Impact     string `json:"impact,omitempty"`
}

type CommitSemantic struct {
	CommitHash   string   `json:"commit"`
	Signals      []Signal `json:"signals,omitempty"`
	FilesTouched int      `json:"files_touched"`
	TouchesTests bool     `json:"touches_tests"`
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

func (c *CommitChange) UnmarshalJSON(data []byte) error {
	type rawCommitChange struct {
		CommitHash interface{} `json:"commit"`
		ChangeType interface{} `json:"change_type"`
		Intent     interface{} `json:"intent"`
		Scope      interface{} `json:"scope"`
		Signals    interface{} `json:"signals"`
		Confidence *float64    `json:"confidence"`
	}

	var raw rawCommitChange
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	c.CommitHash = castString(raw.CommitHash)
	c.ChangeType = castString(raw.ChangeType)
	c.Intent = castString(raw.Intent)
	c.Scope = castString(raw.Scope)
	c.Signals = castStringSlice(raw.Signals)
	if raw.Confidence != nil {
		c.Confidence = *raw.Confidence
	}

	return nil
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
	IsHistorical   bool     `json:"is_historical,omitempty"`
	IsManual       bool     `json:"is_manual,omitempty"`
}

func (t *TaskChange) UnmarshalJSON(data []byte) error {
	type rawTaskChange struct {
		TaskType       interface{} `json:"task_type"`
		TaskIntent     interface{} `json:"task_intent"`
		Scope          interface{} `json:"scope"`
		Commits        interface{} `json:"commits"`
		Confidence     *float64    `json:"confidence"`
		EstimatedHours interface{} `json:"estimated_hours"`
		TechnicalWhy   interface{} `json:"technical_why"`
	}

	var raw rawTaskChange
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	t.TaskType = castString(raw.TaskType)
	t.TaskIntent = castString(raw.TaskIntent)
	t.Scope = castString(raw.Scope)
	if raw.Confidence != nil {
		t.Confidence = *raw.Confidence
	}

	t.Commits = castStringSlice(raw.Commits)

	if raw.EstimatedHours != nil {
		if val, ok := castInt(raw.EstimatedHours); ok {
			t.EstimatedHours = &val
		}
	}

	switch v := raw.TechnicalWhy.(type) {
	case string:
		t.TechnicalWhy = strings.TrimSpace(v)
	case []interface{}:
		var lines []string
		for _, item := range v {
			if s := castString(item); s != "" {
				lines = append(lines, s)
			}
		}
		t.TechnicalWhy = strings.Join(lines, "\n")
	}

	return nil
}

// Pipeline Stage 3 Output
type GroupedTask struct {
	Epic       string  `json:"epic"`
	Tasks      []int   `json:"tasks"`
	Confidence float64 `json:"confidence"`
}

func (g *GroupedTask) UnmarshalJSON(data []byte) error {
	type rawGroupedTask struct {
		Epic       interface{} `json:"epic"`
		Tasks      interface{} `json:"tasks"`
		Confidence *float64    `json:"confidence"`
	}

	var raw rawGroupedTask
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	g.Epic = castString(raw.Epic)
	if raw.Confidence != nil {
		g.Confidence = *raw.Confidence
	}

	g.Tasks = castIntSlice(raw.Tasks)

	return nil
}

func castString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []interface{}:
		if len(val) > 0 {
			return castString(val[0])
		}
	case map[string]interface{}:
		for _, v := range val {
			if s := castString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func castStringSlice(v interface{}) []string {
	var out []string
	switch val := v.(type) {
	case []interface{}:
		for _, item := range val {
			if s := castString(item); s != "" {
				out = append(out, s)
			}
		}
	case string:
		if s := strings.TrimSpace(val); s != "" {
			out = append(out, s)
		}
	case map[string]interface{}:
		for _, v := range val {
			if s := castString(v); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func castInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case float64:
		return int(val), true
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return i, true
		}
	case []interface{}:
		if len(val) > 0 {
			return castInt(val[0])
		}
	case map[string]interface{}:
		for _, v := range val {
			if i, ok := castInt(v); ok {
				return i, true
			}
		}
	}
	return 0, false
}

func castIntSlice(v interface{}) []int {
	var out []int
	switch val := v.(type) {
	case []interface{}:
		for _, item := range val {
			if i, ok := castInt(item); ok {
				out = append(out, i)
			}
		}
	case float64:
		out = append(out, int(val))
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			out = append(out, i)
		}
	case map[string]interface{}:
		for _, v := range val {
			if i, ok := castInt(v); ok {
				out = append(out, i)
			}
		}
	}
	return out
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
