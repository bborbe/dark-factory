---
status: idea
created: "2026-04-25T14:32:00Z"
---

<summary>
- Extract `checkPromptedSpecs` (pkg/processor/processor.go line ~802) into a `pkg/specsweeper/` package behind a `Sweeper` interface
- Single method: `Sweep(ctx) error` — scans specs, transitions `prompted` specs whose linked prompts are all complete to `verifying`
- Used at startup, periodic sweep ticker, and one-shot mode
- Removes ~20 lines from processor.go and isolates the spec-completion-detection logic
</summary>

<objective>
Pull the periodic spec-sweep responsibility out of the processor and into its own service so the auto-complete-on-restart logic can be tested + extended independently.
</objective>

<context>
**Prerequisites:** A1 + A2 must have landed first.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Current code (`pkg/processor/processor.go`):
- `checkPromptedSpecs(ctx) error` at line ~802 — calls `p.specLister.List(ctx)`, filters to `StatusPrompted`, calls `p.autoCompleter.CheckAndComplete` per spec
- Three call sites:
  1. `Process()` startup at line ~183
  2. Sweep ticker case at line ~233
  3. `ProcessQueue()` startup at line ~247

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
    Sweep(ctx context.Context) error
}

func NewSweeper(specLister spec.Lister, autoCompleter spec.AutoCompleter) Sweeper { ... }
```

Move body of `checkPromptedSpecs` into `(*sweeper).Sweep`.

## 2. Wire into processor

- Add `specSweeper specsweeper.Sweeper` to `processor` struct
- Add as constructor parameter (services group)
- Replace all three call sites with `p.specSweeper.Sweep(ctx)`
- Delete `checkPromptedSpecs` method
- Remove `specLister` and `autoCompleter` fields from processor IF they have no remaining users. (`autoCompleter` is also used by `recoverCommittingPrompt` — keep it. `specLister` is only used by `checkPromptedSpecs` — remove.)

## 3. Wire from factory

`pkg/factory/factory.go`: construct `specsweeper.NewSweeper(specLister, autoCompleter)` and pass.

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

make precommit
```
</verification>
