---
status: created
spec: [052-split-prompt-manager]
created: "2026-04-16T19:53:47Z"
branch: dark-factory/split-prompt-manager
---

<summary>
- All eight consumer packages switch their constructor parameter and struct field from `prompt.Manager` to the local `PromptManager` interface defined in the previous prompt
- Package-level helper functions in `pkg/runner` (`normalizeFilenames`, `resumeOrResetExecuting`, `runHealthCheckLoop`, `checkExecutingPrompts`) switch their `mgr prompt.Manager` parameters to `mgr PromptManager`
- `pkg/runner/export_test.go` helper signatures are updated to accept `PromptManager` instead of `prompt.Manager`
- Test files across all eight packages replace `&mocks.Manager{}` with the package-specific fake (e.g., `&mocks.ProcessorPromptManager{}`) and replace stub/spy calls accordingly
- The factory continues to pass `prompt.Manager` (the wide interface) to each consumer — this compiles because `prompt.Manager` is a superset of every narrow interface
- `make test` passes with all behavioral changes intact
</summary>

<objective>
Wire the narrow `PromptManager` interfaces (created in prompt 1) into each consumer package: update constructors, struct fields, helper function parameters, export_test.go helpers, and test files. The factory still passes `prompt.Manager` which satisfies all narrow interfaces by structural typing — no factory changes are needed yet. This prompt achieves full test coverage with per-consumer fakes sized to the actual method count.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

**Precondition:** Prompt 1 (1-spec-052-consumer-interfaces) has already been executed. The following are in place:
- `pkg/processor/prompt_manager.go` defines `PromptManager` (9 methods) with counterfeiter directive
- `pkg/runner/prompt_manager.go` defines `PromptManager` (3 methods) with counterfeiter directive
- `pkg/server/prompt_manager.go` defines `PromptManager` (1 method) with counterfeiter directive
- `pkg/status/prompt_manager.go` defines `PromptManager` (4 methods) with counterfeiter directive
- `pkg/review/prompt_manager.go` defines `PromptManager` (5 methods) with counterfeiter directive
- `pkg/watcher/prompt_manager.go` defines `PromptManager` (1 method) with counterfeiter directive
- `pkg/cmd/prompt_manager.go` defines `PromptManager` (2 methods) with counterfeiter directive
- `mocks/processor-prompt-manager.go` (`ProcessorPromptManager`), `mocks/runner-prompt-manager.go` (`RunnerPromptManager`), etc. exist

Files to read before editing:
- Each `pkg/<consumer>/prompt_manager.go` — exact interface method signatures
- Each generated `mocks/<consumer>-prompt-manager.go` — exact method stub names (e.g., `NormalizeFilenamesReturns`, `NormalizeFilenamesArgsForCall`)
- `pkg/processor/processor.go` — constructor and struct definition
- `pkg/runner/runner.go`, `pkg/runner/oneshot.go`, `pkg/runner/lifecycle.go`, `pkg/runner/health_check.go`, `pkg/runner/export_test.go`
- `pkg/server/queue_action_handler.go`, `pkg/server/queue_helpers.go`
- `pkg/status/status.go`
- `pkg/review/poller.go`
- `pkg/watcher/watcher.go`
- `pkg/cmd/approve.go`, `pkg/cmd/unapprove.go`, `pkg/cmd/prompt_complete.go`
- All corresponding `*_test.go` files that use `mocks.Manager{}`
</context>

<requirements>

## Strategy

For each consumer package, make these mechanical changes in order:
1. Update the struct field type: `prompt.Manager` → `PromptManager`
2. Update the constructor parameter type: `prompt.Manager` → `PromptManager`
3. For `pkg/runner`: update helper function signatures (`normalizeFilenames`, `resumeOrResetExecuting`, `runHealthCheckLoop`, `checkExecutingPrompts`) and `export_test.go`
4. Update test files: replace `*mocks.Manager` → `*mocks.<Package>PromptManager`, replace `&mocks.Manager{}` → `&mocks.<Package>PromptManager{}`, update import paths if needed

