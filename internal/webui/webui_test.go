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

func TestIndexHTMLHasConsoleClasses(t *testing.T) {
	if !strings.Contains(indexHTML, "app-shell") {
		t.Fatal("expected app-shell class in indexHTML")
	}
}

func TestIndexHTMLUsesSSBranding(t *testing.T) {
	if !strings.Contains(indexHTML, "<title>ss Web UI</title>") {
		t.Fatal("expected ss Web UI title")
	}
	if !strings.Contains(indexHTML, "<h1>ss Web UI</h1>") {
		t.Fatal("expected ss Web UI header")
	}
}
