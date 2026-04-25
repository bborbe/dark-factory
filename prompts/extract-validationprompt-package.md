---
status: idea
created: "2026-04-25T14:20:00Z"
---

<summary>
- Extract `resolveValidationPrompt` from `pkg/processor/processor.go` (line ~1359) into a new `pkg/validationprompt/` package behind a `Resolver` interface
- Inject `validationprompt.Resolver` into `processor` via `NewProcessor` — replaces the in-package free function and the implicit `os.Stat`/`os.ReadFile` dependency
- Counterfeiter mock generated via `make generate` so processor tests no longer touch disk
- Return `(text, ok, error)` instead of `(text, ok)` — caller decides logging; no errors swallowed
</summary>

<objective>
Move validationPrompt resolution into its own package behind an injected interface, removing file-I/O from processor and making the concern independently testable.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/` — small services injected via constructor, free helpers doing I/O are a god-object smell.

Current code (`pkg/processor/processor.go`):
- Free function `resolveValidationPrompt(ctx, value) (string, bool)` at line ~1359
- Called from `enrichPromptContent` at line ~1137: `if criteria, ok := resolveValidationPrompt(ctx, p.validationPrompt); ok { ... }`
- Currently swallows read errors as warnings — switch to returning the error so the caller logs.

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

## 2. Wire into processor

In `pkg/processor/processor.go`:
- Add `validationPromptResolver validationprompt.Resolver` to `processor` struct
- Add the same as a parameter to `NewProcessor` (placement: keep existing arg order in this prompt — argument reordering is a separate prompt)
- Replace call site in `enrichPromptContent`:

```go
criteria, ok, err := p.validationPromptResolver.Resolve(ctx, p.validationPrompt)
if err != nil {
    slog.WarnContext(ctx, "failed to resolve validationPrompt",
        "value", p.validationPrompt, "error", err)
} else if ok {
    content = content + report.ValidationPromptSuffix(criteria)
}
```

- Delete `resolveValidationPrompt` from `processor.go`

## 3. Wire from factory

In `pkg/factory/factory.go` (or wherever `NewProcessor` is called), construct `validationprompt.NewResolver()` and pass it.

## 4. Tests

- New `pkg/validationprompt/resolver_test.go` (external test package): cases — empty, inline text, existing file (uses `t.TempDir()`), path-shaped missing file, unreadable file
- Update existing processor tests that depended on the inline `resolveValidationPrompt` behaviour to use the counterfeiter mock

## 5. Generate + verify

```bash
cd /workspace
make generate
make precommit
```

Both must exit 0.

</requirements>

<constraints>
- External test packages
- Coverage ≥80% on new package
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`
- Do not commit
- Argument reordering is OUT OF SCOPE — keep new param adjacent to existing related params
</constraints>

<verification>
```bash
cd /workspace

# Free function gone
! grep -n "func resolveValidationPrompt" pkg/processor/processor.go

# New package exists with mock
ls pkg/validationprompt/resolver.go
ls mocks/validationprompt-resolver.go

# Processor uses injected interface
grep -n "validationprompt.Resolver" pkg/processor/processor.go

# Factory wires it
grep -n "validationprompt.NewResolver" pkg/factory/

make precommit
```
</verification>
