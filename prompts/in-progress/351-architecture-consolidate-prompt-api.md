---
status: committing
container: dark-factory-351-architecture-consolidate-prompt-api
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T22:42:00Z"
queued: "2026-04-25T20:40:48Z"
started: "2026-04-25T20:40:50Z"
---

<summary>
- Unexport ~20 free functions in `pkg/prompt` whose only callers (after this prompt) are `pkg/prompt`'s own `Manager` methods — they become package-private helpers
- Make `prompt.Manager` the sole public API of `pkg/prompt` (besides constructors and value types like `PromptFile`, `Frontmatter`, `Prompt`, `Status`, `Rename`)
- Extend `pkg/cmd/prompt_manager.go::PromptManager` to expose all methods the cmd package uses, so cmd code stops calling free functions directly
- Update every external caller of `prompt.Load`, `prompt.SetStatus`, `prompt.MoveToCompleted`, etc. to call the same method on the injected `PromptManager`
- Pure refactor — no behaviour change; all existing tests pass unchanged
</summary>

<objective>
Eliminate the dual API surface in `pkg/prompt` (40+ exported free functions ↔ ~20 Manager methods that delegate to them) by unexporting the free functions and routing all external code through `Manager`. Reduces the public surface, prevents callers from bypassing Manager's invariants (timestamp injection, dir scoping), and lets future mutation logic live in one place.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Today's relevant code state (post-350):
- `pkg/prompt/prompt.go` defines ~40 exported free functions (e.g. `Load`, `ListQueued`, `ResetExecuting`, `SetStatus`, `MoveToCompleted`, `NormalizeFilenames`) AND ~20 `Manager` methods that mostly delegate to them
- The `Manager` already holds `inProgressDir`, `completedDir`, and `currentDateTimeGetter`, so it has everything needed to call each free function
- External callers (mostly in `pkg/cmd/`, plus other packages like `pkg/processor/`) call the free functions directly with their own `currentDateTimeGetter` and explicit dirs

Examples of external use to convert (all in `pkg/cmd/`):

```bash
grep -rn "prompt\.Load\b\|prompt\.SetStatus\b\|prompt\.ListQueued\b\|prompt\.ResetExecuting\b\|prompt\.MoveToCompleted\b\|prompt\.HasExecuting\b\|prompt\.IncrementRetryCount\b\|prompt\.NormalizeFilenames\b\|prompt\.SetContainer\b\|prompt\.SetVersion\b\|prompt\.SetPRURL\b\|prompt\.SetBranch\b\|prompt\.ReadFrontmatter\b\|prompt\.AllPreviousCompleted\b\|prompt\.FindMissingCompleted\b\|prompt\.FindPromptStatus\b\|prompt\.FindCommitting\b\|prompt\.Title\b\|prompt\.Content\b\|prompt\.ResetFailed\b" --include='*.go' | grep -v "_test\.go" | grep -v "pkg/prompt/"
```

Manager equivalents that already exist (verified):
- `Manager.Load`, `ResetExecuting`, `ResetFailed`, `HasExecuting`, `ListQueued`, `FindCommitting`, `ReadFrontmatter`, `SetStatus`, `SetContainer`, `SetVersion`, `SetPRURL`, `SetBranch`, `IncrementRetryCount`, `Content`, `Title`, `MoveToCompleted`, `NormalizeFilenames`, `AllPreviousCompleted`, `FindMissingCompleted`, `FindPromptStatusInProgress`, `HasQueuedPromptsOnBranch`

`pkg/cmd/prompt_manager.go::PromptManager` currently only declares 2 methods (`NormalizeFilenames`, `MoveToCompleted`) — needs expansion.

Files in scope (locate by symbol; line numbers churn):
- `pkg/prompt/prompt.go` — the ~1428-line hub holding both APIs
- `pkg/prompt/prompt_file_test.go`, `pkg/prompt/prompt_test.go`, etc. — tests that may call free functions directly (test code can keep calling them via in-package access since renaming makes them lowercase still in-package)
- `pkg/cmd/prompt_manager.go` — local interface to extend
- `pkg/cmd/*.go` — every command that calls a `prompt.X` free function (15+ files)
- `pkg/processor/*.go`, `pkg/promptresumer/*.go`, `pkg/failurehandler/*.go`, `pkg/queuescanner/*.go`, etc. — check each for free-function usage
</context>

