---
status: executing
container: dark-factory-345-processor-C1-prompt-resumer
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T14:35:00Z"
queued: "2026-04-25T17:43:13Z"
started: "2026-04-25T17:43:15Z"
---

<summary>
- Extract executing-prompt resume logic from `pkg/processor/processor.go` into a `pkg/promptresumer/` package
- Wraps: `ResumeExecuting` (line ~270), `resumePrompt` (line ~370), `prepareResume` (line ~438), `killTimedOutContainer` (line ~475), `computeReattachDuration` (line ~498)
- Single primary method: `ResumeAll(ctx) error` — scans queue dir, reattaches each executing prompt, drives to completion
- Removes ~165 lines from processor and isolates the most subtle code path (timeout calculation + reattach + post-execution flow)
</summary>

<objective>
Pull executing-prompt resume out of the processor god-object so the reattach + timeout-on-resume logic — currently the trickiest code path in processor.go — becomes independently testable.
</objective>

<context>
**Prerequisites:** A1 + A2 + A3 + B1 + B2 must have landed first.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Current code (`pkg/processor/processor.go`):
- `ResumeExecuting(ctx) error` — Processor interface method called once on daemon startup before the event loop
- `resumePrompt(ctx, path) error` — drives a single executing prompt: load → reconstruct workflow state → reattach to container → reload prompt → validate report → complete
- `prepareResume(ctx, path) (*PromptFile, ContainerName, BaseName, logFile, title, error)` — load + status check + container-name extraction
- `killTimedOutContainer(ctx, pf, containerName, elapsed) error` — kills containers that exceeded `maxPromptDuration` while the daemon was down
- `computeReattachDuration(started string) (remaining, elapsed time.Duration, exceeded bool)` — pure calculation

Touches many deps: `promptManager`, `executor`, `workflowExecutor`, `completionReportValidator` (post-A1), `failureHandler.NotifyFromReport` (post-B2), `releaser` for the post-completion git path, `projectName`, `logDir`, `completedDir`, `maxPromptDuration`.
</context>

<requirements>

## 1. New package `pkg/promptresumer/`

`pkg/promptresumer/resumer.go`:

```go
package promptresumer

//counterfeiter:generate -o ../../mocks/prompt-resumer.go --fake-name Resumer . Resumer

// Resumer reattaches to and drives executing prompts to completion.
// Used once at daemon startup before the event loop.
type Resumer interface {
    ResumeAll(ctx context.Context) error
}

// Local minimal interfaces — defined here (not imported from pkg/processor) to avoid an import cycle.
// Go interface satisfaction is structural, so processor.PromptManager / processor.WorkflowExecutor
// satisfy these automatically.
type PromptManager interface {
    Load(ctx context.Context, path string) (*prompt.PromptFile, error)
    SaveStatus(ctx context.Context, pf *prompt.PromptFile) error
    MarkFailed(ctx context.Context, path string) error
    // ... add only the methods Resumer actually calls
}

type WorkflowExecutor interface {
    ReconstructState(ctx context.Context, base string, pf *prompt.PromptFile) (bool, error)
    Complete(ctx context.Context, ...) error
    // ... only what Resumer needs
}

func NewResumer(
    promptManager PromptManager,
    executor executor.Executor,
    workflowExecutor WorkflowExecutor,
    completionReportValidator completionreport.Validator,
    failureHandler failurehandler.Handler,
    queueDir string,                     // primitive — unwrap processor.Dirs.Queue at boundary
    completedDir string,                 // primitive — unwrap processor.Dirs.Completed at boundary
    logDir string,                       // primitive — unwrap processor.Dirs.Log at boundary
    projectName string,                  // primitive — unwrap processor.ProjectName at boundary
    maxPromptDuration time.Duration,
) Resumer { ... }
```

**Avoid the import cycle:** `pkg/processor` imports `promptresumer`. Therefore `promptresumer` MUST NOT import `pkg/processor`. Use primitives in the public API and define local minimal interfaces.

