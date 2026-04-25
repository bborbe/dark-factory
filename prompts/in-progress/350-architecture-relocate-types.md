---
status: committing
summary: 'Pure architectural refactor: moved BaseName/ContainerName types from pkg/processor to pkg/prompt; introduced project.Name named-string type with Resolve() function replacing Name(); removed processor.ProjectName type; injected queuescanner.Scanner via NewProcessor constructor eliminating SetScanner two-phase init; deleted workflowExecutorResumerAdapter from factory by using lazyPromptProcessor forwarder for circular wiring. All 42 packages pass tests, make precommit exits 0.'
container: dark-factory-350-architecture-relocate-types
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T22:00:00Z"
queued: "2026-04-25T20:08:34Z"
started: "2026-04-25T20:08:35Z"
---

<summary>
- Move `BaseName` and `ContainerName` from `pkg/processor` to `pkg/prompt` — they describe prompt-file metadata, not processor internals
- Drop `processor.ProjectName`; introduce a `project.Name` named-string type in `pkg/project` (the existing `project.Name` function gets renamed to `project.Resolve`); use `project.Name` consistently across factory, processor, runner, failurehandler, promptresumer, review
- Inject `queuescanner.Scanner` as a `NewProcessor` constructor parameter — replaces the `SetScanner` two-phase init that panics at runtime if forgotten
- Delete `workflowExecutorResumerAdapter` from `pkg/factory/factory.go` — the 20-line adapter exists only to bridge `string` ↔ `BaseName` across packages; once `BaseName` lives in `pkg/prompt`, both `processor.WorkflowExecutor` and `promptresumer.WorkflowExecutor` can use the real type and the adapter vanishes
- Pure refactor — no behaviour change; all existing tests pass unchanged
</summary>

<objective>
Address three findings from the architecture audit by moving types to the packages where they belong: `BaseName` / `ContainerName` to `pkg/prompt`, and consolidating `ProjectName` in `pkg/project`. This removes phantom import cycles, deletes the factory adapter shim, and lets `queuescanner.Scanner` be a normal constructor parameter instead of a setter.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

**Architectural principle for this prompt:** types belong in the lowest package that defines them. Moving `BaseName` / `ContainerName` from `pkg/processor` into `pkg/prompt` removes phantom import cycles and lets adapter shims in factory disappear.

Current state (locate by symbol; line numbers churn):
- `pkg/processor/types.go` defines `ProjectName string`
- `pkg/processor/container_name.go` defines `BaseName string` and `ContainerName string` with a `Sanitize()` method
- `pkg/project/name.go` currently defines a **function** `func Name(configOverride string) string` that resolves a project name from config/git/cwd. **There is no `project.Name` type yet.** Part of this prompt is renaming the function and introducing the type — see Requirement 2.
- `pkg/factory/factory.go::workflowExecutorResumerAdapter` (around line 58) wraps `processor.WorkflowExecutor` to satisfy `promptresumer.WorkflowExecutor`. The whole adapter exists because `processor.WorkflowExecutor` takes `processor.BaseName` and `promptresumer.WorkflowExecutor` takes `string` — same call, different types
- `pkg/processor/processor.go` has `SetScanner(s queuescanner.Scanner)` (around line 139) called from `pkg/factory/factory.go::CreateProcessor` (around line 867) AFTER the processor is constructed
- `processor.queueScanner` field is checked for nil at the start of `Process` and `ScanAndProcess`; nil triggers a panic

The audit framing: dual-API surface, phantom adapter, and two-phase init are all symptoms of the same thing — types living in the wrong package. The fix is to move the types, not to add more workarounds.

Read these key files before editing:
- `pkg/processor/types.go` — `ProjectName`, plus other typed primitives we leave alone in this prompt
- `pkg/processor/container_name.go` — `BaseName`, `ContainerName`, sanitization
- `pkg/project/name.go` — existing `Name` type
- `pkg/factory/factory.go` — adapter, NewProcessor wiring (~lines 58–80, 867)
- `pkg/processor/processor.go` — SetScanner, queueScanner field, Process entry
- `pkg/promptresumer/resumer.go` — local `WorkflowExecutor` interface that accepts `string`
- `pkg/queuescanner/scanner.go` — confirm Scanner interface shape
- `pkg/failurehandler/handler.go`, `pkg/runner/runner.go`, `pkg/review/` — call sites that currently `.String()`-erase `ProjectName`
</context>

<requirements>

## 1. Move `BaseName` and `ContainerName` to `pkg/prompt`