<requirements>

## 1. Survey before editing

```bash
cd /workspace
grep -rn "prompt\.[A-Z]" --include='*.go' \
  | grep -v "_test\.go" \
  | grep -v "pkg/prompt/" \
  | grep -v "prompt\.PromptFile\|prompt\.Frontmatter\|prompt\.Prompt\b\|prompt\.Status\|prompt\.Rename\|prompt\.NewManager\|prompt\.NewPromptFile\|prompt\.StripNumberPrefix\|prompt\.Counter\|prompt\.NewCounter\|prompt\.FileMover\|prompt\.SpecList" \
  | sort -u
```

The output is the list of free-function call sites that need to be converted. Save this list (mental note or comment) — you'll re-run the same grep at the end and assert it's empty (apart from the value-type whitelist).

Value types and constructors stay public — they are not in scope for unexport:
- Value types: `PromptFile`, `Frontmatter`, `Prompt`, `Status`, `PromptStatus` (and all `*PromptStatus` constants), `Rename`, `BaseName`, `ContainerName`, `AvailablePromptStatuses`, `PromptStatuses`
- Other exported types and their constructors: `Counter`, `NewCounter`, `FileMover`, `SpecList`
- Constructors: `NewManager`, `NewPromptFile`
- Pure utilities: `StripNumberPrefix` (no side effects, no `currentDateTimeGetter` dependency)
- Sentinels: `ErrEmptyPrompt` and any other `Err*`

## 2. Extend `pkg/cmd/prompt_manager.go::PromptManager` interface

Add every method the cmd package will call after the conversion. Inspect each `pkg/cmd/*.go` file and list the methods used. Expected surface:

```go
type PromptManager interface {
    Load(ctx context.Context, path string) (*prompt.PromptFile, error)
    NormalizeFilenames(ctx context.Context, dir string) ([]prompt.Rename, error)
    MoveToCompleted(ctx context.Context, path string) error
    SetStatus(ctx context.Context, path string, status string) error
    ListQueued(ctx context.Context) ([]prompt.Prompt, error)
    FindCommitting(ctx context.Context) ([]string, error)
    ReadFrontmatter(ctx context.Context, path string) (*prompt.Frontmatter, error)
    HasExecuting(ctx context.Context) bool
    HasQueuedPromptsOnBranch(ctx context.Context, branch string) bool
    AllPreviousCompleted(ctx context.Context, n int) bool
    FindMissingCompleted(ctx context.Context, n int) []int
    FindPromptStatusInProgress(ctx context.Context, number int) string
    Title(ctx context.Context, path string) (string, error)
    Content(ctx context.Context, path string) (string, error)
    ResetExecuting(ctx context.Context) error
    ResetFailed(ctx context.Context) error
    IncrementRetryCount(ctx context.Context, path string) error
    SetContainer(ctx context.Context, path string, name string) error
    SetVersion(ctx context.Context, path string, version string) error
    SetPRURL(ctx context.Context, path string, url string) error
    SetBranch(ctx context.Context, path string, branch string) error
}
```

Match the actual `prompt.Manager` method signatures exactly (including param types from prompt 350 — e.g. `prompt.BaseName`, `prompt.ContainerName` — wherever applicable). Verify by reading `pkg/prompt/prompt.go`'s Manager method block.

If a method exists on `prompt.Manager` but isn't called by cmd, leave it off the interface (small interface principle). Add it later if a caller materialises.

Regenerate the counterfeiter mock: `make generate`.

## 3. Convert external callers in `pkg/cmd/*.go`

For each call site found in step 1's grep, replace `prompt.X(ctx, args, m.currentDateTimeGetter)` with `m.promptManager.X(ctx, simplifiedArgs)`.

