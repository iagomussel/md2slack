# Darkmode Console UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan.

**Goal:** Redesign the embedded Web UI into a dark, “studio console” experience with amber neon accents and improved UX hierarchy.

**Architecture:** Replace Tailwind-heavy styling in the embedded `indexHTML` with a custom CSS system (variables + component classes) while preserving DOM structure/IDs. Update JS-rendered class names to match new styling and add small UI affordances. Add a minimal Go test to lock in key UI tokens.

**Tech Stack:** Go (net/http), embedded HTML/CSS/JS, goldmark markdown renderer.

### Task 1: Add a failing UI token test

**Files:**
- Create: `internal/webui/webui_test.go`

**Step 1: Write the failing test**

```go
package webui

import "testing"

func TestIndexHTMLDarkThemeTokens(t *testing.T) {
    if !strings.Contains(indexHTML, "--amber-500") {
        t.Fatal("expected amber token in indexHTML")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/webui -run TestIndexHTMLDarkThemeTokens -v`
Expected: FAIL with missing token.

**Step 3: Commit**

```bash
git add internal/webui/webui_test.go
git commit -m "test: add dark UI token check"
```

### Task 2: Implement dark console CSS system and update markup

**Files:**
- Modify: `internal/webui/webui.go`

**Step 1: Write minimal implementation**

Update `indexHTML`:
- Add `<link>` tags for display + body fonts (non-default).
- Add `<style>` block with CSS variables, base styles, layout grid, card styles, and components.
- Replace Tailwind classes in HTML with new semantic classes (keep IDs unchanged).
- Update JS-rendered className strings to new class names (task list rows, preview cards, status chips).
- Add subtle animations (staggered card reveal, running stage pulse) in CSS.

**Step 2: Run test to verify it passes**

Run: `go test ./internal/webui -run TestIndexHTMLDarkThemeTokens -v`
Expected: PASS.

**Step 3: Run full tests**

Run: `go test ./...`
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/webui/webui.go

git commit -m "feat: redesign web ui with dark console theme"
```

### Task 3: Refine UI tokens and add a second test guard

**Files:**
- Modify: `internal/webui/webui_test.go`

**Step 1: Write a second test**

```go
func TestIndexHTMLHasConsoleClasses(t *testing.T) {
    if !strings.Contains(indexHTML, "app-shell") {
        t.Fatal("expected app-shell class in indexHTML")
    }
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./internal/webui -run TestIndexHTMLHasConsoleClasses -v`
Expected: PASS.

**Step 3: Commit**

```bash
git add internal/webui/webui_test.go
git commit -m "test: lock console class names"
```
