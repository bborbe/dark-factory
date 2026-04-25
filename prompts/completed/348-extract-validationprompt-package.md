---
status: completed
summary: Extracted resolveValidationPrompt into pkg/validationprompt.Resolver interface, injected into promptenricher.NewEnricher, updated all call sites, generated counterfeiter mock, and replaced disk-touching enricher tests with mock-based tests.
container: dark-factory-348-extract-validationprompt-package
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T14:20:00Z"
queued: "2026-04-25T19:02:56Z"
started: "2026-04-25T19:02:57Z"
completed: "2026-04-25T19:14:55Z"
---

<summary>
- Extract `resolveValidationPrompt` from `pkg/promptenricher/enricher.go` into a new `pkg/validationprompt/` package behind a `Resolver` interface
- Inject `validationprompt.Resolver` into `promptenricher.NewEnricher` — replaces the free function and its implicit `os.Stat`/`os.ReadFile` dependency
- Counterfeiter mock generated via `make generate` so enricher tests no longer touch disk
- Return `(text, ok, error)` instead of `(text, ok)` — caller decides logging; no errors swallowed
- Pure refactor — no behaviour change, all existing tests pass
</summary>

<objective>
Move validationPrompt resolution into its own injected interface, removing file-I/O from `promptenricher` and making the concern independently testable. The function already moved out of `processor` during prompt 340 (A1 leaf extractions); this prompt completes the journey by giving it its own package.
</objective>

<context>
**Prerequisite shipped:** prompt 340 (`processor-A1-leaf-extractions`) moved `resolveValidationPrompt` from `pkg/processor/processor.go` into `pkg/promptenricher/enricher.go` as part of the `PromptEnricher` extraction. Verify with `grep -n resolveValidationPrompt pkg/promptenricher/enricher.go` before starting.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` from the coding plugin docs — small services injected via constructor, free helpers doing I/O are a god-object smell.

Current code (post-340) — locate by symbol; line numbers churn:
- Free function `resolveValidationPrompt(ctx, value) (string, bool)` lives in `pkg/promptenricher/enricher.go`
- Called inside the enricher's `Enrich` method: roughly `if criteria, ok := resolveValidationPrompt(ctx, e.validationPromptCriteria); ok { ... }` (field name may be `validationPromptCriteria` or whatever 340 chose — verify with grep)
- Currently swallows read errors as warnings — switch to returning the error so the caller logs

Reference shape: `pkg/preflight/preflight.go` — small package, `Checker` interface, counterfeiter mock under `mocks/`.
</context>

<requirements>

## 1. New package `pkg/validationprompt/`

Create `pkg/validationprompt/resolver.go`:

```go
package validationprompt

import "context"

//counterfeiter:generate -o ../../mocks/validationprompt-resolver.go --fake-name Resolver . Resolver

// Resolver resolves a validationPrompt config value into criteria text.
//   - empty value          → ("", false, nil)
//   - file path (exists)   → (file contents, true, nil)
//   - path-shaped, missing → ("", false, nil) — log warning at caller
//   - inline text          → (value, true, nil)
//   - file read error      → ("", false, err)
type Resolver interface {
    Resolve(ctx context.Context, value string) (string, bool, error)
}

// NewResolver creates a filesystem-backed Resolver.
func NewResolver() Resolver {
    return &resolver{}
}

type resolver struct{}
```

Move the body of `resolveValidationPrompt` into `(*resolver).Resolve`. Use `errors.Wrap` from `github.com/bborbe/errors` for the read-error case.

**Avoid the import cycle:** `pkg/validationprompt` MUST NOT import `pkg/promptenricher` or `pkg/processor`. It is a leaf package — only stdlib + `github.com/bborbe/errors`.

## 2. Wire into promptenricher

In `pkg/promptenricher/enricher.go`:
- Add `validationPromptResolver validationprompt.Resolver` field to the `enricher` struct
- Add as the LAST parameter to `NewEnricher` — placement matches the convention from prompt 339 (services group). Verify with `grep -n "func NewEnricher" pkg/promptenricher/enricher.go`.
- Replace the call site inside `Enrich`:

```go
criteria, ok, err := e.validationPromptResolver.Resolve(ctx, e.validationPromptCriteria)
if err != nil {
    slog.WarnContext(ctx, "failed to resolve validationPrompt",
        "value", e.validationPromptCriteria, "error", err)
} else if ok {
    content = content + report.ValidationPromptSuffix(criteria)
}
```

(Field name may differ — verify with `grep -n "validationPrompt" pkg/promptenricher/enricher.go` first.)

- Delete `resolveValidationPrompt` from `pkg/promptenricher/enricher.go`
- Confirm processor.go no longer references it: `! grep -n resolveValidationPrompt pkg/processor/processor.go` (should be empty already from 340)

## 3. Wire from factory and update ALL `promptenricher.NewEnricher` call sites

```bash
grep -rn "promptenricher\.NewEnricher(\|NewEnricher(" --include="*.go"
```

Update every call site — typically one in `pkg/factory/factory.go` plus any in `pkg/promptenricher/enricher_test.go` (recurring lesson: helper functions don't always cover all direct constructor calls in tests).

In `pkg/factory/factory.go`, construct `validationprompt.NewResolver()` and pass it to `promptenricher.NewEnricher(...)`. No change to `processor.NewProcessor` — processor doesn't see the resolver.

## 4. Tests

- New `pkg/validationprompt/resolver_test.go` (external test package): cases — empty value, inline text, existing file (uses `t.TempDir()`), path-shaped missing file, unreadable file (chmod 0000 then assert read-error returned)
- Update `pkg/promptenricher/enricher_test.go` to inject a counterfeiter `validationprompt.Resolver` mock instead of relying on disk I/O. Old test cases that wrote files to `t.TempDir()` should be replaced with mock returns.

## 5. Generate + verify

```bash
cd /workspace
make generate
make precommit
```

Both must exit 0.

</requirements>

<constraints>
- Pure refactor — no behaviour change beyond surfacing read errors as warnings (matches prior behaviour)
- External test packages
- Coverage ≥80% on `pkg/validationprompt/`
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`
- Do not commit
- `pkg/validationprompt/` is a LEAF package — no imports of `pkg/promptenricher` or `pkg/processor`
- This prompt does NOT touch `pkg/processor/processor.go` (resolveValidationPrompt already moved out by prompt 340); if grep finds a leftover reference there, it's a regression to flag, not to extend
- Existing tests pass unchanged after the refactor
</constraints>

<verification>
```bash
cd /workspace

# Free function gone from BOTH locations (processor was 340, promptenricher is this prompt)
! grep -rn "func resolveValidationPrompt" pkg/processor/ pkg/promptenricher/

# New package exists with mock
ls pkg/validationprompt/resolver.go
ls mocks/validationprompt-resolver.go

# Promptenricher uses injected interface
grep -n "validationprompt.Resolver" pkg/promptenricher/enricher.go

# Factory wires it
grep -rn "validationprompt.NewResolver" pkg/factory/

# No reverse import — validationprompt MUST NOT import promptenricher or processor
! grep -rn "github.com/bborbe/dark-factory/pkg/promptenricher\|github.com/bborbe/dark-factory/pkg/processor" pkg/validationprompt/

# All NewEnricher call sites updated
grep -rn "NewEnricher(" --include='*.go'

make precommit
```
</verification>
