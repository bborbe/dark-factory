---
status: completed
container: dark-factory-347-processor-C3-queue-scanner
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T14:37:00Z"
queued: "2026-04-25T17:43:13Z"
started: "2026-04-25T18:22:20Z"
completed: "2026-04-25T18:59:28Z"
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
    // ScanAndProcess returns the count of prompts that completed during this scan,
    // which `runQueueTick` feeds into the post-#337 NothingToDoCallback (no-progress
    // detection in one-shot mode). DO NOT drop the count — it preserves the onIdle
    // signal that caused us to delete ProcessQueue in the first place.
    ScanAndProcess(ctx context.Context) (completed int, err error)
    HasPendingVerification(ctx context.Context) bool
}

// PromptManager is the minimal subset this package needs.
// Defined locally to avoid an import cycle (pkg/processor imports queuescanner).
type PromptManager interface {
    ListQueued(ctx context.Context) ([]prompt.Prompt, error)
    Load(ctx context.Context, path string) (*prompt.PromptFile, error)
    SaveStatus(ctx context.Context, pf *prompt.PromptFile) error
    AllPreviousCompleted(ctx context.Context, num int) bool
    // ... add only the methods Scanner actually calls
}

func NewScanner(
    promptManager PromptManager,
    promptProcessor PromptProcessor,
    failureHandler failurehandler.Handler,
    completedDir string,             // primitive — unwrap processor.Dirs.Completed at boundary
) Scanner { ... }
```

**Avoid the import cycle:** `pkg/processor` imports `queuescanner`. Therefore `queuescanner` MUST NOT import `pkg/processor` (don't reference `processor.PromptManager` / `processor.Dirs` / etc.). Use primitives in the public API and define a local minimal `PromptManager` interface — `processor.PromptManager` will satisfy it structurally.

Move bodies of all 6 methods. `skippedPrompts` and `lastBlockedMsg` become fields on `*scanner`.

## 2. Rename `processPrompt` → `ProcessPrompt`

Make the per-prompt entrypoint exported on processor so `Scanner` can call it through `PromptProcessor`. No behaviour change — just visibility.

## 3. Wire into processor

- Add `queueScanner queuescanner.Scanner` to `processor` struct
- Replace ALL `p.processExistingQueued(ctx)` call sites with `p.queueScanner.ScanAndProcess(ctx)`. Find them with `grep -n processExistingQueued pkg/processor/processor.go`. The processor's `runReadyTick` and `runQueueTick` helpers must keep capturing the `completed int` return and feeding it into `tickResult{completedPrompts: completed}` — otherwise the no-progress detection (post-#337 onIdle) silently breaks.
- Replace `p.hasPendingVerification(ctx)` → `p.queueScanner.HasPendingVerification(ctx)`
- Delete the 6 methods listed above
- Remove fields: `skippedPrompts`, `lastBlockedMsg` (live on scanner now)

## 4. Wire from factory and update ALL `NewProcessor` call sites

```bash
grep -rn "processor\.NewProcessor(" --include="*.go"
```

Update every call site (factory + ALL test files — recurring lesson: a `newTestProcessor` helper does NOT cover all direct constructor calls in tests).

Construct the scanner AFTER constructing the processor (since scanner needs `PromptProcessor`):

```go
// CreateProcessor:
proc := processor.NewProcessor(...)  // scanner is nil at this point — proc.SetScanner is required before Process is called
scanner := queuescanner.NewScanner(promptManager, proc, failureHandler, cfg.Prompts.CompletedDir)
proc.SetScanner(scanner)             // breaks the runtime cycle proc → scanner → proc.ProcessPrompt
```

Use the **setter** pattern (not the func-value alternative). The setter must be called by `CreateProcessor` before returning; processor should panic on `Process` if `queueScanner == nil` to catch wiring mistakes.

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
- Final `pkg/processor/processor.go` line count target: < 600 (the file is currently ~895 lines; removing ~310 lines from this prompt's 6 methods lands ~585 — adjust if intermediate prompts shrunk it further)
- `ScanAndProcess` MUST return `(int, error)` — dropping the count silently breaks the post-#337 onIdle no-progress detection
</constraints>

<verification>
```bash
cd /workspace

! grep -n "func (p \*processor) processExistingQueued\|func (p \*processor) processSingleQueued\|func (p \*processor) shouldSkipPrompt\|func (p \*processor) logBlockedOnce\|func (p \*processor) autoSetQueuedStatus\|func (p \*processor) hasPendingVerification" pkg/processor/processor.go

ls pkg/queuescanner/scanner.go mocks/queue-scanner.go mocks/prompt-processor.go
grep -n "queuescanner.Scanner" pkg/processor/processor.go

# No reverse import — queuescanner MUST NOT import processor
! grep -rn "github.com/bborbe/dark-factory/pkg/processor" pkg/queuescanner/

# Factory wires the scanner via setter
grep -n "queuescanner\.\|SetScanner" pkg/factory/factory.go

# Progress signal preserved — runQueueTick / runReadyTick capture int from ScanAndProcess
grep -n "completed.*ScanAndProcess\|p.queueScanner.ScanAndProcess" pkg/processor/processor.go

# All NewProcessor call sites updated
grep -rn "processor\.NewProcessor(" --include='*.go'

# Line-count target
wc -l pkg/processor/processor.go

# ProcessPrompt is exported now
grep -n "func (p \*processor) ProcessPrompt" pkg/processor/processor.go

# Final size check
wc -l pkg/processor/processor.go  # should be < 250

make precommit
```
</verification>
