---
status: draft
spec: [075-bug-prompt-cancel-leaves-running-container-and-file]
created: "2026-05-06T09:20:00Z"
branch: dark-factory/bug-prompt-cancel-leaves-running-container-and-file
---

<summary>
- A new `cancelledDir` config field is added so projects have a dedicated `prompts/cancelled/` directory for cancelled prompt files
- `prompt.Manager` gains a `MoveToCancelled` method that marks the file cancelled (with timestamp) and moves it atomically, mirroring the existing `MoveToCompleted` pattern
- The `cancelled` frontmatter timestamp field is introduced so cancelled prompt files record when they were cancelled
- `listQueued` is updated to skip files with `status: cancelled`, preventing the daemon from picking up cancelled files even if they remain in `in-progress/` during the brief window before the move
- Both the `cmd.PromptManager` and `processor.PromptManager` interfaces gain `MoveToCancelled`, so downstream wiring in prompt 2 compiles
- All `createPromptManager` call sites in `factory.go` are updated to pass `cfg.Prompts.CancelledDir`, keeping the factory in sync with the new constructor signature
- Counterfeiter mocks for both interface changes are regenerated
- New tests confirm `MoveToCancelled` creates the destination directory, writes the correct status and timestamp, and moves the file
</summary>

<objective>
Build the infrastructure needed to move cancelled prompt files to `prompts/cancelled/`: add the config field, implement `MoveToCancelled` on the prompt Manager, add the `cancelled` timestamp to `Frontmatter`, and update all interfaces and factory call sites so everything compiles. This prompt delivers the foundation; prompt 2 wires it into the CLI cancel command and processor.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter, no bare `return err`, no `fmt.Errorf`).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read before editing:
- `pkg/config/config.go` — `PromptsConfig` struct and `Defaults()` function
- `pkg/prompt/prompt.go` — `Frontmatter` struct, `MarkCancelled()`, `NewManager`, `Manager` struct, `moveToCompleted` (the pattern to follow for `moveToCancelled`), `listQueued` (the skip list to update)
- `pkg/cmd/prompt_manager.go` — `PromptManager` interface to extend
- `pkg/processor/prompt_manager.go` — `PromptManager` interface to extend
- `pkg/factory/factory.go` — `createPromptManager` helper and all its call sites
- `pkg/prompt/prompt_manager_test.go` — existing test pattern for Manager methods
- `pkg/prompt/prompt_file_test.go` — existing test pattern for PromptFile methods
</context>

<requirements>

## 1. Add `CancelledDir` to `PromptsConfig` in `pkg/config/config.go`

In the `PromptsConfig` struct, add `CancelledDir` immediately after `RejectedDir`:

```go
type PromptsConfig struct {
    InboxDir      string `yaml:"inboxDir"`
    InProgressDir string `yaml:"inProgressDir"`
    CompletedDir  string `yaml:"completedDir"`
    RejectedDir   string `yaml:"rejectedDir"`
    CancelledDir  string `yaml:"cancelledDir"`
    LogDir        string `yaml:"logDir"`
}
```

In `Defaults()`, add the default value for `CancelledDir` inside the `Prompts` initializer (immediately after `RejectedDir`):

```go
Prompts: PromptsConfig{
    InboxDir:      "prompts",
    InProgressDir: "prompts/in-progress",
    CompletedDir:  "prompts/completed",
    RejectedDir:   "prompts/rejected",
    CancelledDir:  "prompts/cancelled",
    LogDir:        "prompts/log",
},
```

Do NOT add a validation rule for `CancelledDir` in `Validate()` — it is optional and new projects without an explicit config entry will use the default.

## 2. Add `cancelled` timestamp to `Frontmatter` in `pkg/prompt/prompt.go`

In the `Frontmatter` struct, add the `Cancelled` timestamp field immediately after `Rejected`:

