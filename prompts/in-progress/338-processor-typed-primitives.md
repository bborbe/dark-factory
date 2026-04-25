---
status: committing
summary: Replaced NewProcessor primitive parameters with named types (ProjectName, ContainerName, BaseName, Dirs, Commands, MaxContainers, DirtyFileThreshold, AutoRetryLimit, AdditionalInstructions, VerificationGate); moved sanitizeContainerName to ContainerName.Sanitize() method; updated WorkflowExecutor interface and all implementations; updated factory.go, tests, and mocks.
container: dark-factory-338-processor-typed-primitives
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T14:21:00Z"
queued: "2026-04-25T14:50:10Z"
started: "2026-04-25T14:50:11Z"
---

<summary>
- Replace primitive `string` / `int` / `bool` parameters of `NewProcessor` with named types and grouped config structs to make argument-swap mistakes a compile error
- New types: `ProjectName`, `ContainerName` (with `Sanitize()` method replacing the free `sanitizeContainerName`), `Dirs` (Queue/Completed/Log), `Commands` (Validation/ValidationPrompt/Test), `MaxContainers`, `DirtyFileThreshold`, `AutoRetryLimit`, `AdditionalInstructions`, `VerificationGate`
- Push types ALL THE WAY THROUGH: `processor` struct fields, `factory.go` parsing, any internal helpers — no unwrapping at boundaries
- `computePromptMetadata` becomes a small constructor returning `(BaseName, ContainerName)` instead of `(string, string)`
</summary>

<objective>
Make the processor's many same-typed arguments self-documenting and swap-resistant by introducing named types, while pulling `sanitizeContainerName` onto the natural owner (`ContainerName`).
</objective>

<context>
**Prerequisite:** This prompt depends on `extract-validationprompt-package.md` having been applied first (introduces the `validationprompt.Resolver` parameter that this prompt also needs to type-correctly receive).

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Current `NewProcessor` (`pkg/processor/processor.go`, near the top of the file) takes 25+ raw `string` / `int` / `bool` parameters. Adjacent same-typed args mean the compiler can't catch swapped order:

```go
queueDir string,
completedDir string,
logDir string,
projectName string,
// ... swap any two and it compiles fine
```

Current `sanitizeContainerName` is a package-level free function called only from `computePromptMetadata`. Both live in `pkg/processor/processor.go` — locate by symbol name, not line number, since the file changes frequently.

`sanitizeContainerNameRegexp` is a package-level `var` — keep it package-private; do not introduce test hooks that swap it (see `go-composition.md` "Test-Only Package-Level Mutable State").

Naming conventions (Go community + this codebase): named types are PascalCase, singular, in the package they belong to. Group structs use named-field initialization at construction so positional swaps are impossible.
</context>

<requirements>

## 1. Define types

Choose the package per type ownership:

- `pkg/processor/types.go` (NEW file) — types specific to the processor configuration:
  ```go
  type ProjectName string
  type AdditionalInstructions string
  type MaxContainers int
  type DirtyFileThreshold int
  type AutoRetryLimit int
  type VerificationGate bool

  type Dirs struct {
      Queue, Completed, Log string
  }

  type Commands struct {
      Validation, ValidationPrompt, Test string
  }
  ```

- `pkg/processor/container_name.go` (NEW file in processor pkg — keeps the type internal until a second consumer needs it):
  ```go
  type ContainerName string
  type BaseName string

  // Sanitize replaces any character not in [a-zA-Z0-9_-] with '-'.
  func (n ContainerName) Sanitize() ContainerName { ... }

  // String returns the underlying string for use with exec / docker.
  func (n ContainerName) String() string { return string(n) }
  ```

  Move `sanitizeContainerNameRegexp` and the body of `sanitizeContainerName` here. Delete the free function.

## 2. Update `NewProcessor` signature and `processor` struct

