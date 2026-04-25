---
status: idea
created: "2026-04-25T14:21:00Z"
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

Current `NewProcessor` signature (`pkg/processor/processor.go` line 78) takes 25+ raw `string` / `int` / `bool` parameters. Adjacent same-typed args mean the compiler can't catch swapped order. Example:

```go
queueDir string,
completedDir string,
logDir string,
projectName string,
// ... swap any two and it compiles fine
```

Current `sanitizeContainerName` is a package-level free function (line ~1349) called only from `computePromptMetadata` (line ~1274).

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

- `pkg/container/name.go` (NEW package + file) OR `pkg/processor/container_name.go` (NEW file in processor pkg, your call — pick the location that keeps `ContainerName` reusable):
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
- Add a small `pkg/processor/types_test.go` (or wherever ContainerName lives) covering `ContainerName.Sanitize()` behaviour previously tested via `sanitizeContainerName`

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
grep -n "processor.Dirs{\|processor.Commands{" pkg/factory/

# Sanitize method exists
grep -rn "func (.*ContainerName) Sanitize" pkg/

make precommit
```
</verification>
