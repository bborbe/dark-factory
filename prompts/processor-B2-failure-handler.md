---
status: idea
created: "2026-04-25T14:34:00Z"
---

<summary>
- Extract prompt-failure handling from `pkg/processor/processor.go` into a `pkg/failurehandler/` package
- Wraps: `handleProcessError` (line ~695), `checkPostExecutionFailure` (line ~712), `handlePromptFailure` (line ~734), `notifyFailed` (line ~776), `notifyFromReport` (line ~786)
- Single primary method: `Handle(ctx, promptPath, err) error` — returns non-nil to signal daemon shutdown (post-execution failure), nil otherwise
- Removes ~120 lines from processor and isolates retry-policy logic for testing
</summary>

<objective>
Pull all the prompt-failure / retry / shutdown-detection logic out of the processor and into one cohesive service so retry policy can evolve without touching the orchestration loop.
</objective>

<context>
**Prerequisites:** A1 + A2 + A3 + B1 must have landed first.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Current code (`pkg/processor/processor.go`):
- `handleProcessError(ctx, path, err) error` at line ~695 — top-level dispatcher: ctx cancelled → propagate, post-exec failure → propagate (shutdown), else → call `handlePromptFailure` and continue
- `checkPostExecutionFailure(ctx, path, err) error` at line ~712 — detects "file moved to completed/ but post-exec git step failed"
- `handlePromptFailure(ctx, path, err)` at line ~734 — retry vs. mark-failed logic, increments `retryCount`
- `notifyFailed(ctx, path)` at line ~776
- `notifyFromReport(ctx, logFile, promptPath)` at line ~786 — fires partial notification

Caller: `processSingleQueued` at line ~589 (`p.handleProcessError(...)`) and various spots that call `notifyFromReport`.
</context>

<requirements>

## 1. New package `pkg/failurehandler/`

`pkg/failurehandler/handler.go`:

```go
package failurehandler

//counterfeiter:generate -o ../../mocks/failure-handler.go --fake-name Handler . Handler

// Handler decides what to do when a prompt fails. Either re-queues for retry,
// marks failed and notifies, or returns a non-nil error to stop the daemon
// when a post-execution git step failed (manual intervention required).
type Handler interface {
    // Handle is the top-level dispatcher. Returns nil to continue the scan loop;
    // returns wrapped error to stop the daemon (post-execution failure or ctx cancellation).
    Handle(ctx context.Context, promptPath string, err error) error

    // NotifyFromReport fires a partial-completion notification if the log indicates one.
    // Best-effort; failures are logged and swallowed.
    NotifyFromReport(ctx context.Context, logFile string, promptPath string)
}

func NewHandler(
    promptManager processor.PromptManager,
    notifier notifier.Notifier,
    completedDir processor.Dirs,        // uses .Completed
    projectName processor.ProjectName,
    autoRetryLimit processor.AutoRetryLimit,
) Handler { ... }
```

Move bodies of all 5 methods into the new package.

## 2. Wire into processor

- Add `failureHandler failurehandler.Handler` to `processor` struct
- Add as constructor parameter (services group)
- Replace `p.handleProcessError(...)` → `p.failureHandler.Handle(...)`
- Replace `p.notifyFromReport(...)` → `p.failureHandler.NotifyFromReport(...)`
- Delete the 5 methods from processor
- Remove now-unused fields: `autoRetryLimit`, `notifier` (if unused elsewhere — check `preflight.Checker` constructor still gets it via factory; processor itself no longer needs it)

## 3. Wire from factory

`pkg/factory/factory.go`: construct `failurehandler.NewHandler(...)`, pass into `NewProcessor`.

## 4. Tests

- `pkg/failurehandler/handler_test.go`: cover — ctx cancelled (returns wrapped error), post-execution failure (file gone from path, exists in completed/) returns wrapped error, pre-execution failure with retries available (re-queues with incremented count), retries exhausted (marks failed + notifies), `autoRetryLimit == 0` (always marks failed), prompt load fails during failure handling (logs, falls through), notifyFromReport with no log (no-op), notifyFromReport with partial status (notifies)
- Update processor tests to use counterfeiter mock

## 5. CHANGELOG

```
- refactor: extracted FailureHandler from processor — pure refactor, no behaviour change
```

## 6. Verify

```bash
cd /workspace
make generate
make precommit
```

</requirements>

<constraints>
- Pure refactor — no behaviour change, retry semantics identical
- The shutdown-on-post-execution-failure must still trigger — daemon stops, prompt stays in completed/
- External test packages
- Coverage ≥80% on new package
- `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`
- Do not commit
</constraints>

<verification>
```bash
cd /workspace

! grep -n "func (p \*processor) handleProcessError\|func (p \*processor) checkPostExecutionFailure\|func (p \*processor) handlePromptFailure\|func (p \*processor) notifyFailed\|func (p \*processor) notifyFromReport" pkg/processor/processor.go

ls pkg/failurehandler/handler.go mocks/failure-handler.go
grep -n "failurehandler.Handler" pkg/processor/processor.go

make precommit
```
</verification>
