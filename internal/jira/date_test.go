package jira

import "testing"

func TestNormalizeToISODate(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"2026-03-11", "2026-03-11"},
		{"03-11-2026", "2026-03-11"},
	}
	for _, tt := range tests {
		got, err := normalizeToISODate(tt.in)
		if err != nil || got != tt.want {
			t.Fatalf("normalizeToISODate(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}
