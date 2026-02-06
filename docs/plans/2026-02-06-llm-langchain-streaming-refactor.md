# LLM LangChain Streaming Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan.

**Goal:** Remove all non-streaming LLM paths and refactor LLM usage to only LangChain with streaming.

**Architecture:** Keep a single streaming chat pipeline built on LangChain (langchaingo) that handles tool calls and emits stream chunks. Remove non-streaming APIs and any unused provider-specific logic not needed for the streaming flow.

**Tech Stack:** Go, langchaingo, existing md2slack internal packages.

### Task 1: Inventory and delete non-streaming LLM entrypoints

**Files:**
- Modify: `internal/llm/llm.go`
- Modify: `internal/llm/chat.go`
- Modify: `cmd/md2slack/main.go`
- Modify: `cmd/md2slack/processor.go`

**Step 1: Write the failing test**

Create a minimal compile-only check by running `go test` for package build coverage (no new test file required).

**Step 2: Run test to verify it fails**

Run: `go test ./...`
Expected: FAIL due to removed symbols (e.g., `ChatWithRequests`, `callJSON`).

**Step 3: Write minimal implementation**

- Remove `ChatWithRequests` usage and references in `cmd/md2slack/main.go` and any other call sites.
- Delete `internal/llm/chat.go` if it only contains non-streaming logic.
- Delete non-streaming helpers in `internal/llm/llm.go` (e.g., `callJSON`, JSON-only execution), keeping streaming-compatible pieces.

**Step 4: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/llm/llm.go internal/llm/chat.go cmd/md2slack/main.go cmd/md2slack/processor.go
git commit -m "refactor: remove non-streaming LLM paths"
```

### Task 2: Simplify streaming pipeline to LangChain-only

**Files:**
- Modify: `internal/llm/agent.go`
- Modify: `internal/llm/stream.go`
- Modify: `internal/llm/factory.go`
- Modify: `internal/llm/llm.go`

**Step 1: Write the failing test**

Use existing compile coverage (no new test file required).

**Step 2: Run test to verify it fails**

Run: `go test ./...`
Expected: FAIL if any old provider branching or unused interfaces remain referenced.

**Step 3: Write minimal implementation**

- Ensure `StreamChatWithRequests` is the only external chat entrypoint.
- Keep only LangChain types/interfaces used for streaming in `internal/llm/agent.go`.
- Remove provider-specific branches or options not used by streaming flow.
- Keep tool definitions used by streaming calls.

**Step 4: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/llm/agent.go internal/llm/stream.go internal/llm/factory.go internal/llm/llm.go
git commit -m "refactor: streamline LangChain streaming"
```

### Task 3: Clean up config/options and UI hooks

**Files:**
- Modify: `internal/config/config.go`
- Modify: `cmd/md2slack/main.go`
- Modify: `internal/webui/webui.go`

**Step 1: Write the failing test**

Run compile tests (no new test file required).

**Step 2: Run test to verify it fails**

Run: `go test ./...`
Expected: FAIL if any removed fields are still referenced.

**Step 3: Write minimal implementation**

- Remove unused LLM options and config fields that only existed for non-streaming paths.
- Keep only options required for streaming LangChain calls.
- Ensure web UI streaming callback wiring remains intact.

**Step 4: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/config.go cmd/md2slack/main.go internal/webui/webui.go
git commit -m "refactor: trim LLM options for streaming"
```

### Task 4: Verify end-to-end build

**Files:**
- Modify: none (verification only)

**Step 1: Run full test suite**

Run: `go test ./...`
Expected: PASS.

**Step 2: Commit**

No commit unless changes are needed.
