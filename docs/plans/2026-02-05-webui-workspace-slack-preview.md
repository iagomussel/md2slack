# WebUI Workspace Selector + Slack Preview Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Web UI controls for project selection and git username (with scan), make the date a datepicker, and render a larger Slack‑style preview; add Cypress E2E tests.

**Architecture:** Extend gitdiff to accept repo path and author override. Add Web UI endpoints to manage saved project paths and usernames (stored in a simple JSON file under ~/.md2slack), plus scanning usernames from git config/log. Update the embedded Web UI to surface selectors, a modal to add paths, a datepicker, and a larger Slack‑style preview. Add Cypress config + minimal E2E tests that exercise the new UI.

**Tech Stack:** Go net/http, embedded HTML/CSS/JS, git CLI, Cypress.

### Task 1: Add failing tests for new gitdiff options and webui settings storage

**Files:**
- Create: `internal/gitdiff/facts_test.go`
- Create: `internal/webui/settings_test.go`

**Step 1: Write failing test for gitdiff repo/author override**

```go
package gitdiff

import "testing"

func TestGenerateFactsWithOptionsUsesAuthorOverride(t *testing.T) {
    _, err := GenerateFactsWithOptions("02-05-2026", "", "", "Override Name")
    if err == nil {
        t.Fatal("expected error because no repo path provided, but function should attempt to use override")
    }
}
```

**Step 2: Write failing test for settings load/save**

```go
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
        Usernames: []string{"Iago"},
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
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/gitdiff -run TestGenerateFactsWithOptionsUsesAuthorOverride -v`
Expected: FAIL (missing function)

Run: `go test ./internal/webui -run TestSettingsLoadSave -v`
Expected: FAIL (missing Settings helpers)

**Step 4: Commit**

```bash
git add internal/gitdiff/facts_test.go internal/webui/settings_test.go
git commit -m "test: add webui settings and gitdiff option coverage"
```

### Task 2: Implement gitdiff options + settings storage + webui endpoints

**Files:**
- Modify: `internal/gitdiff/facts.go`
- Create: `internal/webui/settings.go`
- Modify: `internal/webui/webui.go`
- Modify: `cmd/md2slack/main.go`

**Step 1: Implement minimal code**

- Add `GenerateFactsWithOptions(date, extra, repoPath, authorOverride string)` and a `runGit(repoPath, cmd string)` helper that sets `Cmd.Dir`.
- Keep existing `GenerateFacts` by calling the new function with empty overrides.
- Add `Settings` struct with `ProjectPaths []string` and `Usernames []string`.
- Store settings in `~/.md2slack/webui.json` (or local `configPath` equivalent). Implement `loadSettings(path)` and `saveSettings(path, Settings)`.
- Add webui handlers:
  - `GET /settings` → return settings + derived project list with repo names
  - `POST /settings` → update settings
  - `POST /scan-users` (payload `{ path: "..." }`) → return usernames from `git config user.name` and recent `git log --format=%an` (unique)
- Update `/run` payload to include `repo_path` and `author`. Send these to `cmd/md2slack` via the run channel.
- Update `cmd/md2slack` to accept repo path/author override and call `GenerateFactsWithOptions`.

**Step 2: Run tests**

Run: `go test ./internal/gitdiff -run TestGenerateFactsWithOptionsUsesAuthorOverride -v`
Expected: PASS

Run: `go test ./internal/webui -run TestSettingsLoadSave -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/gitdiff/facts.go internal/webui/settings.go internal/webui/webui.go cmd/md2slack/main.go

git commit -m "feat: add webui settings and repo/author selection"
```

### Task 3: Update Web UI (date picker, selectors, Slack preview)

**Files:**
- Modify: `internal/webui/webui.go`

**Step 1: Implement minimal UI changes**

- Change date input to `type="date"` and add JS helpers to convert to/from `MM-DD-YYYY`.
- Add “Workspace” panel with:
  - project selector (display repo name + path)
  - “Add path” modal to store path in settings
  - username selector with “Scan” button (pulls from git config + git log)
- Make preview column wider (grid columns) and add `.slack-preview` styles for `#preview` (message card, typography, list styles, bullets, code).

**Step 2: Add small Go test for Slack preview class**

```go
func TestIndexHTMLHasSlackPreviewClass(t *testing.T) {
    if !strings.Contains(indexHTML, "slack-preview") {
        t.Fatal("expected slack-preview class")
    }
}
```

**Step 3: Run tests**

Run: `go test ./internal/webui -run TestIndexHTMLHasSlackPreviewClass -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/webui/webui.go internal/webui/webui_test.go

git commit -m "feat: enhance webui workspace controls and slack preview"
```

### Task 4: Add Cypress E2E tests

**Files:**
- Create: `package.json`
- Create: `cypress.config.ts`
- Create: `cypress/e2e/webui-workspace.cy.ts`

**Step 1: Write failing test**

```ts
it('allows adding a project path and selecting a user', () => {
  cy.visit('http://127.0.0.1:8080');
  cy.contains('Workspace');
});
```

**Step 2: Run and verify fail**

Run: `npx cypress run --spec cypress/e2e/webui-workspace.cy.ts`
Expected: FAIL (no server running / selectors missing)

**Step 3: Implement test wiring**

- Add Cypress config for baseUrl `http://127.0.0.1:8080`.
- Add selectors via data attributes in the Web UI where needed.
- Document how to start server before running tests.

**Step 4: Commit**

```bash
git add package.json cypress.config.ts cypress/e2e/webui-workspace.cy.ts

git commit -m "test: add cypress webui e2e"
```