```go
type Frontmatter struct {
    Status             string   `yaml:"status"`
    Specs              SpecList `yaml:"spec,omitempty,flow"`
    Summary            string   `yaml:"summary,omitempty"`
    Container          string   `yaml:"container,omitempty"`
    DarkFactoryVersion string   `yaml:"dark-factory-version,omitempty"`
    Created            string   `yaml:"created,omitempty"`
    Queued             string   `yaml:"queued,omitempty"`
    Started            string   `yaml:"started,omitempty"`
    Completed          string   `yaml:"completed,omitempty"`
    PRURL              string   `yaml:"pr-url,omitempty"`
    Branch             string   `yaml:"branch,omitempty"`
    Issue              string   `yaml:"issue,omitempty"`
    RetryCount         int      `yaml:"retryCount,omitempty"`
    LastFailReason     string   `yaml:"lastFailReason,omitempty"`
    Rejected           string   `yaml:"rejected,omitempty"`
    RejectedReason     string   `yaml:"rejected_reason,omitempty"`
    Cancelled          string   `yaml:"cancelled,omitempty"`   // NEW: UTC ISO8601 timestamp
}
```

## 3. Update `MarkCancelled()` in `pkg/prompt/prompt.go`

Replace the current (no-op timestamp) `MarkCancelled`:

```go
// MarkCancelled sets status to cancelled with a UTC timestamp.
func (pf *PromptFile) MarkCancelled() {
    pf.Frontmatter.Cancelled = pf.now().UTC().Format(time.RFC3339)
    pf.Frontmatter.Status = string(CancelledPromptStatus)
}
```

The `pf.now()` helper already exists (look for how `MarkCompleted` or `MarkRejected` uses it — same pattern). Check what it returns before using it.

## 4. Add `cancelledDir` to `prompt.Manager` in `pkg/prompt/prompt.go`

### 4a. Update `NewManager` signature

Add `cancelledDir string` as the **fourth parameter** (after `completedDir`, before `mover`):

```go
func NewManager(
    inboxDir string,
    inProgressDir string,
    completedDir string,
    cancelledDir string,
    mover FileMover,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) *Manager {
    return &Manager{
        inboxDir:              inboxDir,
        inProgressDir:         inProgressDir,
        completedDir:          completedDir,
        cancelledDir:          cancelledDir,
        mover:                 mover,
        currentDateTimeGetter: currentDateTimeGetter,
    }
}
```

### 4b. Add `cancelledDir` field to `Manager` struct

Add `cancelledDir string` immediately after `completedDir string`:

```go
type Manager struct {
    inboxDir              string
    inProgressDir         string
    completedDir          string
    cancelledDir          string
    mover                 FileMover
    currentDateTimeGetter libtime.CurrentDateTimeGetter
}
```

## 5. Add `moveToCancelled` private function and `MoveToCancelled` method in `pkg/prompt/prompt.go`

Add the private function immediately after `moveToCompleted` (search for `// MoveToCompleted sets status to "completed"` to find its location):

```go
// moveToCancelled sets status to "cancelled" (with timestamp) and moves a prompt file to the cancelled directory.
func moveToCancelled(
    ctx context.Context,
    path string,
    cancelledDir string,
    mover FileMover,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
    pf, err := load(ctx, path, currentDateTimeGetter)
    if err != nil {
        return errors.Wrap(ctx, err, "load prompt")
    }

    pf.MarkCancelled()
    if err := pf.Save(ctx); err != nil {
        return errors.Wrap(ctx, err, "set cancelled status")
    }

    if err := os.MkdirAll(cancelledDir, 0750); err != nil {
        return errors.Wrap(ctx, err, "create cancelled directory")
    }

    filename := filepath.Base(path)
    dest := filepath.Join(cancelledDir, filename)

    slog.Debug("moving to cancelled", "from", path, "to", dest)

    if err := mover.MoveFile(ctx, path, dest); err != nil {
        return errors.Wrap(ctx, err, "move file")
    }

    return nil
}
```

Add the public method to `Manager` immediately after `MoveToCompleted`:

```go
// MoveToCancelled sets status to "cancelled" (with timestamp) and moves a prompt file to the cancelled/ subdirectory.
func (pm *Manager) MoveToCancelled(ctx context.Context, path string) error {
    return moveToCancelled(ctx, path, pm.cancelledDir, pm.mover, pm.currentDateTimeGetter)
}
```