The factory (`pkg/factory/factory.go`) does NOT change in this prompt. `prompt.Manager` (the wide interface) satisfies every narrow `PromptManager` interface, so passing `prompt.Manager` to consumers that now accept `PromptManager` compiles without change.

Before editing each test file, run:
```bash
grep -n "mocks\.Manager\|promptManager\.\|promptMgr\." pkg/<package>/*_test.go
```
to see all the method stub calls that need renaming.

---

## 1. `pkg/processor`

### 1a. Update `pkg/processor/processor.go`

In the `processor` struct, change the field:
```go
// Before:
promptManager prompt.Manager
// After:
promptManager PromptManager
```

In `NewProcessor`, change the parameter:
```go
// Before:
promptManager prompt.Manager,
// After:
promptManager PromptManager,
```

Remove the import of `"github.com/bborbe/dark-factory/pkg/prompt"` if it is no longer used directly in `processor.go`. (Check: if `processor.go` still references `prompt.PromptFile`, `prompt.Prompt`, `prompt.PromptStatus`, `prompt.ApprovedPromptStatus`, etc. then the import must stay.)

### 1b. Update `pkg/processor/processor_test.go`

Find all occurrences of `*mocks.Manager` and replace with `*mocks.ProcessorPromptManager`.
Find all occurrences of `&mocks.Manager{}` and replace with `&mocks.ProcessorPromptManager{}`.

The method stub names on `ProcessorPromptManager` are named identically to those on `Manager` (e.g., `ListQueuedReturns`, `LoadReturns`, `SetStatusReturns`). The stub calls do not need to change — only the type declaration changes.

Run `make test ./pkg/processor/...` after this step to verify.

---

## 2. `pkg/runner`

### 2a. Update `pkg/runner/runner.go`

In the `runner` struct, change:
```go
// Before:
promptManager prompt.Manager
// After:
promptManager PromptManager
```

In `NewRunner`, change the parameter:
```go
// Before:
promptManager prompt.Manager,
// After:
promptManager PromptManager,
```

### 2b. Update `pkg/runner/oneshot.go`

In the `oneShotRunner` struct, change:
```go
// Before:
promptManager prompt.Manager
// After:
promptManager PromptManager
```

In `NewOneShotRunner`, change the parameter:
```go
// Before:
promptManager prompt.Manager,
// After:
promptManager PromptManager,
```

### 2c. Update `pkg/runner/lifecycle.go`

Change the `normalizeFilenames` helper signature:
```go
// Before:
func normalizeFilenames(ctx context.Context, mgr prompt.Manager, inProgressDir string) error {
// After:
func normalizeFilenames(ctx context.Context, mgr PromptManager, inProgressDir string) error {
```

Change the `resumeOrResetExecuting` helper signature:
```go
// Before:
func resumeOrResetExecuting(
    ctx context.Context,
    inProgressDir string,
    mgr prompt.Manager,
    ...
// After:
func resumeOrResetExecuting(
    ctx context.Context,
    inProgressDir string,
    mgr PromptManager,
    ...
```

Change the `resumeOrResetExecutingEntry` helper signature:
```go
// Before:
    mgr prompt.Manager,
// After:
    mgr PromptManager,
```

If `lifecycle.go` still imports `"github.com/bborbe/dark-factory/pkg/prompt"` for other reasons (e.g., `prompt.PromptFile`, `prompt.PromptStatus`, `prompt.ExecutingPromptStatus`), keep the import. Only remove it if no `prompt.*` symbols remain.

### 2d. Update `pkg/runner/health_check.go`

Change the `runHealthCheckLoop` helper signature:
```go
// Before:
    mgr prompt.Manager,
// After:
    mgr PromptManager,
```

Change the `checkExecutingPrompts` helper signature:
```go
// Before:
    mgr prompt.Manager,
// After:
    mgr PromptManager,
```

Change the `checkExecutingPrompt` helper signature (it also takes `mgr prompt.Manager`):
```go
// Before:
    mgr prompt.Manager,
// After:
    mgr PromptManager,
```

