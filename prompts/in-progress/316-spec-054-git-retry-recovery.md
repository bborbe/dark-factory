---
status: approved
spec: [054-committing-status-git-retry]
created: "2026-04-17T14:00:00Z"
queued: "2026-04-17T14:18:38Z"
branch: dark-factory/committing-status-git-retry
---

<summary>
- After a container exits successfully, the direct workflow sets the prompt to `committing` before any git operations
- The prompt file stays in `in-progress/` until the git commit of the prompt move succeeds
- Git commit operations retry up to 3 times with exponential backoff (2s, 4s, 8s) and a 30-second overall timeout
- A git commit failure after all retries does NOT crash the daemon — the prompt stays `committing` and the error is logged
- On daemon startup, any `committing` prompts in `in-progress/` are detected and their commits re-attempted
- On each 5-second daemon cycle, `committing` prompts are re-attempted before new queued prompts are processed
- Retry log messages: WARN on each retry attempt, INFO on success after retries, ERROR when all retries exhausted
- Only the direct workflow is affected — clone/branch/worktree workflows are unchanged
</summary>

<objective>
Wire the `committing` status into the direct workflow executor and the processor's startup/daemon-cycle recovery loop. After this prompt, a git commit failure during post-container processing no longer crashes the daemon; the prompt self-heals on the next daemon cycle without human intervention.

**Precondition:** Prompt 1 (`1-spec-054-committing-model`) has been executed. `CommittingPromptStatus`, `MarkCommitting()`, `FindCommitting()`, and the updated `ListQueued` are all in place in `pkg/prompt/prompt.go`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-context-cancellation-in-loops.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Read the `run.Retry` / `run.Backoff` API in `vendor/github.com/bborbe/run/run_retry.go` — it is the project's standard retry library.

Key files to read before editing:
- `pkg/git/git.go` — `CommitCompletedFile` (line ~145), `CommitOnly` (line ~72), `gitAddAll` (line ~197), `gitCommit` (line ~358)
- `pkg/processor/workflow_executor_direct.go` — `Complete()` method and how it calls `moveToCompletedAndCommit` then `handleDirectWorkflow`
- `pkg/processor/workflow_helpers.go` — `moveToCompletedAndCommit()` (line 42), `handleDirectWorkflow()` (line 113)
- `pkg/processor/processor.go` — `Processor` interface (lines 38–46), `ResumeExecuting()` (lines 219–239), `processExistingQueued()` (lines 398+), `Process()` loop (lines 147–190), `ProcessQueue()` (lines 192–216)
- `pkg/runner/runner.go` — where `ResumeExecuting` is called (line ~194)
- `pkg/processor/processor_internal_test.go` — `ResumeExecuting` test suite structure (~line 486) to understand test patterns for the new `ResumeCommitting` tests
</context>

<requirements>

## 1. Add `CommitWithRetry` to `pkg/git/git.go`

Use the existing `github.com/bborbe/run` library (already in `go.mod`) for retry logic. Add a default backoff config and a thin wrapper:

```go
// DefaultCommitBackoff defines the default retry backoff for git commit operations.
// 3 retries with exponential backoff: ~2s, ~4s, ~8s.
var DefaultCommitBackoff = run.Backoff{
    Delay:   2 * time.Second,
    Factor:  1.0,
    Retries: 3,
}

// CommitWithRetry runs fn with retry logic using the given backoff configuration.
// Logs WARN on each retry attempt. Pass DefaultCommitBackoff for production use;
// tests can pass a Backoff with Delay: 0 and small Retries.
func CommitWithRetry(ctx context.Context, backoff run.Backoff, fn func(context.Context) error) error {
    return run.Retry(backoff, func(ctx context.Context) error {
        err := fn(ctx)
        if err != nil {
            if _, lockErr := os.Stat(".git/index.lock"); lockErr == nil {
                slog.Warn("retrying git commit, index.lock held", "error", err)
            } else {
                slog.Warn("retrying git commit after failure", "error", err)
            }
        }
        return err
    })(ctx)
}
```

Add required imports: `"log/slog"`, `"github.com/bborbe/run"` (already in `go.mod`).

## 2. Refactor `directWorkflowExecutor.Complete()` in `pkg/processor/workflow_executor_direct.go`

The current `Complete()` calls `moveToCompletedAndCommit` (which moves the prompt file to `completed/` AND commits it) and then `handleDirectWorkflow` (which commits the code changes). 

**The new flow reverses this order** and introduces the `committing` status as a checkpoint before any git operations. The prompt file must NOT be physically moved to `completed/` until the git commit of the prompt move succeeds.

Replace the current `Complete()` body with:

```go
// Complete sets the prompt to committing, commits all work, then moves and commits the prompt file.
// If any git operation fails after retries, the prompt stays committing for the next daemon cycle.
func (e *directWorkflowExecutor) Complete(
    gitCtx, ctx context.Context,
    pf *prompt.PromptFile,
    title, promptPath, completedPath string,
) error {
    // Transition to committing BEFORE any git operations.
    // The file stays in in-progress/ until the commit of the prompt move succeeds.
    pf.MarkCommitting()
    if err := pf.Save(ctx); err != nil {
        return errors.Wrap(ctx, err, "save committing status")
    }

    if err := e.completeCommit(gitCtx, ctx, pf, title, promptPath, completedPath); err != nil {
        slog.Error("git commit failed after all retries, will retry next cycle",
            "file", filepath.Base(promptPath), "error", err)
        return nil // do NOT crash the daemon
    }
    return nil
}

// completeCommit performs the two-phase git commit for the direct workflow:
// (1) commit all work files, (2) move prompt to completed and commit the move.
// Both phases use CommitWithRetry. Returns an error if any phase exhausts all retries.
func (e *directWorkflowExecutor) completeCommit(
    gitCtx, ctx context.Context,
    pf *prompt.PromptFile,
    title, promptPath, completedPath string,
) error {
    // Phase 1: commit all code changes (vendor, source, etc.) with retry.
    if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
        return handleDirectWorkflow(retryCtx, ctx, e.deps, title, "")
    }); err != nil {
        return errors.Wrap(ctx, err, "commit work files")
    }

    // Phase 2: auto-complete specs (best-effort, non-blocking).
    for _, specID := range pf.Specs() {
        if err := e.deps.AutoCompleter.CheckAndComplete(ctx, specID); err != nil {
            slog.Warn("spec auto-complete failed", "spec", specID, "error", err)
        }
    }

    // Phase 3: move prompt to completed/ (sets status: completed, physically moves the file).
    if err := e.deps.PromptManager.MoveToCompleted(ctx, promptPath); err != nil {
        return errors.Wrap(ctx, err, "move to completed")
    }
    slog.Info("moved to completed", "file", filepath.Base(promptPath))

    // Phase 4: commit the prompt-file move with retry.
    if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
        return e.deps.Releaser.CommitCompletedFile(retryCtx, completedPath)
    }); err != nil {
        // The file is now in completed/ but the git commit failed.
        // The recovery path (processCommittingPrompts) handles this:
        // it will detect no dirty work files and just commit the prompt move.
        return errors.Wrap(ctx, err, "commit completed file")
    }

    return nil
}
```

Required imports for `workflow_executor_direct.go`:
- `"log/slog"` (add if not present)
- `"path/filepath"` (add if not present)
- `"github.com/bborbe/dark-factory/pkg/git"` (add if not present)

**Important:** The original `moveToCompletedAndCommit` helper in `workflow_helpers.go` is NOT called from the direct executor anymore. Do NOT delete it — it is still used by other workflow executors (clone, branch, worktree). Only the direct executor's `Complete()` changes.

## 3. Add `ResumeCommitting` to the `Processor` interface

In `pkg/processor/processor.go`, add `ResumeCommitting` to the `Processor` interface after `ResumeExecuting`:

```go
// ResumeCommitting retries git commits for any prompts in "committing" state on startup.
// Called once by the runner before the normal event loop begins.
// Unlike ResumeExecuting, failures are non-fatal: the prompt stays committing and is
// retried on the next daemon cycle.
ResumeCommitting(ctx context.Context) error
```

## 4. Implement `processCommittingPrompts` on `processor`

Add `FindCommitting(ctx context.Context) ([]string, error)` to the `PromptManager` interface in `pkg/processor/prompt_manager.go` (search for `type PromptManager interface`). The `*prompt.Manager` concrete type already implements this method (added in prompt 1).

Then add `processCommittingPrompts` to `pkg/processor/processor.go`:

```go
// processCommittingPrompts retries git commits for prompts in "committing" state.
// Used on startup and on each daemon cycle. Failures are non-fatal.
func (p *processor) processCommittingPrompts(ctx context.Context) {
    paths, err := p.promptManager.FindCommitting(ctx)
    if err != nil {
        slog.Warn("failed to scan for committing prompts", "error", err)
        return
    }
    for _, promptPath := range paths {
        if ctx.Err() != nil {
            return
        }
        if err := p.recoverCommittingPrompt(ctx, promptPath); err != nil {
            slog.Error("git commit failed after all retries, will retry next cycle",
                "file", filepath.Base(promptPath), "error", err)
        }
    }
}
```

Add `recoverCommittingPrompt`:

```go
// recoverCommittingPrompt attempts to commit dirty work files and move the prompt to completed/.
// Called for each "committing" prompt during startup recovery and daemon cycle retries.
// If dirty work files exist, they are committed first (the container's code changes).
// If no dirty files exist, the code was already committed — only the prompt move is needed.
func (p *processor) recoverCommittingPrompt(ctx context.Context, promptPath string) error {
    gitCtx := context.WithoutCancel(ctx)
    completedPath := filepath.Join(p.completedDir, filepath.Base(promptPath))

    pf, err := p.promptManager.Load(ctx, promptPath)
    if err != nil {
        return errors.Wrap(ctx, err, "load committing prompt")
    }
    title := pf.Title()
    if title == "" {
        title = strings.TrimSuffix(filepath.Base(promptPath), ".md")
    }

    // Check if dirty work files remain (i.e., code commit from phase 1 never happened).
    hasDirty, err := git.HasDirtyFiles(gitCtx)
    if err != nil {
        return errors.Wrap(ctx, err, "check dirty files")
    }

    if hasDirty {
        // Commit all dirty work files (vendor, source, etc.) with retry.
        if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
            return git.CommitAll(retryCtx, title)
        }); err != nil {
            return errors.Wrap(ctx, err, "commit work files during recovery")
        }
        slog.Info("committed work files during committing recovery", "file", filepath.Base(promptPath))
    }

    // Auto-complete specs (best-effort).
    for _, specID := range pf.Specs() {
        if err := p.autoCompleter.CheckAndComplete(ctx, specID); err != nil {
            slog.Warn("spec auto-complete failed", "spec", specID, "error", err)
        }
    }

    // Move prompt to completed/ (sets status: completed on disk, physically moves file).
    if err := p.promptManager.MoveToCompleted(ctx, promptPath); err != nil {
        return errors.Wrap(ctx, err, "move to completed during recovery")
    }

    // Commit the prompt-file move with retry.
    if err := git.CommitWithRetry(gitCtx, git.DefaultCommitBackoff, func(retryCtx context.Context) error {
        return p.releaser.CommitCompletedFile(retryCtx, completedPath)
    }); err != nil {
        return errors.Wrap(ctx, err, "commit completed file during recovery")
    }

    slog.Info("git commit recovery succeeded", "file", filepath.Base(completedPath))
    return nil
}
```

Note: `recoverCommittingPrompt` requires access to `p.autoCompleter`. Verify that field exists on the `processor` struct (it does — it is used in `processPrompt`).

## 5. Add `CommitAll` to `pkg/git/git.go`

The recovery path needs a "commit all dirty files with a given message" function. Add to `pkg/git/git.go`:

```go
// CommitAll stages all changes and commits with the given message.
// Used during committing recovery to commit work files left from a prior run.
func CommitAll(ctx context.Context, message string) error {
    if err := gitAddAll(ctx); err != nil {
        return errors.Wrap(ctx, err, "git add")
    }
    // Check if there is anything to commit
    statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
    // #nosec G204 -- fixed command with no user input
    output, err := statusCmd.Output()
    if err != nil {
        return errors.Wrap(ctx, err, "git status")
    }
    if len(strings.TrimSpace(string(output))) == 0 {
        return nil // nothing to commit
    }
    return gitCommit(ctx, message)
}
```

Also add `HasDirtyFiles` (exported, called from `processor`). Add to `pkg/git/git.go`:

```go
// HasDirtyFiles returns true if there are any uncommitted changes in the working tree.
func HasDirtyFiles(ctx context.Context) (bool, error) {
    // #nosec G204 -- fixed command with no user input
    cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
    output, err := cmd.Output()
    if err != nil {
        return false, errors.Wrap(ctx, err, "git status")
    }
    return len(strings.TrimSpace(string(output))) > 0, nil
}
```

## 6. Implement `ResumeCommitting` on `processor`

Add to `pkg/processor/processor.go`:

```go
// ResumeCommitting retries git commits for any prompts still in "committing" state on startup.
func (p *processor) ResumeCommitting(ctx context.Context) error {
    p.processCommittingPrompts(ctx)
    return nil // always non-fatal
}
```

## 7. Call `processCommittingPrompts` in the daemon loop

In `pkg/processor/processor.go`, update `Process()` to call `processCommittingPrompts` on each tick BEFORE `processExistingQueued`:

```go
case <-ticker.C:
    // Process committing prompts first (retry pending git commits).
    p.processCommittingPrompts(ctx)
    // Periodic scan for queued prompts (in case we missed a signal)
    if err := p.processExistingQueued(ctx); err != nil {
        slog.Warn("prompt failed; queue blocked until manual retry", "error", err)
    }
```

Similarly update the `case <-p.ready:` branch:

```go
case <-p.ready:
    p.skippedPrompts = make(map[string]libtime.DateTime)
    p.processCommittingPrompts(ctx)
    if err := p.processExistingQueued(ctx); err != nil {
        slog.Warn("prompt failed; queue blocked until manual retry", "error", err)
    }
```