Create `pkg/prompt/base_name.go` and `pkg/prompt/container_name.go` (or one file `pkg/prompt/names.go` if it stays small) with the types and the `Sanitize()` method moved verbatim from `pkg/processor/container_name.go`. Same names — `prompt.BaseName`, `prompt.ContainerName`.

Move the corresponding test file `pkg/processor/container_name_test.go` → `pkg/prompt/container_name_test.go` (adjust package declaration to `prompt_test`).

Delete `pkg/processor/container_name.go` and `pkg/processor/container_name_test.go`.

Update every consumer:

```bash
grep -rn "processor\.BaseName\|processor\.ContainerName" --include='*.go'
```

Replace `processor.BaseName` → `prompt.BaseName`, `processor.ContainerName` → `prompt.ContainerName`. Add the `pkg/prompt` import where needed; remove now-unused `pkg/processor` imports.

## 2. Introduce `project.Name` type, rename existing function

The existing `pkg/project/name.go` has only a function `func Name(configOverride string) string`. We need a typed `project.Name string` to use across packages. Plan in this exact order:

### 2a. Rename the existing function

`func Name(configOverride string) string` → `func Resolve(configOverride string) string`. Same body, just renamed. Update its call sites (search via `grep -rn "project\.Name(" --include='*.go'`) — all factory + cmd entry points become `project.Resolve(cfg.ProjectName)`.

### 2b. Introduce the named type

In `pkg/project/name.go` (or a new sibling file `pkg/project/types.go` — pick the simpler placement):

```go
// Name is a typed alias for the resolved project name. Construct via Resolve.
type Name string

// String returns the underlying string for use at boundaries (logs, exec args, etc.).
func (n Name) String() string { return string(n) }
```

Update `Resolve` to return `Name`. **Wrap every existing return statement** in `Name(...)` — there are five (config override, git root, git remote, cwd basename, ultimate fallback). Don't miss any:

```go
func Resolve(configOverride string) Name {
    if configOverride != "" {
        return Name(configOverride)
    }
    if name := tryGitRoot(); name != "" {
        return Name(name)
    }
    if name := tryGitRemote(); name != "" {
        return Name(name)
    }
    if wd, err := os.Getwd(); err == nil {
        return Name(filepath.Base(wd))
    }
    return Name("dark-factory")
}
```

### 2c. Delete `processor.ProjectName`

Remove the type from `pkg/processor/types.go`. Find all `processor.ProjectName` references with `grep -rn "processor\.ProjectName" --include='*.go'` and replace with `project.Name`.

### 2d. Update every consumer to use `project.Name` directly

Update field types, parameter types, and local-interface declarations in:
- `pkg/processor/processor.go` (struct field, `NewProcessor` parameter)
- `pkg/factory/factory.go` (currently does `processor.ProjectName(project.Name(cfg.ProjectName))` chain — becomes `project.Resolve(cfg.ProjectName)` returning `project.Name` directly)
- `pkg/failurehandler/handler.go`, `pkg/promptresumer/resumer.go`, `pkg/runner/runner.go`, `pkg/review/*.go` — accept `project.Name` instead of `string` (their constructors and local-interface signatures both update). Structural typing means the change propagates automatically; just update declared types.

After the change, no package converts back and forth — `project.Name` flows from factory through every constructor unchanged. Use `.String()` only at true boundaries (logging, `exec.Command` args, docker name strings).

## 3. Inject `queuescanner.Scanner` via constructor

In `pkg/processor/processor.go`:

- Add `queueScanner queuescanner.Scanner` as a `NewProcessor` parameter (place adjacent to other service params per the established services-first convention)
- Remove the `SetScanner` method
- Remove the nil check + panic in `Process` and elsewhere — the constructor now guarantees non-nil
- Remove the comment "Call SetScanner before invoking Process — the processor panics if queueScanner is nil"

In `pkg/factory/factory.go::CreateProcessor` — resolve the value-level cycle (scanner holds proc; proc holds scanner) using a **lazy-forwarder pattern contained inside the factory**:

```go
// Define once at the top of factory.go (or in an internal helper file).
// Implements queuescanner.PromptProcessor and forwards to a back-reference
// set after the processor is constructed.
type lazyPromptProcessor struct {
    inner queuescanner.PromptProcessor
}

func (l *lazyPromptProcessor) ProcessPrompt(ctx context.Context, pr prompt.Prompt) error {
    return l.inner.ProcessPrompt(ctx, pr)
}
```

Wiring:

