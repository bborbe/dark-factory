---
status: idea
created: "2026-04-25T14:30:00Z"
---

<summary>
- First leaf extraction from `pkg/processor/processor.go` (1395 lines) — pull two pure-ish helpers behind small injected interfaces
- `CompletionReportValidator`: wraps `validateCompletionReport` (line ~1289) — single method `Validate(ctx, logFile) (*report.CompletionReport, error)`
- `PromptEnricher`: wraps `enrichPromptContent` (line ~1118) — single method `Enrich(ctx, content) string`
- Both are SRP services injected into `processor` via constructor — counterfeiter mocks make processor tests deterministic
- Foundation for the larger processor decomposition (prompts A2 → C3)
</summary>

<objective>
Extract two leaf helpers from the processor god-object into small injected services so they can be unit-tested in isolation and processor tests stop exercising completion-report parsing / content-enrichment as side effects.
</objective>

<context>
**Prerequisites:** `extract-validationprompt-package.md`, `processor-typed-primitives.md`, `processor-rename-and-reorder.md` must have landed first — this prompt assumes typed primitives + the `validationprompt.Resolver` interface already exist.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/` — small interfaces (1–2 methods), all deps via constructor, package functions wrapped behind interfaces.

Targets in `pkg/processor/processor.go`:
- `validateCompletionReport(ctx, logFile) (*report.CompletionReport, error)` — free function at line ~1289. Calls `report.ParseFromLog`, `report.ScanForCriticalFailures`. Used in two places (`resumePrompt` line ~421 and `processPrompt` line ~1069).
- `enrichPromptContent(ctx, content)` — method on `processor` at line ~1118. Reads `p.additionalInstructions`, `p.releaser.HasChangelog`, `p.testCommand`, `p.validationCommand`, `p.validationPromptResolver`. Used once (line ~1018).

Reference shape: `pkg/preflight/preflight.go` — small package, single-method interface, `New*` constructor, mock under `mocks/`.
</context>

<requirements>

## 1. New package `pkg/completionreport/`

`pkg/completionreport/validator.go`:

```go
package completionreport

//counterfeiter:generate -o ../../mocks/completion-report-validator.go --fake-name Validator . Validator

// Validator parses and consistency-checks the completion report from a prompt log.
type Validator interface {
    Validate(ctx context.Context, logFile string) (*report.CompletionReport, error)
}

func NewValidator() Validator {
    return &validator{}
}

type validator struct{}
```

Move the body of `validateCompletionReport` into `(*validator).Validate`. No behaviour change.

## 2. New package `pkg/promptenricher/`

`pkg/promptenricher/enricher.go`:

```go
package promptenricher

//counterfeiter:generate -o ../../mocks/prompt-enricher.go --fake-name Enricher . Enricher

// Enricher prepends additionalInstructions and appends machine-parseable suffixes
// (completion report, changelog hint, test command, validation command, validation criteria).
type Enricher interface {
    Enrich(ctx context.Context, content string) string
}

func NewEnricher(
    releaser git.Releaser,
    validationPromptResolver validationprompt.Resolver,
    additionalInstructions processor.AdditionalInstructions,
    commands processor.Commands,
) Enricher { ... }
```

Move the body of `enrichPromptContent` into `(*enricher).Enrich`. Carry forward the same suffixes in the same order.

## 3. Wire into processor

In `pkg/processor/processor.go`:
- Add `completionReportValidator completionreport.Validator` and `promptEnricher promptenricher.Enricher` to `processor` struct
- Add the same as parameters to `NewProcessor` (services group, per existing convention from `processor-rename-and-reorder.md`)
- Replace both call sites of `validateCompletionReport` with `p.completionReportValidator.Validate(ctx, logFile)`
- Replace `p.enrichPromptContent(ctx, content)` with `p.promptEnricher.Enrich(ctx, content)`
- Delete `validateCompletionReport` (free) and `enrichPromptContent` (method)
- Remove the now-unused fields: `additionalInstructions`, `testCommand`, `validationCommand`, `validationPromptResolver` (they live on the enricher now). Keep `releaser` — still used elsewhere (`recoverCommittingPrompt`, workflow commits).

## 4. Wire from factory

`pkg/factory/factory.go`: construct both services and pass into `NewProcessor`. Remove the now-redundant primitive parameters from the call.

## 5. Tests

- `pkg/completionreport/validator_test.go` (external test pkg): cover all branches in current `validateCompletionReport` — no report + no critical failure (success), no report + critical failure (error), parseable success, parseable failure status, consistency override, malformed report
- `pkg/promptenricher/enricher_test.go`: cover empty / non-empty additionalInstructions, with / without changelog, with / without testCommand / validationCommand / validationPrompt criteria, ordering of appended suffixes
- Update processor tests that depended on the inlined logic to use counterfeiter mocks

## 6. CHANGELOG

```
- refactor: extracted CompletionReportValidator and PromptEnricher from processor — pure refactor, no behaviour change
```

## 7. Verify

```bash
cd /workspace
make generate
make precommit
```

Both must exit 0.

</requirements>

<constraints>
- Pure refactor — no behaviour change
- External test packages
- Coverage ≥80% on both new packages
- `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`
- Do not commit
</constraints>

<verification>
```bash
cd /workspace

# Free function gone
! grep -n "^func validateCompletionReport" pkg/processor/processor.go

# Method gone
! grep -n "func (p \*processor) enrichPromptContent" pkg/processor/processor.go

# New packages exist
ls pkg/completionreport/ pkg/promptenricher/
ls mocks/completion-report-validator.go mocks/prompt-enricher.go

# Processor uses interfaces
grep -n "completionreport.Validator\|promptenricher.Enricher" pkg/processor/processor.go

make precommit
```

Line count of `pkg/processor/processor.go` should drop by ~100 lines.
</verification>
