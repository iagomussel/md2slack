package webui

import (
	"strings"
	"testing"
)

func TestIndexHTMLHasSvelteMarkers(t *testing.T) {
	if !strings.Contains(indexHTML, "data-sveltekit-preload-data") {
		t.Fatal("expected sveltekit markers in indexHTML")
	}
}

func TestIndexHTMLHasAppScripts(t *testing.T) {
	if !strings.Contains(indexHTML, "/_app/immutable/") {
		t.Fatal("expected svelte app scripts in indexHTML")
	}
}

func TestIndexHTMLHasRootDiv(t *testing.T) {
	if !strings.Contains(indexHTML, "display: contents") {
		t.Fatal("expected svelte contents div")
	}
}
