---
status: committing
spec: [075-bug-prompt-cancel-leaves-running-container-and-file]
summary: Wired MoveToCancelled into CLI cancel command and processor so cancelled prompts always move out of in-progress/ into cancelled/
container: dark-factory-383-spec-075-cli-and-processor
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T09:20:00Z"
queued: "2026-05-06T10:06:58Z"
started: "2026-05-06T10:24:13Z"
branch: dark-factory/bug-prompt-cancel-leaves-running-container-and-file
---

<summary>
- `dark-factory prompt cancel <id>` now moves approved/queued prompts to `prompts/cancelled/` immediately — the file is no longer left in `in-progress/`
- `dark-factory prompt cancel <id>` on an executing prompt writes `status: cancelled` to trigger the existing cancellationwatcher (which stops the container), then the processor moves the file to `prompts/cancelled/` after the container exits
- The processor's `ProcessPrompt` calls `MoveToCancelled` after detecting user cancellation, ensuring the file is always removed from `in-progress/` regardless of which codepath handled the container stop
- Cancelling the same prompt twice is idempotent: if the file is already in `cancelled/`, the second call returns exit 0 with no error
- Cancelling a non-existent prompt returns a clear error (exit non-zero)
- `dark-factory prompt cancel` on an approved (non-running) prompt no longer requires the operator to manually move the file
- CHANGELOG `## Unreleased` entry is added
</summary>

<objective>
Wire `MoveToCancelled` (built in prompt 1) into the CLI cancel command and the processor so that cancelled prompt files are always moved out of `in-progress/` and into `cancelled/`. After this prompt, the full cancellation lifecycle is correct: approved prompts move immediately, executing prompts move after the container stops, and the daemon cannot respawn cancelled prompts.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter, no bare `return err`, no `fmt.Errorf`).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read before editing (read each fully before making any change):
- `pkg/cmd/cancel.go` — current `cancelCommand` struct, `NewCancelCommand`, `Run` method
- `pkg/cmd/cancel_test.go` — existing test suite to extend
- `pkg/cmd/prompt_manager.go` — `PromptManager` interface (now has `MoveToCancelled` from prompt 1)
- `pkg/cmd/prompt_finder.go` — `FindPromptFile` and `FindPromptFileInDirs` signatures
- `pkg/processor/processor.go` — `ProcessPrompt` and `runContainer` methods; find the exact lines where `cancelled = true` is returned
- `pkg/factory/factory.go` — `CreateCancelCommand` function (~line 1067) to update
- `CHANGELOG.md` — to append an `## Unreleased` entry
</context>

<requirements>

## 1. Update `cancelCommand` in `pkg/cmd/cancel.go`

### 1a. Add `cancelledDir string` field

Add `cancelledDir string` to the `cancelCommand` struct:

```go
type cancelCommand struct {
    queueDir     string
    cancelledDir string
    promptManager PromptManager
}
```

### 1b. Update `NewCancelCommand` signature

Add `cancelledDir string` as the second parameter (after `queueDir`):

```go
func NewCancelCommand(
    queueDir string,
    cancelledDir string,
    promptManager PromptManager,
) CancelCommand {
    return &cancelCommand{
        queueDir:      queueDir,
        cancelledDir:  cancelledDir,
        promptManager: promptManager,
    }
}
```

### 1c. Update `Run` to handle approved vs executing and add idempotency

Replace the current `Run` implementation entirely:

