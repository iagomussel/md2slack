package webui

import (
	"strings"
	"testing"
)

func TestIndexHTMLDarkThemeTokens(t *testing.T) {
	if !strings.Contains(indexHTML, "--amber-500") {
		t.Fatal("expected amber token in indexHTML")
	}
}
