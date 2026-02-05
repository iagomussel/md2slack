# Anthropic Provider + Server Config Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add native Anthropic Messages API support and a configurable `[server]` section with auto-incrementing port fallback.

**Architecture:** Extend config parsing to include `LLM.token` and `Server` settings, pass them into runtime options, and add a provider branch for Anthropic in the LLM request pipeline. Implement a small port-selection helper in `cmd/md2slack` that probes for free ports when starting the web UI.

**Tech Stack:** Go, net/http, ini.v1 config, Anthropic Messages API.

### Task 1: Add failing config parsing tests

**Files:**
- Create: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
package config

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoadReadsServerAndLLMToken(t *testing.T) {
    dir := t.TempDir()
    cfgPath := filepath.Join(dir, "config.ini")
    content := ` + "`" + `
[server]
host=0.0.0.0
port=9999
auto_increment_port=false

[llm]
provider=anthropic
model=claude-3-5-sonnet-20241022
token=TEST_TOKEN
base_url=https://api.anthropic.com/v1/messages
` + "`" + `
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestLoadReadsServerAndLLMToken -v`
Expected: FAIL (missing fields).

**Step 3: Commit**

```bash
git add internal/config/config_test.go
git commit -m "test: cover server and llm token config"
```

### Task 2: Implement config parsing and server settings

**Files:**
- Modify: `internal/config/config.go`
- Modify: `cmd/md2slack/main.go`

**Step 1: Write minimal implementation**

Add to `LLMConfig`:
```go
Token string
```
Add `ServerConfig` struct and add to `Config`:
```go
type ServerConfig struct {
    Host string
    Port int
    AutoIncrementPort bool
}
```
Update `Load()` to parse:
- `[llm] token` default ""
- `[server] host` default "127.0.0.1"
- `[server] port` default 8080
- `[server] auto_increment_port` default true

Update `cmd/md2slack/main.go` to:
- pick `webAddr` from config unless flag is explicitly set (if flag value != default)
- add a `resolveWebAddr(host, port, autoIncrement)` helper that probes ports and returns the first free `host:port`

**Step 2: Run test to verify it passes**

Run: `go test ./internal/config -run TestLoadReadsServerAndLLMToken -v`
Expected: PASS.

**Step 3: Commit**

```bash
git add internal/config/config.go cmd/md2slack/main.go

git commit -m "feat: add server config and llm token"
```

### Task 3: Add Anthropic provider support

**Files:**
- Modify: `internal/llm/llm.go`
- Modify: `cmd/md2slack/main.go`

**Step 1: Write a failing test**

```go
package llm

import "testing"

func TestAnthropicProviderUsesMessagesEndpoint(t *testing.T) {
    opts := LLMOptions{Provider: "anthropic", BaseUrl: "https://api.anthropic.com/v1/messages", ModelName: "claude-3-5-sonnet-20241022"}
    url := resolveLLMURL(opts)
    if url != "https://api.anthropic.com/v1/messages" {
        t.Fatalf("unexpected url: %s", url)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/llm -run TestAnthropicProviderUsesMessagesEndpoint -v`
Expected: FAIL (missing helper).

**Step 3: Implement minimal code**

In `llm.go`:
- Add `Token` to `LLMOptions`.
- Add `AnthropicMessage`/request/response structs.
- Add `resolveLLMURL(options)` helper to choose URL based on provider.
- In `callJSON`, add a `case "anthropic"` branch:
  - URL default `https://api.anthropic.com/v1/messages`
  - Build payload with `system` string and `messages` array; map our `OpenAIMessage` list into Anthropic messages (`system` + `user/assistant` content).
  - Set `max_tokens` (use a safe default like 1024 or derived from context size).
  - Send headers: `x-api-key: <Token>`, `anthropic-version: 2023-06-01`, `content-type: application/json`.
  - Parse response text from `content[0].text`.
- Update `cmd/md2slack/main.go` to pass `cfg.LLM.Token` into `LLMOptions`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/llm -run TestAnthropicProviderUsesMessagesEndpoint -v`
Expected: PASS.

**Step 5: Run full tests**

Run: `go test ./...`
Expected: PASS.

**Step 6: Commit**

```bash
git add internal/llm/llm.go internal/llm/llm_test.go cmd/md2slack/main.go

git commit -m "feat: add anthropic provider"
```