```go
// Two-phase wiring stays in factory; processor's public API is one-phase.
ppForwarder := &lazyPromptProcessor{}
scanner := queuescanner.NewScanner(promptManager, ppForwarder, failureHandler, completedDir)
proc := processor.NewProcessor(..., scanner, ...)  // one-phase constructor
ppForwarder.inner = proc                            // close the loop
```

After this:
- `processor.NewProcessor` takes `scanner` as a normal parameter; no `SetScanner`
- The lazy back-reference lives in `pkg/factory` (where wiring concerns belong)
- The processor's public API is single-phase: nil-check + panic at runtime → gone
- Remove the existing `SetScanner` invocation (around line 867 of `factory.go`)

## 4. Delete `workflowExecutorResumerAdapter`

In `pkg/factory/factory.go`:

- Delete the `workflowExecutorResumerAdapter` struct, its constructor, and both methods (`ReconstructState`, `Complete`)
- Where the adapter was passed to `promptresumer.NewResumer`, pass the real `processor.WorkflowExecutor` directly
- Update `pkg/promptresumer/resumer.go`'s local `WorkflowExecutor` interface to use `prompt.BaseName` (now that `BaseName` lives in `pkg/prompt`, the resumer can import the type without cycling on `pkg/processor`)

Verify the resumer's local interface signature matches the real `processor.WorkflowExecutor` after the type move:

```go
// pkg/promptresumer/resumer.go (local interface)
type WorkflowExecutor interface {
    ReconstructState(ctx context.Context, base prompt.BaseName, pf *prompt.PromptFile) (bool, error)
    Complete(ctx context.Context, ...) error
}
```

The structural match should be exact — `processor.WorkflowExecutor` will satisfy it without an adapter.

## 5. Tests

- `pkg/prompt/container_name_test.go` (moved from `pkg/processor/`) — same tests, adjusted package declaration
- All existing tests in `pkg/processor/`, `pkg/factory/`, `pkg/promptresumer/`, `pkg/failurehandler/`, `pkg/runner/`, `pkg/review/` continue to pass after the type updates
- No new tests required — pure refactor

If any test depended on the `lazyPromptProcessor` or the `SetScanner` shape, update it minimally to reflect the new wiring. Constructor tests should now pass `scanner` directly.

## 6. CHANGELOG

Append to `## Unreleased` in `CHANGELOG.md`:

```
- refactor: relocate BaseName/ContainerName from pkg/processor to pkg/prompt; consolidate ProjectName as pkg/project.Name; inject queuescanner.Scanner via NewProcessor constructor (eliminates SetScanner two-phase init); delete workflowExecutorResumerAdapter from factory (no longer needed once BaseName lives in pkg/prompt)
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
- Pure refactor — no behaviour change. All existing tests must pass unchanged.
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Search ALL `processor.NewProcessor(` call sites with `grep -rn "processor\.NewProcessor(" --include='*.go'` and update every one (recurring lesson from prior prompts: helper functions don't always cover all direct constructor calls in tests)
- Generated code (mocks/) is regenerated via `make generate` — do not hand-edit
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new errors (unlikely needed in a pure relocation)
- `pkg/project` is a leaf package — must not import `pkg/processor` or `pkg/prompt`
- `pkg/prompt` may not import `pkg/processor` (existing constraint; check after type moves)
- The lazy-PromptProcessor forwarder lives in `pkg/factory` only — do NOT put it in `pkg/processor` (that would re-introduce the API smell)
- Types belong in the lowest package that defines them; we are moving DOWN, not adding new packages
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
cd /workspace

# Types relocated
grep -n "type BaseName\|type ContainerName" pkg/prompt/
! grep -rn "type BaseName\|type ContainerName" pkg/processor/

# ProjectName consolidated
! grep -rn "processor\.ProjectName\|type ProjectName" pkg/processor/ pkg/factory/
grep -n "type Name string\|type Name = string" pkg/project/

# Resolver renamed (function call sites updated)
grep -n "func Resolve" pkg/project/name.go
! grep -rn "project\.Name(" --include='*.go' pkg/factory/ pkg/cmd/ main.go

# SetScanner gone
! grep -rn "SetScanner" pkg/processor/ pkg/factory/

# Adapter gone
! grep -n "workflowExecutorResumerAdapter" pkg/factory/factory.go

# All NewProcessor call sites still compile (no count assertion — covered by make precommit)
grep -rn "processor\.NewProcessor(" --include='*.go'

# No new processor → prompt cycle
! grep -rn "github.com/bborbe/dark-factory/pkg/processor" pkg/prompt/
```
</verification>