Examples:

```go
// before
pf, err := prompt.Load(ctx, path, a.currentDateTimeGetter)

// after (Manager carries the getter)
pf, err := a.promptManager.Load(ctx, path)
```

```go
// before
if err := prompt.SetStatus(ctx, path, "approved", a.currentDateTimeGetter); err != nil { ... }

// after
if err := a.promptManager.SetStatus(ctx, path, "approved"); err != nil { ... }
```

If a cmd struct doesn't currently have `promptManager` injected, add it via the constructor, mirroring the pattern of cmd structs that already have it (e.g. `NewApproveCommand`).

Update the corresponding `Create*Command` factory function in `pkg/factory/factory.go` to pass the existing `promptManager`.

If a cmd struct stops needing `currentDateTimeGetter` directly after the conversion, drop the field and parameter.

## 4. Special case: `prompt.Load`

`Load` is currently exported and used very widely. Two viable paths:

- **Path A (preferred):** unexport `Load` to `load`. Every external caller switches to `Manager.Load`.
- **Path B:** keep `Load` exported because of its broad use. This is a pragmatic compromise but leaves the dual-API smell.

**Pick Path A.** All cmd files already have promptManager available; the conversion is mechanical.

## 5. Convert callers in other packages

After cmd, repeat the survey for non-cmd packages. Confirmed callers of `prompt.Load` (and likely other free functions):

- `pkg/generator/` — 3 sites
- `pkg/server/queue_helpers.go`
- `pkg/slugmigrator/migrator.go`
- `pkg/spec/spec.go`
- `pkg/watcher/watcher.go`
- `pkg/reindex/specref.go`
- `pkg/processor/` (in extracted files) — route through `processor.PromptManager` local interface
- `pkg/promptresumer/`, `pkg/failurehandler/`, `pkg/queuescanner/`, `pkg/committingrecoverer/`, `pkg/promptenricher/`, `pkg/specsweeper/` — these define their own local `PromptManager` interfaces (structural-typing trick from prior prompts); expand each interface as needed and add the matching method calls

Local `PromptManager` interfaces are declared in 9+ packages: `cmd`, `processor`, `review`, `runner`, `server`, `status`, `watcher`, `committingrecoverer`, `failurehandler`, `promptresumer`, `queuescanner`. Each needs the relevant method(s) added if they're used in that package. Run `make generate` after extending interfaces so counterfeiter regenerates all impacted mocks in one pass.

## 6. Unexport the free functions in `pkg/prompt/prompt.go`

Once all external callers are converted, rename the following exported free functions to lowercase (in-package only):

- `ResetExecuting` → `resetExecuting`
- `ResetFailed` → `resetFailed`
- `HasExecuting` → `hasExecuting`
- `ListQueued` → `listQueued`
- `FindCommitting` → `findCommitting`
- `Load` → `load`
- `ReadFrontmatter` → `readFrontmatter`
- `SetStatus` → `setStatus`
- `SetContainer` → `setContainer`
- `SetVersion` → `setVersion`
- `SetPRURL` → `setPRURL`
- `SetBranch` → `setBranch`
- `IncrementRetryCount` → `incrementRetryCount`
- `Content` → `content`
- `Title` → `title`
- `MoveToCompleted` → `moveToCompleted`
- `NormalizeFilenames` → `normalizeFilenames`
- `AllPreviousCompleted` → `allPreviousCompleted`
- `FindMissingCompleted` → `findMissingCompleted`
- `FindPromptStatus` → `findPromptStatus` (note: free function `FindPromptStatus(ctx, dir, n)` is what `Manager.FindPromptStatusInProgress` delegates to — only the free function is renamed; the Manager method name stays the same)

Update every reference inside `pkg/prompt/` (Manager methods + tests inside `package prompt` if any). External tests in `package prompt_test` cannot call the unexported names — convert those to use `Manager` too.

## 7. Verify

```bash
cd /workspace
make generate
make precommit
```

Both must exit 0.