```go
func (a *cancelCommand) Run(ctx context.Context, args []string) error {
    if len(args) != 1 {
        return errors.Errorf(ctx, "usage: dark-factory prompt cancel <id>")
    }
    id := args[0]

    // Primary search: in-progress (the queue).
    path, err := FindPromptFile(ctx, a.queueDir, id)
    if err != nil {
        // Idempotency: if the file is already in cancelled/, treat as success.
        if a.cancelledDir != "" {
            cancelledPath, findErr := FindPromptFile(ctx, a.cancelledDir, id)
            if findErr == nil {
                pf, loadErr := a.promptManager.Load(ctx, cancelledPath)
                if loadErr == nil && pf.Frontmatter.Status == string(prompt.CancelledPromptStatus) {
                    fmt.Printf("already cancelled: %s\n", filepath.Base(cancelledPath))
                    return nil
                }
            }
        }
        return errors.Errorf(ctx, "prompt not found: %s", id)
    }

    pf, err := a.promptManager.Load(ctx, path)
    if err != nil {
        return errors.Wrap(ctx, err, "load prompt")
    }

    switch prompt.PromptStatus(pf.Frontmatter.Status) {
    case prompt.ApprovedPromptStatus:
        // Not yet running: mark cancelled and move the file immediately.
        if err := a.promptManager.MoveToCancelled(ctx, path); err != nil {
            return errors.Wrap(ctx, err, "move to cancelled")
        }
        fmt.Printf("cancelled: %s\n", filepath.Base(path))
        return nil

    case prompt.ExecutingPromptStatus:
        // Container is running: write status=cancelled to trigger the
        // cancellationwatcher (daemon-side), which will stop the container.
        // The processor moves the file to cancelled/ after the container exits.
        pf.MarkCancelled()
        if err := pf.Save(ctx); err != nil {
            return errors.Wrap(ctx, err, "save prompt")
        }
        fmt.Printf("cancelled: %s\n", filepath.Base(path))
        return nil

    default:
        return errors.Errorf(
            ctx,
            "cannot cancel prompt with status %q (only approved or executing prompts can be cancelled)",
            pf.Frontmatter.Status,
        )
    }
}
```

**Important**: The CLI must NOT block on container stop. For executing prompts, writing `status: cancelled` is sufficient — the daemon's `cancellationwatcher` detects the file write via fsnotify and stops the container asynchronously.

## 2. Update `processor.ProcessPrompt` in `pkg/processor/processor.go`

Find the block in `ProcessPrompt` that handles `cancelled = true` (returned by `runContainer`). Currently it reads:

```go
cancelled, execErr := p.runContainer(ctx, content, logFile, containerName, pr.Path)
if cancelled {
    return nil // proceed to next prompt; status is already set to cancelled
}
```

Replace the `if cancelled` block with:

```go
cancelled, execErr := p.runContainer(ctx, content, logFile, containerName, pr.Path)
if cancelled {
    // Move the file out of in-progress/ so the daemon cannot respawn it.
    if moveErr := p.promptManager.MoveToCancelled(ctx, pr.Path); moveErr != nil {
        slog.Warn("failed to move cancelled prompt", "file", filepath.Base(pr.Path), "error", moveErr)
        // Non-fatal: log and continue. The cancelled status prevents re-execution.
    }
    return nil // proceed to next prompt
}
```

**Why non-fatal**: if the file was already moved (e.g., a race with a concurrent CLI call on the same prompt), logging a warning and proceeding is correct. The cancelled status already prevents re-execution.

## 3. Update `CreateCancelCommand` in `pkg/factory/factory.go`

Find `CreateCancelCommand` (search for `func CreateCancelCommand`). Add `cfg.Prompts.CancelledDir` as the second argument to `cmd.NewCancelCommand`:

```go
func CreateCancelCommand(
    cfg config.Config,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.CancelCommand {
    promptManager, _ := createPromptManager(
        cfg.Prompts.InboxDir,
        cfg.Prompts.InProgressDir,
        cfg.Prompts.CompletedDir,
        cfg.Prompts.CancelledDir,
        currentDateTimeGetter,
    )
    return cmd.NewCancelCommand(cfg.Prompts.InProgressDir, cfg.Prompts.CancelledDir, promptManager)
}
```

## 4. Update `pkg/cmd/cancel_test.go`

