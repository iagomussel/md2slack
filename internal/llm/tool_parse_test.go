package llm

import (
	"encoding/json"
	"testing"
)

func TestNormalizeToolParams_CoerceIndexAndHours(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		params map[string]interface{}
		want   map[string]interface{}
	}{
		{
			name:   "add_time index string with embedded hours",
			tool:   "add_time",
			params: map[string]interface{}{"index": "0 estimated_hours=6 # comment"},
			want:   map[string]interface{}{"index": 0, "hours": 6},
		},
		{
			name:   "add_time index string only",
			tool:   "add_time",
			params: map[string]interface{}{"index": "0"},
			want:   map[string]interface{}{"index": 0},
		},
		{
			name:   "add_details index and details concatenated",
			tool:   "add_details",
			params: map[string]interface{}{"index": `0 "Added repository name output"`},
			want:   map[string]interface{}{"index": 0, "details": "Added repository name output"},
		},
		{
			name:   "add_commit_reference commit alias",
			tool:   "add_commit_reference",
			params: map[string]interface{}{"index": "0", "commit": "86e9451"},
			want:   map[string]interface{}{"index": 0, "hash": "86e9451"},
		},
		{
			name:   "add_time hours from estimated_hours",
			tool:   "add_time",
			params: map[string]interface{}{"index": 0, "estimated_hours": "8"},
			want:   map[string]interface{}{"index": 0, "hours": 8},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeToolParams(tt.tool, tt.params)
			for k, wantVal := range tt.want {
				g, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				switch w := wantVal.(type) {
				case int:
					if gi, ok := coerceInt(g); !ok || gi != w {
						t.Errorf("%q: want %d, got %v", k, w, g)
					}
				case string:
					if gs := castString(g); gs != w {
						t.Errorf("%q: want %q, got %q", k, w, gs)
					}
				}
			}
		})
	}
}

func TestNormalizedParamsMarshalToValidJSON(t *testing.T) {
	// Ensure normalized params produce JSON that tools can unmarshal (index/hours as numbers)
	params := map[string]interface{}{"index": "0 estimated_hours=6"}
	normalizeToolParams("add_time", params)
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		Index int `json:"index"`
		Hours int `json:"hours"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal (tool expects int): %v", err)
	}
	if decoded.Index != 0 || decoded.Hours != 6 {
		t.Errorf("want index=0 hours=6, got index=%d hours=%d", decoded.Index, decoded.Hours)
	}
}

// TestParseToolCallsFromText_LogLines parses log-style lines and verifies
// normalized params unmarshal into tool structs (no "cannot unmarshal string into int").
func TestParseToolCallsFromText_LogLines(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantTool string
		checkJSON func(t *testing.T, params map[string]interface{})
	}{
		{
			name:     "add_time with embedded hours",
			text:     "add_time index=0 estimated_hours=6 # Estimated time is around 5h based on complexity.",
			wantTool: "add_time",
			checkJSON: func(t *testing.T, params map[string]interface{}) {
				b, _ := json.Marshal(params)
				var st struct {
					Index int `json:"index"`
					Hours int `json:"hours"`
				}
				if err := json.Unmarshal(b, &st); err != nil {
					t.Fatalf("add_time tool unmarshal: %v", err)
				}
				if st.Index != 0 || st.Hours != 6 {
					t.Errorf("add_time: want index=0 hours=6, got index=%d hours=%d", st.Index, st.Hours)
				}
			},
		},
		{
			name:     "add_details index and details concatenated",
			text:     `add_details(index="0 \"Added repository name output in history storage module\"")`,
			wantTool: "add_details",
			checkJSON: func(t *testing.T, params map[string]interface{}) {
				b, _ := json.Marshal(params)
				var st struct {
					Index   int    `json:"index"`
					Details string `json:"details"`
				}
				if err := json.Unmarshal(b, &st); err != nil {
					t.Fatalf("add_details tool unmarshal: %v", err)
				}
				if st.Index != 0 {
					t.Errorf("add_details: want index=0, got %d", st.Index)
				}
				if st.Details == "" {
					t.Errorf("add_details: want non-empty details")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := parseToolCallsFromText(tt.text)
			if len(calls) != 1 {
				t.Fatalf("expected 1 tool call, got %d", len(calls))
			}
			if calls[0].Tool != tt.wantTool {
				t.Fatalf("expected tool %q, got %q", tt.wantTool, calls[0].Tool)
			}
			tt.checkJSON(t, calls[0].Parameters)
		})
	}
}