Re-run the survey from step 1 — should yield ZERO results (all `prompt.X` calls outside `pkg/prompt/` are now value-type references only):

```bash
cd /workspace
grep -rn "prompt\.[A-Z]" --include='*.go' \
  | grep -v "_test\.go" \
  | grep -v "pkg/prompt/" \
  | grep -v "prompt\.PromptFile\|prompt\.Frontmatter\|prompt\.Prompt\b\|prompt\.Status\|prompt\.Rename\|prompt\.NewManager\|prompt\.NewPromptFile\|prompt\.StripNumberPrefix\|prompt\.Counter\|prompt\.NewCounter\|prompt\.FileMover\|prompt\.SpecList\|prompt\.BaseName\|prompt\.ContainerName\|prompt\.AvailablePromptStatuses\|prompt\.PromptStatuses\|prompt\.ErrEmptyPrompt\|prompt\.[A-Za-z]*PromptStatus\b"
```

Empty output = success.

## 8. CHANGELOG

Append to `## Unreleased` in `CHANGELOG.md`:

```
- refactor: pkg/prompt API consolidation — unexport ~20 free-function helpers; prompt.Manager is now the sole public mutation API; cmd packages use promptManager.X instead of prompt.X(...) free-function calls; eliminates dual-API surface
```

</requirements>

<constraints>
- Pure refactor — no behaviour change. All existing tests must pass unchanged.
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Value types, constructors, and pure utilities stay exported (`PromptFile`, `Frontmatter`, `Prompt`, `Status`, `Rename`, `BaseName`, `ContainerName`, `NewManager`, `NewPromptFile`, `StripNumberPrefix`, `ErrEmptyPrompt`, `AvailablePromptStatuses`, `*PromptStatus` constants)
- `pkg/prompt/` must not import `pkg/processor` or any other higher-level package (existing constraint)
- Local `PromptManager` interfaces in consumer packages (cmd, processor, etc.) follow the small-interface principle — only declare the methods that package actually calls
- Search ALL non-test, non-pkg/prompt callers; do NOT leave any `prompt.<UpperCase>` free-function calls outside the value-type whitelist
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new errors (unlikely needed)
- Counterfeiter mocks regenerated via `make generate` — do not hand-edit
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
cd /workspace

# Free functions unexported (no exported function with these specific names)
! grep -n "^func ResetExecuting\|^func ResetFailed\|^func HasExecuting\|^func ListQueued\|^func FindCommitting\|^func Load\b\|^func ReadFrontmatter\|^func SetStatus\|^func SetContainer\|^func SetVersion\|^func SetPRURL\|^func SetBranch\|^func IncrementRetryCount\|^func Content\|^func Title\|^func MoveToCompleted\|^func NormalizeFilenames\|^func AllPreviousCompleted\|^func FindMissingCompleted\|^func FindPromptStatus" pkg/prompt/prompt.go

# Lowercase versions exist
grep -n "^func resetExecuting\|^func load\b\|^func setStatus\|^func moveToCompleted" pkg/prompt/prompt.go

# No external callers of unexported names (whitelist value types)
grep -rn "prompt\.[A-Z]" --include='*.go' \
  | grep -v "_test\.go" \
  | grep -v "pkg/prompt/" \
  | grep -v "prompt\.PromptFile\|prompt\.Frontmatter\|prompt\.Prompt\b\|prompt\.Status\|prompt\.Rename\|prompt\.NewManager\|prompt\.NewPromptFile\|prompt\.StripNumberPrefix\|prompt\.Counter\|prompt\.NewCounter\|prompt\.FileMover\|prompt\.SpecList\|prompt\.BaseName\|prompt\.ContainerName\|prompt\.AvailablePromptStatuses\|prompt\.PromptStatuses\|prompt\.ErrEmptyPrompt\|prompt\.[A-Za-z]*PromptStatus\b"
# Should produce empty output

# cmd PromptManager interface expanded
wc -l pkg/cmd/prompt_manager.go
# Expect ~25-30 lines (was 20; +20+ method signatures)
```
</verification>