Read the existing test file before editing. Extend the `BeforeEach` to create a `cancelledDir` temp subdirectory and pass it to `NewCancelCommand`.

Add or update the following test cases (keep existing ones that remain valid):

### 4a. Update `BeforeEach`

`prompt.NewManager` requires a non-nil `FileMover`. `git.NewReleaser()` (from `github.com/bborbe/dark-factory/pkg/git`) implements `FileMover` via `MoveFile` (uses `git mv`, falls back to `os.Rename`) — verified at `pkg/git/git.go:101,148`. Use it as the mover. Passing `nil` would NPE the moment any test calls `MoveToCancelled`.

Add the import for `pkg/git` if not already present.

```go
BeforeEach(func() {
    var err error
    tempDir, err = os.MkdirTemp("", "cancel-test-*")
    Expect(err).NotTo(HaveOccurred())

    queueDir = filepath.Join(tempDir, "queue")
    err = os.MkdirAll(queueDir, 0750)
    Expect(err).NotTo(HaveOccurred())

    cancelledDir = filepath.Join(tempDir, "cancelled")
    // Do NOT pre-create cancelledDir — test that MoveToCancelled creates it on demand.

    cancelCmd = cmd.NewCancelCommand(
        queueDir,
        cancelledDir,
        prompt.NewManager("", queueDir, "", cancelledDir, git.NewReleaser(), libtime.NewCurrentDateTime()),
    )
    ctx = context.Background()
})
```

`cancelledDir` should be declared as a `var` in the surrounding `Describe` block alongside `queueDir`, `tempDir`, `cancelCmd`, `ctx` so it's accessible from individual It blocks.

### 4b. Test: approved prompt moves to cancelled dir

```
Describe("Cancel an approved prompt", func() {
    It("moves the file to cancelled dir", func() {
        testFile := filepath.Join(queueDir, "080-approved.md")
        // Write file with status: approved
        // Call cancelCmd.Run(ctx, []string{"080-approved.md"})
        // Assert err == nil
        // Assert testFile does NOT exist (moved away)
        // Assert filepath.Join(cancelledDir, "080-approved.md") EXISTS
        // Assert content of new file has status: cancelled
        // Assert content of new file has non-empty cancelled: <timestamp>
    })
})
```

### 4c. Test: executing prompt marks cancelled but does NOT move the file

```
Describe("Cancel an executing prompt", func() {
    It("marks cancelled but leaves file in queue dir (processor moves it)", func() {
        testFile := filepath.Join(queueDir, "081-executing.md")
        // Write file with status: executing
        // Call cancelCmd.Run
        // Assert err == nil
        // Assert testFile STILL EXISTS in queueDir (not moved)
        // Assert content has status: cancelled
    })
})
```

### 4d. Test: idempotency — second cancel is exit 0

`cancelledDir` is declared in the outer `Describe` (per 4a). The BeforeEach does NOT pre-create it; create it inside this It block before writing the cancelled file:

```
Describe("Cancel an already-cancelled prompt (in cancelled dir)", func() {
    It("is idempotent: returns nil", func() {
        err := os.MkdirAll(cancelledDir, 0750)
        Expect(err).NotTo(HaveOccurred())

        testFile := filepath.Join(cancelledDir, "084-cancelled.md")
        // Write file with status: cancelled to cancelledDir
        // Call cancelCmd.Run(ctx, []string{"084-cancelled.md"})
        // Assert err == nil (idempotent)
    })
})
```

### 4e. Keep existing tests that still apply

- Cancel a completed prompt → error
- Cancel a failed prompt → error
- Cancel with no args → usage error
- Cancel with unknown ID → "prompt not found" error