Also call `processCommittingPrompts` at the START of `Process()` (on startup, after the initial `processExistingQueued`):

```go
// After processExistingQueued on startup, also retry any committing prompts.
p.processCommittingPrompts(ctx)
```

## 8. Call `ResumeCommitting` in `pkg/runner/runner.go`

After the existing `r.processor.ResumeExecuting(ctx)` call (~line 194), add:

```go
// Daemon-only: retry git commits for any prompts left in "committing" state.
if err := r.processor.ResumeCommitting(ctx); err != nil {
    slog.Warn("resume committing failed on startup, will retry on next cycle", "error", err)
    // non-fatal — continue startup
}
```

## 9. Update mocks

After adding `FindCommitting` to the `PromptManager` interface in `pkg/processor/processor.go` and `ResumeCommitting` to the `Processor` interface, regenerate the mocks:

```bash
cd /workspace && go generate ./pkg/processor/...
```

OR manually add the new methods to `mocks/processor.go` and any mock that implements `PromptManager` (find with `grep -rn "PromptManager" mocks/`).

Verify all mock files compile:
```bash
cd /workspace && go build ./mocks/...
```

## 10. Write tests

### `pkg/git/git_test.go` — `CommitWithRetry`

Add a test suite for `CommitWithRetry` in the existing Ginkgo suite. Test scenarios:
1. `fn` succeeds on first attempt — returns nil, no retries
2. `fn` fails once then succeeds — returns nil, logged retry
3. `fn` fails all 3 retries — returns error
4. Context already cancelled — returns error immediately

Use a counter to track call count with zero-delay backoff:
```go
callCount := 0
fn := func(ctx context.Context) error {
    callCount++
    if callCount < 3 {
        return errors.Errorf(ctx, "simulated failure")
    }
    return nil
}
// Zero delay in tests — no real sleeps.
testBackoff := run.Backoff{Delay: 0, Factor: 0, Retries: 3}
err := git.CommitWithRetry(ctx, testBackoff, fn)
Expect(err).NotTo(HaveOccurred())
Expect(callCount).To(Equal(3))
```

Since `backoff` is a parameter, tests pass `Delay: 0` to avoid any real sleeps.

### `pkg/processor/processor_internal_test.go` — `ResumeCommitting`

Add a `Describe("ResumeCommitting", ...)` block modeled after the existing `Describe("ResumeExecuting", ...)` block (~line 486). Test scenarios:

1. No `committing` prompts — no git operations, returns nil
2. One `committing` prompt with dirty files — commits work files, moves to completed, commits prompt
3. One `committing` prompt with NO dirty files — skips work commit, moves to completed, commits prompt
4. Git commit fails all retries — prompt stays `committing`, function returns nil (non-fatal)

Use the existing mock infrastructure (`mocks.FakeProcessor`, etc.). Check existing test helpers for creating temp prompt files.

## 11. Write CHANGELOG entry

Add an `## Unreleased` section at the top of `CHANGELOG.md` (above the latest versioned section) if it does not exist, then append:

```
- feat: retry git commit with exponential backoff (3 retries, 2s/4s/8s) on index.lock or failure
- feat: direct workflow sets `committing` status before git ops; daemon continues on commit failure
- feat: startup and daemon-cycle recovery for `committing` prompts
```

## 12. Run `make test`

```bash
cd /workspace && make test
```

Must pass before proceeding.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Only the direct workflow executor (`workflow_executor_direct.go`) is changed — do NOT touch clone, branch, or worktree executors
- `moveToCompletedAndCommit` in `workflow_helpers.go` must NOT be deleted or changed — it is still used by non-direct executors
- `processCommittingPrompts` must treat `recoverCommittingPrompt` errors as non-fatal — log and continue, never crash
- Use `errors.Wrapf` / `errors.Wrap` from `github.com/bborbe/errors` for all error wrapping (no `fmt.Errorf`)
- Use `context.WithoutCancel(ctx)` for `gitCtx` in recovery (matches existing pattern in `resumePrompt`)
- All existing tests must still pass
- `CommitWithRetry` tests MUST use `run.Backoff{Delay: 0, Retries: 3}` — never real delays in tests
- Production callers pass `git.DefaultCommitBackoff`; use the existing `github.com/bborbe/run` retry library, do NOT reimplement retry logic
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "MarkCommitting\|CommittingPromptStatus" pkg/processor/workflow_executor_direct.go` — at least 1 match
2. `grep -n "CommitWithRetry" pkg/git/git.go` — at least 1 match
3. `grep -n "processCommittingPrompts\|ResumeCommitting" pkg/processor/processor.go` — at least 2 matches
4. `grep -n "ResumeCommitting" pkg/runner/runner.go` — 1 match
5. `go test ./pkg/git/...` and `go test ./pkg/processor/...` — both pass
</verification>
