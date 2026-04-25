---
status: idea
created: "2026-04-25T14:37:00Z"
---

<summary>
- Final extraction: pull queue-scanning loop from `pkg/processor/processor.go` into a `pkg/queuescanner/` package
- Wraps: `processExistingQueued` (line ~526), `processSingleQueued` (line ~553), `shouldSkipPrompt` (line ~602), `logBlockedOnce` (line ~636), `autoSetQueuedStatus` (line ~662), `hasPendingVerification` (line ~1200)
- Two methods: `ScanAndProcess(ctx) error` (drains the queue) and `HasPendingVerification(ctx) bool` (used by the event loop)
- Final processor.go shape: ~150–200 lines — just `Process()`, `ProcessQueue()`, `processPrompt()` orchestration
</summary>

<objective>
Final pass on the processor god-object: extract the queue scanner into its own service so processor reduces to a pure event-loop + per-prompt orchestrator.
</objective>

<context>
**Prerequisites:** A1 + A2 + A3 + B1 + B2 + C1 + C2 must have landed first.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Current code (`pkg/processor/processor.go`):
- `processExistingQueued(ctx) error` at line ~526 — outer loop over queued prompts
- `processSingleQueued(ctx) (done bool, err error)` at line ~553 — inner step: list, filter, validate, dispatch to `processPrompt`
- `shouldSkipPrompt(ctx, pr) bool` at line ~602 — skip-validated cache (`p.skippedPrompts`)
- `logBlockedOnce(ctx, pr)` at line ~636 — dedup blocked logging (`p.lastBlockedMsg`)
- `autoSetQueuedStatus(ctx, pr) error` at line ~662 — folder-as-truth status normalization
- `hasPendingVerification(ctx) bool` at line ~1200

These methods own private state: `skippedPrompts map[string]libtime.DateTime`, `lastBlockedMsg string`. State lives on the scanner.

Notable wrinkle: `processSingleQueued` calls `p.processPrompt(ctx, pr)` — the per-prompt orchestrator. `processPrompt` STAYS on processor (it's the orchestration layer the scanner depends on). Resolve via the scanner accepting a `PromptProcessor` interface that `processor` itself satisfies:

```go
type PromptProcessor interface {
    ProcessPrompt(ctx context.Context, pr prompt.Prompt) error
}
```

Wire processor → scanner with `processor` implementing `PromptProcessor`. Renames `processPrompt` → `ProcessPrompt`.
</context>

<requirements>

## 1. New package `pkg/queuescanner/`

`pkg/queuescanner/scanner.go`:

```go
package queuescanner

//counterfeiter:generate -o ../../mocks/queue-scanner.go --fake-name Scanner . Scanner
//counterfeiter:generate -o ../../mocks/prompt-processor.go --fake-name PromptProcessor . PromptProcessor

// PromptProcessor executes a single prompt end-to-end.
// Implemented by *processor (avoids a cycle: scanner depends on processor's per-prompt entrypoint).
type PromptProcessor interface {
    ProcessPrompt(ctx context.Context, pr prompt.Prompt) error
}

// Scanner drives the queue-scan loop: list queued, validate, dispatch to PromptProcessor, handle blockers.
type Scanner interface {
    ScanAndProcess(ctx context.Context) error
    HasPendingVerification(ctx context.Context) bool
}

func NewScanner(
    promptManager processor.PromptManager,
    promptProcessor PromptProcessor,
    failureHandler failurehandler.Handler,
    dirs processor.Dirs,
) Scanner { ... }
```

Move bodies of all 6 methods. `skippedPrompts` and `lastBlockedMsg` become fields on `*scanner`.

## 2. Rename `processPrompt` → `ProcessPrompt`

Make the per-prompt entrypoint exported on processor so `Scanner` can call it through `PromptProcessor`. No behaviour change — just visibility.

## 3. Wire into processor

- Add `queueScanner queuescanner.Scanner` to `processor` struct
- Add as constructor parameter (services group)
- Replace `p.processExistingQueued(ctx)` → `p.queueScanner.ScanAndProcess(ctx)` (3 call sites: line ~188, ~219, ~227)
- Replace `p.hasPendingVerification(ctx)` → `p.queueScanner.HasPendingVerification(ctx)`
- Delete the 6 methods listed above
- Remove fields: `skippedPrompts`, `lastBlockedMsg` (live on scanner now)

## 4. Wire from factory

Construct the scanner AFTER constructing the processor (since scanner needs `PromptProcessor`):

```go
proc := processor.NewProcessor(...)  // without scanner yet — temporary
scanner := queuescanner.NewScanner(promptManager, proc, failureHandler, dirs)
proc.SetScanner(scanner)             // setter to break the cycle
```

OR: change the construction to a two-phase pattern where processor exposes a setter for the scanner. Document the cycle-break in code comments.

(Alternative: have processor expose a method-value bound to `ProcessPrompt` and pass it as a `func` instead of an interface — uglier, recommend setter.)

## 5. Tests

- `pkg/queuescanner/scanner_test.go`: cover — empty queue, queued prompt processed, pending verification (no scan), validate-fail (skipped, recorded in skippedPrompts), file unchanged on next pass (skipped silently), file modified (re-validated), blocked on prior prompt (logged once), prior completed (unblocks), preflight skip propagates and stops scan, status auto-set
- Update processor tests to mock `Scanner`

## 6. CHANGELOG

```
- refactor: extracted QueueScanner from processor — final pass; processor.go reduced from 1395 to ~200 lines
```

## 7. Verify

```bash
cd /workspace
make generate
make precommit
```

</requirements>

<constraints>
- Pure refactor — no behaviour change
- The `processor → scanner → processor.ProcessPrompt` cycle must be broken via setter or two-phase construction (documented)
- External test packages
- Coverage ≥80% on new package
- `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`
- Do not commit
- Final `pkg/processor/processor.go` line count target: < 250 (verify in <verification>)
</constraints>

<verification>
```bash
cd /workspace

! grep -n "func (p \*processor) processExistingQueued\|func (p \*processor) processSingleQueued\|func (p \*processor) shouldSkipPrompt\|func (p \*processor) logBlockedOnce\|func (p \*processor) autoSetQueuedStatus\|func (p \*processor) hasPendingVerification" pkg/processor/processor.go

ls pkg/queuescanner/scanner.go mocks/queue-scanner.go mocks/prompt-processor.go
grep -n "queuescanner.Scanner" pkg/processor/processor.go

# ProcessPrompt is exported now
grep -n "func (p \*processor) ProcessPrompt" pkg/processor/processor.go

# Final size check
wc -l pkg/processor/processor.go  # should be < 250

make precommit
```
</verification>
