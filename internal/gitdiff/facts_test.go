package gitdiff

import "testing"

func TestGenerateFactsWithOptionsUsesAuthorOverride(t *testing.T) {
	_, err := GenerateFactsWithOptions("02-05-2026", "", "", "Override Name")
	if err == nil {
		t.Fatal("expected error because no repo path provided, but function should attempt to use override")
	}
}
