# Task CRUD SQLite Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make tasks a first-class CRUD stored in SQLite, and route all task modifications (tools, manual edits, analysis run) through a unified storage layer using stable `task_id`.

**Architecture:** Add a task CRUD module in `internal/storage` with `repo + date` keys and stable `task_id`. Update web UI handlers, analysis pipeline, and LLM tools to call the unified CRUD functions rather than mutating in-memory lists. DB becomes the source of truth; handlers reload from DB after each mutation.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), existing md2slack storage/webui/llm packages.

### Task 1: Add task CRUD storage module and schema

**Files:**
- Modify: `internal/storage/database.go`
- Create: `internal/storage/tasks.go`
- Modify: `internal/storage/storage.go`
- Test: `internal/storage/storage_test.go` (if exists) or new `internal/storage/tasks_test.go`

**Step 1: Write the failing test**

Create a test that:
- Inserts tasks for `repoA/dateA` and `repoA/dateB` and ensures isolation
- Updates a task by `task_id` and verifies the updated row
- Deletes a task and verifies remaining tasks

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -v`
Expected: FAIL with missing CRUD functions.

**Step 3: Write minimal implementation**

- Extend `initDB` schema with a `tasks` table (if not exists):
  - `id INTEGER PRIMARY KEY AUTOINCREMENT` (stable `task_id`)
  - `repo_name TEXT`, `date TEXT`
  - `task_json TEXT` (store the task payload)
  - `created_at`, `updated_at` (optional but recommended)
  - Unique index on `(repo_name, date, id)`
- Implement CRUD in `internal/storage/tasks.go`:
  - `LoadTasks(repo, date) ([]gitdiff.TaskChange, error)`
  - `CreateTask(repo, date, task) (taskID int, []gitdiff.TaskChange, error)`
  - `UpdateTask(repo, date, taskID, task) ([]gitdiff.TaskChange, error)`
  - `DeleteTasks(repo, date, ids []int) ([]gitdiff.TaskChange, error)`
  - `ReplaceTasks(repo, date, tasks []gitdiff.TaskChange) error`
- Ensure `task_id` is set on `TaskChange` when loading from DB (add field if needed).
- Update `internal/storage/storage.go` to expose these helpers.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/storage/database.go internal/storage/tasks.go internal/storage/storage.go internal/storage/tasks_test.go
git commit -m "feat: add sqlite task CRUD"
```

### Task 2: Wire CRUD into manual edit and run analysis

**Files:**
- Modify: `internal/webui/webui.go`
- Modify: `internal/webui/chat_handler.go`
- Modify: `cmd/md2slack/processor.go`
- Modify: `internal/storage/storage.go`

**Step 1: Write the failing test**

Add/extend a test to verify that manual update calls persist immediately (or add a small unit test around handlers if available).

**Step 2: Run test to verify it fails**

Run: `go test ./internal/webui -v`
Expected: FAIL until CRUD wired.

**Step 3: Write minimal implementation**

- `handleUpdateTask`: call `storage.UpdateTask(...)` using `repo/date` from server state, return the reloaded list.
- `handleLoadHistory`: load tasks via `storage.LoadTasks(...)` instead of history blob.
- `handleClearTasks`: delete all tasks for repo/date (new helper, e.g., `DeleteAllTasks`).
- Analysis pipeline: after generating tasks, call `storage.ReplaceTasks(repo, date, tasks)` so DB is the source of truth.
- Keep `SaveHistory` for report/summary persistence if still needed, but tasks come from task CRUD.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/webui -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/webui/webui.go internal/webui/chat_handler.go cmd/md2slack/processor.go internal/storage/storage.go
git commit -m "refactor: route manual edits and analysis through task CRUD"
```

### Task 3: Update LLM tools to use CRUD layer

**Files:**
- Modify: `internal/llm/tools/create_task.go`
- Modify: `internal/llm/tools/update_task.go`
- Modify: `internal/llm/tools/delete_task.go`
- Modify: `internal/llm/tools/tools.go`
- Modify: `internal/llm/stream.go`

**Step 1: Write the failing test**

Add/extend a test that simulates tool calls and verifies DB updates (integration style in `internal/llm` or `internal/storage`).

**Step 2: Run test to verify it fails**

Run: `go test ./internal/llm -v`
Expected: FAIL until tools write to DB.

**Step 3: Write minimal implementation**

- Add `Repo` and `Date` to tool context (pass into `tools.NewTaskTools`).
- In each tool, call the storage CRUD functions and return updated task list.
- Ensure `task_id` is used (not list index) for updates/deletes.
- After tool execution, reload tasks from DB and return them to UI.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/llm -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/llm/tools/*.go internal/llm/stream.go internal/llm/tools/tools.go
git commit -m "refactor: LLM tools persist tasks via sqlite"
```

### Task 4: End-to-end verification

**Files:**
- Modify: none (verification only)

**Step 1: Run full test suite**

Run: `go test ./...`
Expected: PASS.

**Step 2: Manual smoke check**

Run `./ssbot`, create a task via chat, refresh UI, and ensure task persists for the same repo/date.

**Step 3: Commit**

No commit unless changes are needed.