**Note**: The test for "Cancel an already cancelled prompt" in the OLD queue dir (status: cancelled in queueDir) should now test the idempotency path differently:
- A file with `status: cancelled` in `queueDir` does not match the `ApprovedPromptStatus` or `ExecutingPromptStatus` switch cases → returns "cannot cancel prompt with status cancelled" error. This is still correct for the direct-in-queue case.
- The idempotency (exit 0) only applies when the file has already been MOVED to `cancelledDir`.

Update or clarify the `Cancel an already cancelled prompt` test accordingly.

## 5. Add processor test for `MoveToCancelled` after cancellation

In `pkg/processor/processor_test.go` (or a new file `pkg/processor/processor_cancel_test.go` using package `processor_test`), add a test case verifying that when `runContainer` returns `cancelled=true`, the processor calls `promptManager.MoveToCancelled`:

```
Describe("ProcessPrompt — cancellation", func() {
    It("calls MoveToCancelled when container is cancelled", func() {
        // Set up processor with:
        //   - MockExecutor that returns context.Canceled (simulating container stopped)
        //   - MockCancellationWatcher that immediately fires (closes channel)
        //   - MockPromptManager
        // Call ProcessPrompt
        // Assert promptManager.MoveToCancelled was called with the prompt path
        // Assert ProcessPrompt returned nil
    })
})
```

Look at `pkg/processor/processor_test.go` existing test structure and use the same BeforeEach/mock-setup pattern.

## 6. Add CHANGELOG entry

In `CHANGELOG.md`, add (or append to) an `## Unreleased` section at the top:

```
## Unreleased

- fix: `prompt cancel` now moves approved prompts to `prompts/cancelled/` immediately, preventing daemon re-spawn
- fix: processor moves executing prompts to `prompts/cancelled/` after container stops on cancel
- fix: `prompt cancel` is idempotent — cancelling an already-cancelled prompt returns exit 0
- fix: cancelled prompts with `status: cancelled` timestamp written to frontmatter
```

If `## Unreleased` already exists, append these bullets to it.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT regress `prompt approve` / `prompt requeue` / `prompt retry` — those commands are unchanged
- The CLI MUST return after writing `status: cancelled` for executing prompts — do NOT call `docker stop` from the CLI, do NOT block on container exit. The daemon's `cancellationwatcher` handles the container stop.
- The processor's `MoveToCancelled` call after `cancelled = true` MUST be non-fatal on failure (log + continue) — the cancelled status already prevents re-execution
- Counterfeiter mocks (never manual) — use `mocks.ProcessorPromptManager` for processor tests
- External test package naming: `package cmd_test` and `package processor_test`
- Copyright header required on any new files
- Errors wrapped with `errors.Wrap(ctx, ...)` / `errors.Errorf(ctx, ...)` — never `fmt.Errorf`
- All existing tests must still pass — run `make test` iteratively
- The `cancelledDir` parameter in `NewCancelCommand` must be passed as the second argument (between `queueDir` and `promptManager`)
- `FindPromptFile` searches a single directory — use it for both `queueDir` and `cancelledDir` lookups separately (two calls, not one)
- The `prompt.NewManager` constructor now takes `cancelledDir` as 4th param (added in prompt 1) — make sure the test BeforeEach uses the updated signature
</constraints>

<verification>
Run `make precommit` — must pass.

Additional spot checks:
```bash
# Confirm cancelledDir is in cancelCommand struct and NewCancelCommand
grep -n "cancelledDir" pkg/cmd/cancel.go

# Confirm MoveToCancelled is called in cancelCommand.Run for approved status
grep -A 5 "ApprovedPromptStatus:" pkg/cmd/cancel.go

# Confirm MoveToCancelled called in ProcessPrompt after cancellation
grep -A 5 "if cancelled {" pkg/processor/processor.go

# Confirm factory passes cancelledDir
grep -A 5 "CreateCancelCommand" pkg/factory/factory.go | grep -i cancelled

# Run cancel-specific tests
go test ./pkg/cmd/... -run TestCancel -v
go test ./pkg/processor/... -v
```
</verification>