## 6. Add `CancelledPromptStatus` to the skip list in `listQueued` in `pkg/prompt/prompt.go`

Find the `listQueued` private function. In the loop that skips files by status, add `CancelledPromptStatus` to the existing skip list:

```go
if fm.Status == string(ExecutingPromptStatus) ||
    fm.Status == string(CommittingPromptStatus) ||
    fm.Status == string(CompletedPromptStatus) ||
    fm.Status == string(FailedPromptStatus) ||
    fm.Status == string(InReviewPromptStatus) ||
    fm.Status == string(PendingVerificationPromptStatus) ||
    fm.Status == string(CancelledPromptStatus) {
    slog.Debug("skipping prompt", "file", entry.Name(), "status", fm.Status)
    continue
}
```

This is defense-in-depth: if a cancelled file somehow remains in `in-progress/`, it will never be picked up by the scanner.

## 7. Add `MoveToCancelled` to `cmd.PromptManager` interface in `pkg/cmd/prompt_manager.go`

```go
// PromptManager is the subset of prompt.Manager that the cmd package uses.
type PromptManager interface {
    Load(ctx context.Context, path string) (*prompt.PromptFile, error)
    NormalizeFilenames(ctx context.Context, dir string) ([]prompt.Rename, error)
    MoveToCompleted(ctx context.Context, path string) error
    MoveToCancelled(ctx context.Context, path string) error
}
```

## 8. Add `MoveToCancelled` to `processor.PromptManager` interface in `pkg/processor/prompt_manager.go`

```go
type PromptManager interface {
    ListQueued(ctx context.Context) ([]prompt.Prompt, error)
    Load(ctx context.Context, path string) (*prompt.PromptFile, error)
    AllPreviousCompleted(ctx context.Context, n int) bool
    FindMissingCompleted(ctx context.Context, n int) []int
    FindPromptStatusInProgress(ctx context.Context, number int) string
    SetStatus(ctx context.Context, path string, status string) error
    MoveToCompleted(ctx context.Context, path string) error
    MoveToCancelled(ctx context.Context, path string) error
    HasQueuedPromptsOnBranch(ctx context.Context, branch string, excludePath string) (bool, error)
    SetPRURL(ctx context.Context, path string, url string) error
    FindCommitting(ctx context.Context) ([]string, error)
}
```

## 9. Update `createPromptManager` in `pkg/factory/factory.go`

### 9a. Add `cancelledDir string` parameter

```go
func createPromptManager(
    inboxDir string,
    inProgressDir string,
    completedDir string,
    cancelledDir string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (*prompt.Manager, git.Releaser) {
    releaser := git.NewReleaser()
    promptManager := prompt.NewManager(
        inboxDir,
        inProgressDir,
        completedDir,
        cancelledDir,
        releaser,
        currentDateTimeGetter,
    )
    return promptManager, releaser
}
```

### 9b. Update ALL call sites of `createPromptManager`

There are ~15 call sites in `factory.go`. Find them all with:
```bash
grep -n "createPromptManager(" pkg/factory/factory.go
```

For each call site, pass `cfg.Prompts.CancelledDir` as the new fourth argument (after `cfg.Prompts.CompletedDir`, before `currentDateTimeGetter`).

Example pattern — every call that currently looks like:
```go
createPromptManager(
    cfg.Prompts.InboxDir,
    cfg.Prompts.InProgressDir,
    cfg.Prompts.CompletedDir,
    currentDateTimeGetter,
)
```
Must become:
```go
createPromptManager(
    cfg.Prompts.InboxDir,
    cfg.Prompts.InProgressDir,
    cfg.Prompts.CompletedDir,
    cfg.Prompts.CancelledDir,
    currentDateTimeGetter,
)
```

**Do not miss any call site** — skipping one will break compilation. After editing, run:
```bash
grep -c "createPromptManager" pkg/factory/factory.go
```
and verify every occurrence has been updated by confirming the file compiles (`make test`).

