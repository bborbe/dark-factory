---
status: completed
spec: [052-split-prompt-manager]
summary: Removed the 20-method prompt.Manager god interface, exported the concrete Manager struct, updated NewManager to return *Manager, renamed all method receivers from *manager to *Manager, deleted mocks/prompt-manager.go, and updated factory.go to pass *prompt.Manager throughout.
container: dark-factory-308-spec-052-remove-god-interface
dark-factory-version: v0.111.2
created: "2026-04-16T19:53:47Z"
queued: "2026-04-16T21:01:46Z"
started: "2026-04-16T23:04:18Z"
completed: "2026-04-16T23:09:23Z"
---

<summary>
- The `Manager` interface is removed from `pkg/prompt/prompt.go`; the private `manager` struct is renamed to `Manager` (exported concrete type)
- `NewManager` return type changes from `Manager` (interface) to `*Manager` (concrete pointer)
- All `(pm *manager)` receiver methods become `(pm *Manager)` receivers
- The counterfeiter directive for the old god interface is deleted; `mocks/prompt-manager.go` is deleted
- `pkg/factory/factory.go` changes `createPromptManager` to return `*prompt.Manager` instead of `prompt.Manager`; all factory consumers receive `*prompt.Manager` directly
- `pkg/prompt/prompt_test.go` is updated: `prompt.Manager` type annotations become `*prompt.Manager`
- `make precommit` passes
</summary>

<objective>
Remove the 20-method god interface from `pkg/prompt`, export the concrete struct, update the factory to pass `*prompt.Manager`, and delete the now-unused `mocks/prompt-manager.go`. After prompts 1 and 2, every consumer already accepts its own narrow interface that `*prompt.Manager` satisfies by structural typing — this prompt completes the refactoring.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`

**Precondition:** Prompts 1 and 2 (1-spec-052-consumer-interfaces and 2-spec-052-update-consumers) have already been executed. The following are in place:
- Every consumer package (`pkg/processor`, `pkg/runner`, `pkg/server`, `pkg/status`, `pkg/review`, `pkg/watcher`, `pkg/cmd`) has a local `PromptManager` interface and uses it in its constructor and struct field
- Every consumer test uses the package-specific fake (e.g., `mocks.ProcessorPromptManager`)
- The factory (`pkg/factory/factory.go`) still passes `prompt.Manager` (the wide interface) to consumers — this must change in this prompt
- `mocks/prompt-manager.go` still exists — it must be deleted in this prompt

Files to read before editing:
- `pkg/prompt/prompt.go` — the full file (Manager interface at ~line 479, manager struct at ~line 524, all receivers)
- `pkg/prompt/prompt_test.go` — find `prompt.Manager` type usages near `HasQueuedPromptsOnBranch` tests
- `pkg/factory/factory.go` — `createPromptManager` function (~lines 119–134) and all call sites that receive its return value
- `mocks/prompt-manager.go` — confirm it only contains the wide Manager fake before deleting
</context>

<requirements>

## 1. Update `pkg/prompt/prompt.go`

### 1a. Remove the `Manager` interface and its counterfeiter directive

Delete the following block entirely (approximately lines 479–505):
```go
//counterfeiter:generate -o ../../mocks/prompt-manager.go --fake-name Manager . Manager

