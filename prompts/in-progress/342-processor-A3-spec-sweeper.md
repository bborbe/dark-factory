---
status: committing
summary: Extracted checkPromptedSpecs into pkg/specsweeper.Sweeper interface with NewSweeper constructor, wired into processor via specSweeper field replacing specLister, updated all three NewProcessor call sites (factory.go + two test locations), generated counterfeiter mock, added 100%-coverage tests, and updated CHANGELOG.md.
container: dark-factory-342-processor-A3-spec-sweeper
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T14:32:00Z"
queued: "2026-04-25T15:41:25Z"
started: "2026-04-25T16:22:55Z"
---

<summary>
- Extract `checkPromptedSpecs` from `pkg/processor/processor.go` into a `pkg/specsweeper/` package behind a `Sweeper` interface
- Single method: `Sweep(ctx) (transitioned int, err error)` — scans specs, transitions `prompted` specs whose linked prompts are all complete to `verifying`. Returns the count of specs that transitioned so the caller (runSweepTick) can report progress for the post-337 `onIdle` callback
- Used at startup and the periodic sweep ticker (two call sites — `ProcessQueue` was removed in 337)
- Removes ~20 lines from processor.go and isolates the spec-completion-detection logic
</summary>

<objective>
Pull the periodic spec-sweep responsibility out of the processor and into its own service so the auto-complete-on-restart logic can be tested + extended independently.
</objective>

<context>
**Prerequisites:** A1 + A2 must have landed first.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Current code (`pkg/processor/processor.go` — locate by symbol):
- `checkPromptedSpecs(ctx) (int, error)` — calls `p.specLister.List(ctx)`, filters to `StatusPrompted`, calls `p.autoCompleter.CheckAndComplete` per spec, returns count of specs that transitioned + error. **Returns `(int, error)` already (post-337); the new `Sweep` keeps the same signature** so `runSweepTick` continues feeding `transitionedSpecs` into `tickResult.madeProgress()`.
- Two call sites (verify by `grep -n checkPromptedSpecs pkg/processor/processor.go`):
  1. `Process()` startup
  2. `runSweepTick` — fed into `tickResult{transitionedSpecs: transitioned}` for the `onIdle` callback

`ProcessQueue` was removed in prompt 337 — there is no third call site any more.

The spec sweep is conceptually independent from prompt processing — it only happens to live in `processor` for historical reasons.
</context>

<requirements>

## 1. New package `pkg/specsweeper/`

`pkg/specsweeper/sweeper.go`:

```go
package specsweeper

//counterfeiter:generate -o ../../mocks/spec-sweeper.go --fake-name Sweeper . Sweeper

// Sweeper transitions specs in `prompted` status whose linked prompts are all complete
// to `verifying`. Self-healing safety net for the per-prompt auto-complete path.
type Sweeper interface {
    // Sweep returns the number of specs that transitioned to verifying.
    // The count is consumed by the processor's runSweepTick to drive the
    // NothingToDoCallback (no-progress detection in one-shot mode).
    Sweep(ctx context.Context) (transitioned int, err error)
}

func NewSweeper(specLister spec.Lister, autoCompleter spec.AutoCompleter) Sweeper { ... }
```

Move body of `checkPromptedSpecs` into `(*sweeper).Sweep`. The signature and partial-count semantics carry over verbatim — error from CheckAndComplete stops iteration and returns the count-so-far.

## 2. Wire into processor

- Add `specSweeper specsweeper.Sweeper` to `processor` struct
- Add as constructor parameter (services group)
- Replace BOTH call sites with `transitioned, err := p.specSweeper.Sweep(ctx)` — preserve the count assignment so `runSweepTick` keeps feeding `tickResult.transitionedSpecs`
- Delete `checkPromptedSpecs` method
- Remove `specLister` field — only used by `checkPromptedSpecs`. Verify with `grep -n p.specLister pkg/processor/processor.go` — if no other reference, drop the field. Keep `autoCompleter` — still used by `recoverCommittingPrompt`.

## 3. Wire from factory and update ALL `NewProcessor` call sites

```bash
grep -rn "processor\.NewProcessor(" --include="*.go"
```

Update every call site (factory + all test files — recurring lesson from 337/338/339: a `newTestProcessor` helper does NOT cover all direct constructor calls in tests).

`pkg/factory/factory.go`: construct `specsweeper.NewSweeper(specLister, autoCompleter)` and pass into `NewProcessor`. Remove the now-unused direct `specLister` parameter from `NewProcessor` if it was passed.

## 4. Tests

- `pkg/specsweeper/sweeper_test.go` (external test pkg): cover — no specs (no-op), no prompted specs (no calls to CheckAndComplete), prompted spec succeeds, prompted spec returns error (propagates), list error (propagates wrapped)
- Update processor tests that exercised `checkPromptedSpecs` to use the counterfeiter mock

## 5. CHANGELOG

```
- refactor: extracted SpecSweeper from processor — pure refactor, no behaviour change
```

## 6. Verify

```bash
cd /workspace
make generate
make precommit
```

</requirements>

<constraints>
- Pure refactor — no behaviour change
- External test packages
- Coverage ≥80% on new package
- `errors.Wrap` from `github.com/bborbe/errors`
- Do not commit
</constraints>

<verification>
```bash
cd /workspace

! grep -n "func (p \*processor) checkPromptedSpecs" pkg/processor/processor.go
ls pkg/specsweeper/sweeper.go mocks/spec-sweeper.go
grep -n "specsweeper.Sweeper\|p.specSweeper" pkg/processor/processor.go

# Progress signal preserved — Sweep returns int and runSweepTick reads it
grep -n "transitioned.*Sweep\|p.specSweeper.Sweep" pkg/processor/processor.go

# All NewProcessor call sites updated
grep -rn "processor\.NewProcessor(" --include='*.go'

make precommit
```
</verification>