## 10. Regenerate counterfeiter mocks

After updating the interfaces, regenerate the mocks:
```bash
go generate ./pkg/cmd/...
go generate ./pkg/processor/...
```

Verify `mocks/cmd-prompt-manager.go` now has a `MoveToCancelled` method and `mocks/processor-prompt-manager.go` also has a `MoveToCancelled` method.

Also check if any other package or test references `prompt.NewManager` directly (not via factory) and update those call sites:
```bash
grep -rn "prompt\.NewManager(" --include="*.go" .
```
Update any test files that call `prompt.NewManager` directly to add the `cancelledDir` argument (e.g., pass `""` or a temp dir).

## 11. Add tests for `MoveToCancelled`

In `pkg/prompt/prompt_manager_test.go` (following the existing `Describe("MoveToCompleted", ...)` pattern), add a `Describe("MoveToCancelled", ...)` block:

```
Describe("MoveToCancelled", func() {
    It("moves file to cancelled dir with status cancelled and timestamp", func() {
        // Create a temp dir with in-progress/ and cancelled/ subdirs
        // Write a prompt file with status: approved
        // Call manager.MoveToCancelled(ctx, path)
        // Assert: file no longer exists at original path
        // Assert: file exists in cancelledDir
        // Assert: new file content has status: cancelled
        // Assert: new file content has non-empty cancelled: timestamp
    })

    It("creates cancelledDir if it does not exist", func() {
        // Use a cancelledDir path that doesn't exist yet
        // Call MoveToCancelled
        // Assert dir was created and file is there
    })
})
```

Also add a unit test for `MarkCancelled()` in `pkg/prompt/prompt_file_test.go`:
```
Describe("MarkCancelled", func() {
    It("sets status to cancelled with timestamp", func() {
        // Create a PromptFile
        // Call MarkCancelled()
        // Assert Status == "cancelled"
        // Assert Cancelled != ""  (timestamp is set)
    })
})
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change any directory names or remove existing fields — this is additive only
- `cancelledDir` param must be the 4th argument of `NewManager` (between `completedDir` and `mover`) — check the current signature before editing
- Do NOT add validation for `CancelledDir` in `Config.Validate()` — it defaults correctly
- `moveToCancelled` must follow the exact same pattern as `moveToCompleted` (load → mark → save → mkdirAll → move)
- Wrap all errors with `errors.Wrap(ctx, ...)` or `errors.Wrapf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Copyright header required on any new files
- After adding `CancelledPromptStatus` to `listQueued` skip list, do NOT remove it from `autoSetQueuedStatus` in `scanner.go` — both guards serve different purposes
- Do NOT change any other field order in `Frontmatter` struct except adding `Cancelled` after `RejectedReason`
- All existing tests must still pass — use `make test` iteratively after each step
- The `now()` helper on `PromptFile` is a private method — verify its name by reading the file before using it
</constraints>

<verification>
Run `make precommit` — must pass.

Additional spot checks:
```bash
# Confirm CancelledDir is in PromptsConfig and Defaults
grep -n "CancelledDir\|cancelledDir" pkg/config/config.go

# Confirm Cancelled timestamp field in Frontmatter
grep -n "Cancelled.*yaml" pkg/prompt/prompt.go

# Confirm moveToCancelled and MoveToCancelled exist
grep -n "moveToCancelled\|MoveToCancelled" pkg/prompt/prompt.go

# Confirm CancelledPromptStatus in listQueued skip list
grep -A 15 "func listQueued" pkg/prompt/prompt.go | grep -i cancelled

# Confirm interfaces updated
grep -n "MoveToCancelled" pkg/cmd/prompt_manager.go pkg/processor/prompt_manager.go

# Confirm all createPromptManager calls have 5 args
grep -A 5 "createPromptManager(" pkg/factory/factory.go | grep -c "CancelledDir"

# Confirm mock has MoveToCancelled
grep -n "MoveToCancelled" mocks/cmd-prompt-manager.go mocks/processor-prompt-manager.go
```
</verification>