If `health_check.go` still imports `"github.com/bborbe/dark-factory/pkg/prompt"` for other reasons (e.g., `prompt.PromptFile`, `prompt.ExecutingPromptStatus`), keep the import.

### 2e. Update `pkg/runner/export_test.go`

Change `CheckExecutingPromptsForTest` parameter:
```go
// Before:
    mgr prompt.Manager,
// After:
    mgr PromptManager,
```

Change `RunHealthCheckLoopForTest` parameter:
```go
// Before:
    mgr prompt.Manager,
// After:
    mgr PromptManager,
```

Note: `export_test.go` is in `package runner` (not `package runner_test`), so it can use the unexported `PromptManager` interface directly.

### 2f. Update `pkg/runner/runner_test.go`, `pkg/runner/oneshot_test.go`, `pkg/runner/health_check_test.go`

In each file, replace `*mocks.Manager` with `*mocks.RunnerPromptManager` and `&mocks.Manager{}` with `&mocks.RunnerPromptManager{}`.

The method stub names are identical (e.g., `NormalizeFilenamesReturns`, `LoadReturns`, `ListQueuedReturns`).

Run `make test ./pkg/runner/...` after this step to verify.

---

## 3. `pkg/server`

### 3a. Update `pkg/server/queue_action_handler.go`

The `NewQueueActionHandler` function captures `promptManager` in a closure (not a struct field). Change its parameter type:
```go
// Before:
func NewQueueActionHandler(
    inboxDir string,
    queueDir string,
    promptManager prompt.Manager,
    ...
// After:
func NewQueueActionHandler(
    inboxDir string,
    queueDir string,
    promptManager PromptManager,
    ...
```

Also update the internal helper functions `handleQueueAll` and `handleQueueSingle` — they also take `promptManager prompt.Manager`. Change to `promptManager PromptManager` in their signatures.

### 3b. Update `pkg/server/queue_helpers.go`

Change all three helper function signatures (`queueSingleFile`, `queueAllFiles`, `moveToQueue`) — each takes `promptManager prompt.Manager`. Change to `promptManager PromptManager`.

If the `prompt` import is no longer used in `queue_helpers.go` after this change (check: does it still use `prompt.Load`, `prompt.PromptFile`, etc.), remove it. Otherwise keep it.

### 3c. Update `pkg/server/queue_action_handler_test.go`

Replace `*mocks.Manager` with `*mocks.ServerPromptManager` and `&mocks.Manager{}` with `&mocks.ServerPromptManager{}`.

Run `make test ./pkg/server/...` after this step to verify.

---

## 4. `pkg/status`

### 4a. Update `pkg/status/status.go`

In the `checker` struct, change:
```go
// Before:
promptMgr prompt.Manager
// After:
promptMgr PromptManager
```

In `NewChecker`, change the parameter:
```go
// Before:
promptMgr prompt.Manager,
// After:
promptMgr PromptManager,
```

If `status.go` no longer imports `"github.com/bborbe/dark-factory/pkg/prompt"` for anything else, remove the import. Otherwise keep it.

### 4b. Update `pkg/status/status_test.go`

Replace `*mocks.Manager` with `*mocks.StatusPromptManager` and `&mocks.Manager{}` with `&mocks.StatusPromptManager{}`.

Run `make test ./pkg/status/...` after this step to verify.

---

## 5. `pkg/review`

### 5a. Update `pkg/review/poller.go`

In the `reviewPoller` struct, change:
```go
// Before:
promptManager prompt.Manager
// After:
promptManager PromptManager
```

In `NewReviewPoller`, change the parameter:
```go
// Before:
promptManager prompt.Manager,
// After:
promptManager PromptManager,
```

If `poller.go` still uses `prompt.PromptFile`, `prompt.FailedPromptStatus`, etc., keep the `prompt` import.

### 5b. Update `pkg/review/poller_test.go`

Replace `*mocks.Manager` with `*mocks.ReviewPromptManager` and `&mocks.Manager{}` with `&mocks.ReviewPromptManager{}`.

Run `make test ./pkg/review/...` after this step to verify.

---

## 6. `pkg/watcher`

### 6a. Update `pkg/watcher/watcher.go`