- Replace the corresponding `string` / `int` / `bool` parameters with their typed equivalents
- `Dirs` and `Commands` collapse 6 string parameters into 2 struct parameters
- `processor` struct fields use the same types — do not unwrap to `string` / `int` internally

Argument REORDERING is OUT OF SCOPE for this prompt — keep the existing relative order; just swap the types in place. Reordering is the next prompt.

## 3. Update internal call sites

`computePromptMetadata` becomes:

```go
func computePromptMetadata(promptPath string, projectName ProjectName) (BaseName, ContainerName) {
    base := BaseName(strings.TrimSuffix(filepath.Base(promptPath), ".md"))
    name := ContainerName(string(projectName) + "-" + string(base)).Sanitize()
    return base, name
}
```

Audit all uses of the old primitive params inside `processor.go` — every read becomes typed. Where a `string` is required at a system boundary (e.g. `exec.CommandContext`, docker name argument, log fields), call `.String()` or do an explicit conversion.

## 4. Update `factory.go`

Where the factory currently builds raw strings/ints from config, build the typed versions:

```go
dirs := processor.Dirs{
    Queue:     filepath.Join(promptsDir, "queue"),
    Completed: filepath.Join(promptsDir, "completed"),
    Log:       filepath.Join(promptsDir, "log"),
}
commands := processor.Commands{
    Validation:       cfg.ValidationCommand,
    ValidationPrompt: cfg.ValidationPrompt,
    Test:             cfg.TestCommand,
}
```

Pass typed values into `NewProcessor`.

## 5. Update tests

- All tests constructing `NewProcessor` directly need updating to the new types
- Mocks regenerated via `make generate`
- Add `pkg/processor/container_name_test.go` covering `ContainerName.Sanitize()`. Boundary tests required (per dod.md "Test the boundaries"):
  - Behaviour previously tested via `sanitizeContainerName` (slashes, spaces, unicode → all replaced)
  - **Docker name regex contract**: `Sanitize()` output matches `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` (Docker's container-name rule). Test pathological inputs: leading dash, leading dot, leading underscore, empty, all-special-chars. If `Sanitize()` can produce a leading dash or dot (current sanitizer only filters chars), explicitly assert that — and either fix sanitizer to prepend a safe prefix or document the contract limitation.
- Existing tests for `computePromptMetadata` keep passing (signature changed but behaviour is identical).

## 6. CHANGELOG

Append to `## Unreleased` in `CHANGELOG.md`:

```
- refactor: NewProcessor primitive parameters replaced with named types (ProjectName, ContainerName, Dirs, Commands, ...) — purely internal, no behaviour change
```

## 7. Verify

```bash
cd /workspace
make generate
make precommit
```

Must exit 0.

</requirements>

<constraints>
- Argument REORDERING is OUT OF SCOPE — separate prompt handles that
- Types must be used everywhere — no unwrapping to raw strings inside processor
- At boundaries (exec, docker, log fields), explicit `.String()` or conversions are fine
- **No `Validate()` / `Parse()` methods on the new types** — sanitization happens in `ContainerName.Sanitize()` only; other types are zero-validation alias types. If validation is needed later, add it in a follow-up prompt with a clear contract.
- `sanitizeContainerNameRegexp` stays package-private; do NOT add a test setter or swap helper (anti-pattern per `go-composition.md`)
- External test packages where applicable
- Coverage ≥80% on changed packages
- `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new errors
- Do not commit
</constraints>

<verification>
```bash
cd /workspace

# Free function removed
! grep -n "^func sanitizeContainerName" pkg/processor/processor.go

# New types in use
grep -n "ProjectName\|ContainerName\|processor\.Dirs\|processor\.Commands" pkg/processor/processor.go

# Factory passes typed values
grep -rn "processor.Dirs{\|processor.Commands{" pkg/factory/

# Sanitize method exists
grep -rn "func (.*ContainerName) Sanitize" pkg/

make precommit
```
</verification>