Move all 5 methods. `computeReattachDuration` stays unexported. `prepareResume`, `killTimedOutContainer` stay unexported.

## 2. Update `Processor` interface

The current `Processor` interface (line ~62) declares `ResumeExecuting(ctx) error`. Two options:

- **a.** Keep on Processor — processor delegates to `p.resumer.ResumeAll(ctx)`. Existing callers don't change. (Recommended.)
- **b.** Remove from Processor — caller (factory or runner) calls `resumer.ResumeAll` directly. Bigger blast radius.

Recommend **a**. Keep the daemon-runner's existing call shape unchanged.

## 3. Wire into processor

- Add `resumer promptresumer.Resumer` to `processor` struct
- Add as constructor parameter (services group)
- `ResumeExecuting` becomes a one-liner: `return p.resumer.ResumeAll(ctx)`
- Delete `resumePrompt`, `prepareResume`, `killTimedOutContainer`, `computeReattachDuration`
- Remove fields: `maxPromptDuration` (lives on resumer now); `logDir` (lives on resumer)

Keep `completedDir` on processor — still used by `recoverCommittingPrompt` (until C2 lands).

## 4. Wire from factory and update ALL `NewProcessor` call sites

```bash
grep -rn "processor\.NewProcessor(" --include="*.go"
```

Update every call site (factory + ALL test files — recurring lesson: a `newTestProcessor` helper does NOT cover all direct constructor calls in tests).

Construct `promptresumer.NewResumer(...)` in `pkg/factory/factory.go`, unwrap `processor.Dirs.*` and `processor.ProjectName` to primitives at the boundary, pass into `NewProcessor`. Drop the now-redundant primitives that previously went directly to processor.

## 5. Tests

- `pkg/promptresumer/resumer_test.go`: cover — empty queue dir (no-op), non-executing prompt skipped, missing container name (resets to approved), workflow state cannot be reconstructed (resets to approved), exceeded timeout on resume (kills container + marks failed), normal reattach + completion, reattach error path, post-completion validation success / failure, log-dir path computation
- `computeReattachDuration_test.go` (or table-driven inside `resumer_test.go`): zero duration, no started timestamp, malformed timestamp, in-window, exceeded
- Update processor tests to mock `Resumer`

## 6. CHANGELOG

```
- refactor: extracted PromptResumer from processor — pure refactor, no behaviour change
```

## 7. Verify

```bash
cd /workspace
make generate
make precommit
```

</requirements>

<constraints>
- Pure refactor — no behaviour change. `ResumeExecuting` continues to be called once on startup.
- Reattach timeout calculation must be byte-identical (existing tests likely cover edge cases — they must still pass)
- External test packages
- Coverage ≥80% on new package
- `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`
- Do not commit
</constraints>

<verification>
```bash
cd /workspace

! grep -n "func (p \*processor) resumePrompt\|func (p \*processor) prepareResume\|func (p \*processor) killTimedOutContainer\|func (p \*processor) computeReattachDuration" pkg/processor/processor.go

ls pkg/promptresumer/resumer.go mocks/prompt-resumer.go
grep -n "promptresumer.Resumer" pkg/processor/processor.go

# ResumeExecuting is now a one-liner
grep -A2 "func (p \*processor) ResumeExecuting" pkg/processor/processor.go | head -5

# No reverse import — promptresumer MUST NOT import processor
! grep -rn "github.com/bborbe/dark-factory/pkg/processor" pkg/promptresumer/

# Factory wires the resumer
grep -n "promptresumer\." pkg/factory/factory.go

# All NewProcessor call sites updated
grep -rn "processor\.NewProcessor(" --include='*.go'

# Removed fields gone from processor
! grep -n "maxPromptDuration\b" pkg/processor/processor.go
! grep -n "p\.logDir\b" pkg/processor/processor.go

make precommit
```
</verification>