In the `watcher` struct, change:
```go
// Before:
promptManager prompt.Manager
// After:
promptManager PromptManager
```

In `NewWatcher`, change the parameter:
```go
// Before:
promptManager prompt.Manager,
// After:
promptManager PromptManager,
```

### 6b. Update `pkg/watcher/watcher_test.go`

Replace all `*mocks.Manager` occurrences with `*mocks.WatcherPromptManager` and `&mocks.Manager{}` with `&mocks.WatcherPromptManager{}`.

Note: `watcher_test.go` uses local variables (`promptManager := &mocks.Manager{}`) rather than `BeforeEach` — update each declaration.

Run `make test ./pkg/watcher/...` after this step to verify.

---

## 7. `pkg/cmd`

### 7a. Update `pkg/cmd/approve.go`

In the `approveCommand` struct, change:
```go
// Before:
promptManager prompt.Manager
// After:
promptManager PromptManager
```

Change the `NewApproveCommand` (or equivalent) constructor parameter from `prompt.Manager` to `PromptManager`.

### 7b. Update `pkg/cmd/unapprove.go`

Same pattern: struct field and constructor parameter `prompt.Manager` → `PromptManager`.

### 7c. Update `pkg/cmd/prompt_complete.go`

Same pattern: struct field and constructor parameter `prompt.Manager` → `PromptManager`.

### 7d. Update test files

In `pkg/cmd/approve_test.go`, `pkg/cmd/unapprove_test.go`, `pkg/cmd/prompt_complete_test.go`:
Replace `*mocks.Manager` with `*mocks.CmdPromptManager` and `&mocks.Manager{}` with `&mocks.CmdPromptManager{}`.

Run `make test ./pkg/cmd/...` after this step to verify.

---

## 8. Verify factory still compiles

The factory at `pkg/factory/factory.go` currently calls:
- `NewProcessor(... promptManager, ...)` where `promptManager` is of type `prompt.Manager` (the wide interface)
- Similarly for all other consumers

After this prompt's changes, each consumer constructor accepts a narrow `PromptManager` interface. But `prompt.Manager` (the wide interface, which has 20 methods) is a superset of every narrow interface (which has ≤9 methods). In Go, an interface value satisfies any other interface whose method set is a subset. Therefore the factory continues to compile without modification.

Verify this by running:
```bash
cd /workspace && go build ./pkg/factory/...
```

If it fails, read the error and fix it before proceeding.

## 9. Run full `make test`

```bash
cd /workspace && make test
```

All tests must pass.

</requirements>

<constraints>
- Do NOT modify `pkg/factory/factory.go` in this prompt — the factory still passes `prompt.Manager` and that is intentional (works by structural typing)
- Do NOT remove `mocks/prompt-manager.go` — that happens in prompt 3
- Do NOT remove the `Manager` interface from `pkg/prompt/prompt.go` — that happens in prompt 3
- Do NOT commit — dark-factory handles git
- Preserve all existing logic — only change types, never restructure methods or move code
- When removing `prompt.Manager` type references, only remove the import if NO other symbol from `pkg/prompt` is used in that file
- Do not touch `go.mod` / `go.sum` / `vendor/`
- All existing tests must pass after each sub-step (run `make test ./pkg/<package>/...` iteratively)
</constraints>

<verification>
Run `make test` in `/workspace` — must pass.

Additional spot checks:
1. `grep -rn "prompt\.Manager" pkg/processor/ pkg/runner/ pkg/server/ pkg/status/ pkg/review/ pkg/watcher/ pkg/cmd/` — should return zero matches (no consumer file should reference the wide interface anymore)
2. `grep -rn "mocks\.Manager" pkg/` — should return zero matches (all tests use package-specific fakes)
3. `go build ./pkg/factory/...` — must succeed (factory still compiles without change)
4. `grep -n "PromptManager" pkg/processor/processor.go` — field and parameter show `PromptManager`
5. `grep -n "PromptManager" pkg/runner/runner.go pkg/runner/oneshot.go` — field and parameter show `PromptManager`
</verification>
