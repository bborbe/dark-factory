---
status: completed
summary: Extracted CommittingRecoverer from processor into pkg/committingrecoverer — pure refactor with no behaviour change, 94.4% test coverage on new package.
container: dark-factory-346-processor-C2-committing-recoverer
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T14:36:00Z"
queued: "2026-04-25T17:43:13Z"
started: "2026-04-25T18:05:16Z"
completed: "2026-04-25T18:22:19Z"
---

<summary>
- Extract committing-prompt recovery from `pkg/processor/processor.go` into a `pkg/committingrecoverer/` package
- Wraps: `processCommittingPrompts` (line ~298), `recoverCommittingPrompt` (line ~318)
- Two methods: `RecoverAll(ctx)` (best-effort, logs failures) and `Recover(ctx, promptPath) error` (single-prompt recovery, used internally)
- Removes ~70 lines from processor and isolates the git-retry logic
</summary>

<objective>
Pull committing-state recovery out of the processor god-object into a service that can be tested without spawning real git commands.
</objective>

<context>
**Prerequisites:** A1 + A2 + A3 + B1 + B2 + C1 must have landed first.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Current code (`pkg/processor/processor.go`):
- `processCommittingPrompts(ctx)` at line ~298 — finds all committing prompts, recovers each, swallows errors (logs)
- `recoverCommittingPrompt(ctx, path) error` at line ~318 — committing-recovery flow: load prompt → check dirty → commit work → auto-complete linked specs → move to completed → commit completed file
- `ResumeCommitting(ctx) error` at line ~291 (Processor interface method) just calls `processCommittingPrompts`

Touches: `promptManager`, `releaser`, `autoCompleter`, `completedDir`, plus package-level `git.HasDirtyFiles`, `git.CommitWithRetry`, `git.CommitAll`, `git.DefaultCommitBackoff`.
</context>

<requirements>

## 1. New package `pkg/committingrecoverer/`

`pkg/committingrecoverer/recoverer.go`:

```go
package committingrecoverer

//counterfeiter:generate -o ../../mocks/committing-recoverer.go --fake-name Recoverer . Recoverer

// Recoverer retries git commits for prompts left in `committing` status (e.g. after a daemon crash).
type Recoverer interface {
    // RecoverAll iterates all committing prompts; failures are logged and swallowed.
    RecoverAll(ctx context.Context)

    // Recover attempts to commit dirty work files and move a single prompt to completed.
    Recover(ctx context.Context, promptPath string) error
}

// Local minimal interface — defined here (not imported from pkg/processor) to avoid an import cycle.
type PromptManager interface {
    Load(ctx context.Context, path string) (*prompt.PromptFile, error)
    FindCommitting(ctx context.Context) ([]string, error)
    MoveToCompleted(ctx context.Context, path string) error
    // ... add only the methods Recoverer actually calls
}

func NewRecoverer(
    promptManager PromptManager,
    releaser git.Releaser,
    autoCompleter spec.AutoCompleter,
    completedDir string,                 // primitive — unwrap processor.Dirs.Completed at boundary
) Recoverer { ... }
```

**Avoid the import cycle:** `pkg/processor` imports `committingrecoverer`. Therefore `committingrecoverer` MUST NOT import `pkg/processor`. Use primitives in the public API and define a local minimal `PromptManager` interface — `processor.PromptManager` will satisfy it structurally.

The package-level `git.*` calls (`HasDirtyFiles`, `CommitWithRetry`, `CommitAll`) stay as-is — extracting another git-wrapper is out of scope. Document the dependency in the constructor docstring.

## 2. Update `Processor` interface

`ResumeCommitting(ctx) error` stays on `Processor` (matches the symmetry with `ResumeExecuting`). Implementation becomes:

```go
func (p *processor) ResumeCommitting(ctx context.Context) error {
    p.committingRecoverer.RecoverAll(ctx)
    return nil // always non-fatal
}
```

## 3. Wire into processor

- Add `committingRecoverer committingrecoverer.Recoverer` to `processor` struct
- Add as constructor parameter (services group)
- Replace ALL internal calls to `p.processCommittingPrompts(ctx)` with `p.committingRecoverer.RecoverAll(ctx)`. Find them with `grep -n processCommittingPrompts pkg/processor/processor.go` (locate by symbol; line numbers churn between groups).
- Delete `processCommittingPrompts` and `recoverCommittingPrompt`
- Leave `releaser` and `autoCompleter` fields in place — C3 (queue scanner) is the next prompt and may reference them; if not, a follow-up cleanup prompt removes them.

## 4. Wire from factory and update ALL `NewProcessor` call sites

```bash
grep -rn "processor\.NewProcessor(" --include="*.go"
```

Update every call site (factory + ALL test files — recurring lesson: a `newTestProcessor` helper does NOT cover all direct constructor calls in tests).

Construct `committingrecoverer.NewRecoverer(...)` in `pkg/factory/factory.go`, unwrap `processor.Dirs.Completed` to a primitive at the boundary, pass into `NewProcessor`.

## 5. Tests

- `pkg/committingrecoverer/recoverer_test.go`: cover — find returns empty (no-op), find returns paths, dirty files present (commits work then moves), no dirty files (only moves), spec auto-complete fails (logs, continues), move fails (returns error wrapped), commit-completed-file fails (returns error wrapped), ctx cancelled mid-loop (stops cleanly)
- Update processor tests to mock `Recoverer`

## 6. CHANGELOG

```
- refactor: extracted CommittingRecoverer from processor — pure refactor, no behaviour change
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
- Failures inside `RecoverAll` remain logged-and-swallowed; only `Recover` per-prompt returns errors
- External test packages
- Coverage ≥80% on new package
- `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`
- Do not commit
</constraints>

<verification>
```bash
cd /workspace

! grep -n "func (p \*processor) processCommittingPrompts\|func (p \*processor) recoverCommittingPrompt" pkg/processor/processor.go

ls pkg/committingrecoverer/recoverer.go mocks/committing-recoverer.go
grep -n "committingrecoverer.Recoverer" pkg/processor/processor.go

# No reverse import — committingrecoverer MUST NOT import processor
! grep -rn "github.com/bborbe/dark-factory/pkg/processor" pkg/committingrecoverer/

# Factory wires the recoverer
grep -n "committingrecoverer\." pkg/factory/factory.go

# All NewProcessor call sites updated
grep -rn "processor\.NewProcessor(" --include='*.go'

make precommit
```
</verification>
