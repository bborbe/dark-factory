---
status: committing
summary: Extracted CompletionReportValidator (pkg/completionreport) and PromptEnricher (pkg/promptenricher) from processor god-object — both injected via constructor, mocks generated, tests at 91.7% and 92.0% coverage, processor.go reduced by 133 lines, make precommit exits 0
container: dark-factory-340-processor-A1-leaf-extractions
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T14:30:00Z"
queued: "2026-04-25T15:41:25Z"
started: "2026-04-25T15:41:27Z"
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

Targets in `pkg/processor/processor.go` (locate by symbol name; line numbers churn):
- `validateCompletionReport(ctx, logFile) (*report.CompletionReport, error)` — free function. Calls `report.ParseFromLog`, `report.ScanForCriticalFailures`. Used in two places (`resumePrompt` and `processPrompt`).
- `enrichPromptContent(ctx, content)` — method on `processor`. Reads `p.additionalInstructions`, `p.releaser.HasChangelog`, `p.cmds.Test`, `p.cmds.Validation`, `p.cmds.ValidationPrompt`, and calls the free function `resolveValidationPrompt(ctx, p.cmds.ValidationPrompt)`. Used once (in `runPrompt`).

Note: there is **no** `validationPromptResolver` field on `processor` — `resolveValidationPrompt` is a free function in `processor.go`. The enricher needs to either (a) take the resolver as a function-typed parameter, (b) call the free function directly (after we re-home it), or (c) accept a `validationprompt.Resolver` interface only if `extract-validationprompt-package.md` actually introduced one. Verify the prerequisite first; pick (b) if no interface exists yet.

Note: `p.testCommand` / `p.validationCommand` fields do **not** exist — those values live on `p.cmds` (a `Commands` struct) after `processor-typed-primitives.md`.

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

// Use primitive types in the constructor signature to avoid an import cycle
// (pkg/processor imports promptenricher; promptenricher cannot import processor).
// The factory unwraps processor.Commands into individual strings before calling NewEnricher.
func NewEnricher(
    releaser git.Releaser,
    additionalInstructions string,
    testCommand string,
    validationCommand string,
    validationPromptCriteria string, // raw value from cfg; resolver runs inside Enrich
) Enricher { ... }
```

If `extract-validationprompt-package.md` actually introduced a `validationprompt.Resolver` interface in a neutral package (no processor dependency), prefer accepting that interface here; otherwise inline the `resolveValidationPrompt` body into `enricher.go` so the resolver logic moves with it.

Move the body of `enrichPromptContent` into `(*enricher).Enrich`. Carry forward the same suffixes in the same order.

## 3. Wire into processor

In `pkg/processor/processor.go`:
- Add `completionReportValidator completionreport.Validator` and `promptEnricher promptenricher.Enricher` to `processor` struct
- Add the same as parameters to `NewProcessor` (services group, per existing convention from `processor-rename-and-reorder.md`)
- Replace both call sites of `validateCompletionReport` with `p.completionReportValidator.Validate(ctx, logFile)`
- Replace `p.enrichPromptContent(ctx, content)` with `p.promptEnricher.Enrich(ctx, content)`
- Delete `validateCompletionReport` (free) and `enrichPromptContent` (method)
- Move the `resolveValidationPrompt` free function into `pkg/promptenricher/` (it's only used by the enricher) and delete it from `processor.go`
- Remove the now-unused fields: `additionalInstructions`, the relevant fields on `p.cmds` (or the whole `Commands` struct field if nothing else reads it). Verify with `grep -n "p\.cmds\|p\.additionalInstructions" pkg/processor/processor.go` after deletion. Keep `releaser` — still used elsewhere (`recoverCommittingPrompt`, workflow commits).

## 4. Wire from factory and update ALL `NewProcessor` call sites

```bash
grep -rn "processor\.NewProcessor(" --include="*.go"
```

This finds every call site. Update ALL of them — typically: `pkg/factory/factory.go`, `pkg/processor/processor_test.go` (helper `newTestProcessor`), and any direct `processor.NewProcessor(...)` calls inside test files (recurring lesson from 337/338/339: the helper does NOT cover all test call sites). Each site needs the new service params.

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

# Factory + tests updated for the new constructor params
grep -rn "promptenricher\.\|completionreport\." pkg/factory/factory.go pkg/processor/processor_test.go

# All NewProcessor call sites pass the new services (zero compile errors)
grep -rn "processor\.NewProcessor(" --include='*.go'

make precommit
```

Line count of `pkg/processor/processor.go` should drop by ~100 lines.
</verification>
