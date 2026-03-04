# Code review cleanup: pre-existing issues

Addresses findings from PR #1 code review that are pre-existing issues (not introduced by the version bump).

## Must Fix

### 1. Accept `ctx` in `ParseFromLog` — `pkg/report/parse.go`

`ParseFromLog` creates `context.Background()` instead of accepting `ctx context.Context`. Add `ctx` parameter, pass it through to all `errors.Wrap` calls. Update caller in `pkg/processor/processor.go` (`validateCompletionReport`).

### 2. Inject time dependency in `pkg/prompt/prompt.go`

5 calls to `time.Now()` in `PrepareForExecution`, `MarkCompleted`, `MarkFailed`, `MarkQueued`, `SetStatus`. Add a `nowFunc func() time.Time` field to `PromptFile`, defaulting to `time.Now`. Use it in all 5 locations. This enables deterministic testing.

### 3. Fix hardcoded defaults in `pkg/status/status.go`

`NewChecker` hardcodes `logDir: "prompts/log"` and `serverPort: 8080`, diverging from `config.Defaults()` (which has `ServerPort: 0`). Either:
- Accept these values as parameters to `NewChecker` (preferred — already done in `NewCheckerWithOptions`), or
- Remove `NewChecker` and only keep `NewCheckerWithOptions`, updating all callers

## Should Fix

### 4. Add `ctx` to `PromptFile.Save()` — `pkg/prompt/prompt.go`

`Save()` uses `fmt.Errorf` instead of `errors.Wrap(ctx, ...)`. Add `ctx context.Context` parameter. Update all callers.

### 5. Fix worktree double-cleanup — `pkg/processor/processor.go`

`setupWorktreeForExecution` defers worktree removal on function return, but the worktree is needed by the caller after return. Then `handleWorktreeWorkflow` removes it again. Remove the defer cleanup from `setupWorktreeForExecution` — let `handleWorktreeWorkflow` own cleanup exclusively.

### 6. Normalize `errors.Wrapf` → `errors.Wrap` — `pkg/runner/runner.go:136`

Replace `errors.Wrapf` with `errors.Wrap` + `fmt.Sprintf` to match project convention.

### 7. Remove business logic from factory — `pkg/factory/factory.go`

- Move `if cfg.ServerPort > 0` check into `server.NewServer` or a `NewOptionalServer` wrapper
- Move `slog.Info("project name resolved")` into `project.Name()` or `runner.NewRunner()`

### 8. Eliminate duplicate dependency creation in factory — `pkg/factory/factory.go`

`releaser` and `promptManager` are constructed identically in `CreateRunner`, `CreateStatusCommand`, and `CreateQueueCommand`. Extract shared construction.

## Missing Tests

### 9. Add tests for `extractPromptBaseName` — `pkg/executor/executor.go`

Pure function with two branches (with/without prompt prefix). Add `DescribeTable` test.

### 10. Add tests for `processUnreleasedSection` — `pkg/git/git.go`

Add `git_internal_test.go` with cases:
- No `## Unreleased` section → returns unchanged, `false`
- `## Unreleased` with entries → renames to version, `true`
- `## Unreleased` followed by another `##` → only renames first

### 11. Add tests for `MoveFile` — `pkg/git/git.go`

Three code paths: non-git fallback, staging failure, full git move. Add tests for each.

## Nice to Have

### 12. Hoist compiled regexps to package level

Move `regexp.MustCompile` calls from inside functions to `var` blocks:
- `pkg/processor/processor.go` — `sanitizeContainerName`
- `pkg/prompt/prompt.go` — `scanPromptFiles`, `hasNumberPrefix`, `extractNumberFromFilename`

### 13. Replace bubble sort with `sort.Slice` — `pkg/status/status.go:253`

`sortPromptsByTimeDescending` uses O(n²) manual sort.

### 14. Replace `formatInt` with `strconv.Itoa` — `pkg/status/status.go:409`

### 15. Use `errors.Errorf` in `main.go:58`

Replace `fmt.Errorf` — `ctx` is available.

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for changed packages
- Follow existing patterns exactly
