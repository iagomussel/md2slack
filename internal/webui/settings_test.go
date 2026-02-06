package webui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webui.json")

	s := Settings{
		ProjectPaths: []string{"/tmp/repo"},
		Usernames:    []string{"Iago"},
	}
	if err := saveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ProjectPaths) != 1 || loaded.ProjectPaths[0] != "/tmp/repo" {
		t.Fatalf("unexpected project paths: %#v", loaded.ProjectPaths)
	}
	if len(loaded.Usernames) != 1 || loaded.Usernames[0] != "Iago" {
		t.Fatalf("unexpected usernames: %#v", loaded.Usernames)
	}
}