// Manager manages prompt file operations.
type Manager interface {
    ResetExecuting(ctx context.Context) error
    ResetFailed(ctx context.Context) error
    HasExecuting(ctx context.Context) bool
    ListQueued(ctx context.Context) ([]Prompt, error)
    Load(ctx context.Context, path string) (*PromptFile, error)
    ReadFrontmatter(ctx context.Context, path string) (*Frontmatter, error)
    SetStatus(ctx context.Context, path string, status string) error
    SetContainer(ctx context.Context, path string, name string) error
    SetVersion(ctx context.Context, path string, version string) error
    SetPRURL(ctx context.Context, path string, url string) error
    SetBranch(ctx context.Context, path string, branch string) error
    IncrementRetryCount(ctx context.Context, path string) error
    Content(ctx context.Context, path string) (string, error)
    Title(ctx context.Context, path string) (string, error)
    MoveToCompleted(ctx context.Context, path string) error
    NormalizeFilenames(ctx context.Context, dir string) ([]Rename, error)
    AllPreviousCompleted(ctx context.Context, n int) bool
    FindMissingCompleted(ctx context.Context, n int) []int
    FindPromptStatusInProgress(ctx context.Context, number int) string
    // HasQueuedPromptsOnBranch returns true if any queued prompt (other than excludePath)
    // has the given branch value in its frontmatter.
    HasQueuedPromptsOnBranch(ctx context.Context, branch string, excludePath string) (bool, error)
}
```

### 1b. Update `NewManager` and the concrete struct

Change the `NewManager` function return type from `Manager` to `*Manager`:
```go
// Before:
func NewManager(
    inboxDir string,
    inProgressDir string,
    completedDir string,
    mover FileMover,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) Manager {
    return &manager{

// After:
func NewManager(
    inboxDir string,
    inProgressDir string,
    completedDir string,
    mover FileMover,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) *Manager {
    return &Manager{
```

### 1c. Rename the `manager` struct to `Manager`

Change the struct declaration:
```go
// Before:
// manager implements Manager.
type manager struct {

// After:
// Manager manages prompt file operations.
type Manager struct {
```

### 1d. Update all method receivers

There are 20 methods with receiver `(pm *manager)`. Replace ALL occurrences of `(pm *manager)` with `(pm *Manager)`.

Run this to find all of them first:
```bash
grep -n "(pm \*manager)" pkg/prompt/prompt.go
```

Use a global replace — do NOT miss any method. Affected methods include:
`ResetExecuting`, `ResetFailed`, `HasExecuting`, `ListQueued`, `Load`, `ReadFrontmatter`, `SetStatus`, `SetContainer`, `SetVersion`, `SetPRURL`, `SetBranch`, `IncrementRetryCount`, `Content`, `Title`, `MoveToCompleted`, `NormalizeFilenames`, `AllPreviousCompleted`, `FindMissingCompleted`, `FindPromptStatusInProgress`, `HasQueuedPromptsOnBranch`.

Note: `SetContainer`, `SetVersion`, `SetBranch`, `Content`, `ResetExecuting`, `ResetFailed` are no longer part of any consumer interface — but they remain accessible on `*Manager` (callers can still call them directly on the concrete struct if needed, maintaining backward compatibility per the spec constraint). Do NOT delete these methods.

### 1e. Update struct literal in `NewManager`

Change `&manager{` to `&Manager{` (already done in step 1b above — just double-check the struct field names are unchanged).

## 2. Update `pkg/prompt/prompt_test.go`

Find the test that uses `prompt.Manager` as a type annotation:
```bash
grep -n "prompt\.Manager" pkg/prompt/prompt_test.go
```

For each occurrence like:
```go
var mgr prompt.Manager
mgr = prompt.NewManager(...)
```
Change to either:
```go
mgr := prompt.NewManager(...)
```
or:
```go
var mgr *prompt.Manager
mgr = prompt.NewManager(...)
```

The simpler `:=` form is preferred. Verify the test still compiles by reading the surrounding context.

## 3. Delete `mocks/prompt-manager.go`

```bash
rm /workspace/mocks/prompt-manager.go
```

Before deleting, confirm no remaining test file uses `mocks.Manager`:
```bash
grep -rn "mocks\.Manager" pkg/
```
This must return zero matches (should have been cleaned up in prompt 2). If any matches remain, fix those files before deleting.

## 4. Update `pkg/factory/factory.go`

### 4a. Update `createPromptManager`

Change the return type from `prompt.Manager` to `*prompt.Manager`:
```go
// Before:
func createPromptManager(
    inboxDir string,
    inProgressDir string,
    completedDir string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (prompt.Manager, git.Releaser) {
    releaser := git.NewReleaser()
    promptManager := prompt.NewManager(
        inboxDir,
        inProgressDir,
        completedDir,
        releaser,
        currentDateTimeGetter,
    )
    return promptManager, releaser
}

// After:
func createPromptManager(
    inboxDir string,
    inProgressDir string,
    completedDir string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (*prompt.Manager, git.Releaser) {
    releaser := git.NewReleaser()
    promptManager := prompt.NewManager(
        inboxDir,
        inProgressDir,
        completedDir,
        releaser,
        currentDateTimeGetter,
    )
    return promptManager, releaser
}
```

### 4b. Update all call sites of `createPromptManager` in factory.go

Find them:
```bash
grep -n "createPromptManager\|promptManager prompt\." pkg/factory/factory.go
```

Any variable declared as `prompt.Manager` that receives the return of `createPromptManager` must change to `*prompt.Manager`. The variable name stays the same.

For example:
```go
// Before:
promptManager, releaser := createPromptManager(...)
// OR if typed explicitly:
var promptManager prompt.Manager
promptManager, releaser = createPromptManager(...)

// After:
promptManager, releaser := createPromptManager(...)
// (short declaration already infers *prompt.Manager; no explicit type needed)
```

If the variable is typed explicitly as `prompt.Manager` anywhere in factory.go, change it to `*prompt.Manager`.

### 4c. Verify all consumer constructors accept `*prompt.Manager`

After this change, the factory passes `*prompt.Manager` to each consumer constructor. Each consumer constructor now takes its local `PromptManager` interface. Since `*prompt.Manager` implements all methods declared by each narrow `PromptManager` interface (the `*Manager` struct has all 20 methods, covering every narrow interface's subset), this compiles via structural typing.

Run:
```bash
cd /workspace && go build ./pkg/factory/...
```

If compilation fails, read the error and fix it. Common causes:
- A function in factory.go that explicitly types a parameter as `prompt.Manager` (not `*prompt.Manager`) — fix by updating the parameter type or removing the explicit type annotation
- A function elsewhere that accepts `prompt.Manager` (the now-deleted interface) — grep for remaining usages: `grep -rn "prompt\.Manager" pkg/`

## 5. Clean up any remaining `prompt.Manager` references

```bash
grep -rn "prompt\.Manager" pkg/
```

This should return zero matches. If any remain:
- `pkg/prompt/prompt.go` — impossible since we removed the interface
- Any other file — read it and change `prompt.Manager` to `*prompt.Manager` (if it's a variable/parameter type) or to the local `PromptManager` interface (if it's a consumer that should be using its local interface)

## 6. Write CHANGELOG entry

Check if `CHANGELOG.md` has an `## Unreleased` section. If not, add one. Append:

```
- refactor: replace 20-method prompt.Manager god interface with per-consumer narrow interfaces at point of use
```

## 7. Run `make precommit`

```bash
cd /workspace && make precommit
```

Must exit 0. If it fails, fix the failing target (`make lint`, `make gosec`, etc.) one at a time, then re-run `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- The 20 methods on the concrete `*Manager` struct must ALL be preserved — methods like `SetContainer`, `SetVersion`, `SetBranch`, `Content`, `ResetExecuting`, `ResetFailed` are not on any consumer interface but must remain accessible on the concrete type per the spec constraint: "pkg/prompt's public API does not shrink in a way that breaks any current caller — methods stay accessible on the concrete struct even if the god interface is gone"
- Do NOT rename any method or change any method signature — only change receiver type from `*manager` to `*Manager`
- Do NOT add any new behavior or logic
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Wrap all non-nil errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors` (no new error paths expected in this prompt)
- After deleting `mocks/prompt-manager.go`, verify `mocks/mocks.go` does not reference `Manager` — if it does, remove that reference
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -rn "prompt\.Manager" pkg/` — zero matches (god interface is gone)
2. `grep -rn "mocks\.Manager" pkg/` — zero matches (old wide fake is gone)
3. `ls mocks/prompt-manager.go 2>&1` — "No such file or directory" (file was deleted)
4. `grep -n "type Manager struct" pkg/prompt/prompt.go` — one match (exported concrete struct)
5. `grep -n "func NewManager" pkg/prompt/prompt.go` — returns `*Manager`
6. `grep -n "createPromptManager" pkg/factory/factory.go` — return type is `*prompt.Manager`
7. `grep -rn "type Manager interface" pkg/prompt/` — zero matches (interface is gone)
8. `go test ./pkg/prompt/...` — passes (prompt_test.go compiles with `*prompt.Manager`)
9. `grep -rh "type.*interface {" pkg/prompt/prompt.go | wc -l` — should be 0 or very small (no god interface)
</verification>
