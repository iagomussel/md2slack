package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsServerAndLLMToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.ini")
	content := `
[server]
host=0.0.0.0
port=9999
auto_increment_port=false

[llm]
provider=anthropic
model=claude-3-5-sonnet-20241022
token=TEST_TOKEN
base_url=https://api.anthropic.com/v1/messages
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer os.Chdir(cwd)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Host != "0.0.0.0" || cfg.Server.Port != 9999 || cfg.Server.AutoIncrementPort != false {
		t.Fatalf("unexpected server config: %+v", cfg.Server)
	}
	if cfg.LLM.Token != "TEST_TOKEN" {
		t.Fatalf("expected LLM token")
	}
}

func TestLoadLeavesBaseURLBlankByDefault(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.ini")
	content := `
[llm]
provider=anthropic
model=claude-3-5-sonnet-20241022
token=TEST_TOKEN
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer os.Chdir(cwd)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.LLM.BaseURL != "" {
		t.Fatalf("expected empty base_url by default, got %q", cfg.LLM.BaseURL)
	}
}
