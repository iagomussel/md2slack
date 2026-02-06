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

func TestIndexHTMLHasSlackPreviewClass(t *testing.T) {
	if !strings.Contains(indexHTML, "slack-preview") {
		t.Fatal("expected slack-preview class")
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

func TestIndexHTMLHasTaskModal(t *testing.T) {
	if !strings.Contains(indexHTML, "id=\"task-modal\"") {
		t.Fatal("expected task modal container")
	}
}

func TestIndexHTMLHasActionsMenu(t *testing.T) {
	if !strings.Contains(indexHTML, "id=\"action-menu\"") {
		t.Fatal("expected actions dropdown")
	}
}
