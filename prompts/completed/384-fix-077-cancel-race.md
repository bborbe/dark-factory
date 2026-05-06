---
status: completed
spec: [077-bug-cancelled-prompt-classified-as-failed-due-to-stop-then-close-race]
summary: 'Fixed cancel race: swapped close(ch) before StopAndRemoveContainer in watcher.go and added deterministic file-reread fallback in processor.go runContainer; added ordering test in watcher_test.go and fallback test in processor_cancel_test.go.'
container: dark-factory-384-fix-077-cancel-race
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T16:30:00Z"
queued: "2026-05-06T16:47:43Z"
started: "2026-05-06T16:54:29Z"
completed: "2026-05-06T16:58:11Z"
branch: dark-factory/bug-cancelled-prompt-classified-as-failed-due-to-stop-then-close-race
---

<summary>
- Cancelling an executing prompt now correctly classifies it as `cancelled` (not `failed`) in every case
- The prompt file is moved to `prompts/cancelled/` with `status: cancelled` after a user-triggered cancel
- No `lastFailReason` is written when a prompt is cancelled by the user
- Daemon log shows `prompt cancelled` (not `prompt failed`) after a cancel signal
- The fix is in two places: the cancellationwatcher swaps `close(ch)` before blocking on `StopAndRemoveContainer`, and the processor adds a deterministic fallback that re-reads the prompt file after `Execute` returns to detect `status: cancelled` regardless of goroutine scheduling
- The cancel-while-approved (queued, not running) path is not affected — existing behaviour preserved
- Container still stops within ~5 seconds of the cancel signal
- CHANGELOG.md `## Unreleased` entry added
</summary>

<objective>
Fix the race condition that causes a cancelled executing prompt to be classified as `failed`. The root cause is that `cancellationwatcher` closes the cancel channel AFTER `StopAndRemoveContainer` returns (i.e., AFTER the container has already exited and `Execute` has already returned), leaving the processor goroutine with no time to set `cancelledByUser = true`. Two complementary fixes: (1) swap the watcher's operation order so `close(ch)` fires before the blocking stop, and (2) add a deterministic fallback in the processor that re-reads the prompt file after `Execute` returns and detects `status: cancelled` independently of goroutine scheduling.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-concurrency-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-context-cancellation-in-loops.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read in full before editing:

- `pkg/cancellationwatcher/watcher.go` — the watch goroutine where `StopAndRemoveContainer` is called before `close(ch)` (the bug)
- `pkg/cancellationwatcher/watcher_test.go` — existing tests to extend
- `pkg/processor/processor.go` — `runContainer` function (lines ~376–420) and `moveCancelledPrompt` helper (lines ~474–487)
- `pkg/processor/processor_cancel_test.go` — existing cancellation test to extend
- `CHANGELOG.md` — to append an `## Unreleased` entry

The bug trace (from spec 077):
```
16:10:44.542 prompt cancelled, stopping container     ← watcher fires
16:10:44.882 docker container exited with error  exit status 143  ← Execute returns
16:10:44.882 prompt failed                            ← cancelledByUser still false
```
The watcher calls `StopAndRemoveContainer` (blocks ~300ms–1s), THEN calls `close(ch)`. By the time `close(ch)` fires, `executor.Execute` has already returned, so the goroutine in `runContainer` that reads `<-cancelledCh` has no opportunity to set `cancelledByUser = true` before the main goroutine checks it.
</context>

<requirements>

## 1. Fix `pkg/cancellationwatcher/watcher.go`: close channel before stopping container

In the `watch` method, find the block that detects `status: cancelled` (lines ~98–106):

**Current (buggy) order:**
```go
if pf.Frontmatter.Status == string(prompt.CancelledPromptStatus) {
    slog.Info("prompt cancelled, stopping container",
        "file", filepath.Base(promptPath),
        "container", containerName,
    )
    w.executor.StopAndRemoveContainer(ctx, containerName)  // blocks until container exits
    close(ch)                                              // only fires AFTER container exits
    return
}
```

**Fixed order:**
```go
if pf.Frontmatter.Status == string(prompt.CancelledPromptStatus) {
    slog.Info("prompt cancelled, stopping container",
        "file", filepath.Base(promptPath),
        "container", containerName,
    )
    close(ch)                                              // signal processor BEFORE blocking
    w.executor.StopAndRemoveContainer(ctx, containerName)  // then stop container
    return
}
```

**Why this matters:** Closing `ch` first gives the goroutine in `runContainer` a scheduling window during the ~300ms–1s blocking `StopAndRemoveContainer` call to set `cancelledByUser = true`. Without this swap, `close(ch)` fires only after the container has already exited and `Execute` has already returned.

This is a two-line swap. No new imports, no new functions.

## 2. Fix `pkg/processor/processor.go:runContainer`: add deterministic fallback via file re-read

The goroutine-based approach (writing `cancelledByUser = true`) is still inherently racy — Go's memory model does not guarantee the write is visible without synchronization. Add a deterministic fallback after `Execute` returns.

Find the current post-Execute logic in `runContainer` (lines ~400–419):

```go
execErr := p.executor.Execute(execCtx, content, logFile, containerName.String())

if cancelledByUser {
    slog.Info("prompt cancelled", "file", filepath.Base(promptPath))
    return true, nil
}
if execErr != nil {
    if ctx.Err() != nil {
        slog.Info("daemon shutting down, leaving container running")
    } else {
        slog.Info("docker container exited with error", "error", execErr)
    }
    return false, errors.Wrap(ctx, execErr, "execute prompt")
}
if ctx.Err() != nil {
    slog.Info("daemon shutting down, leaving container running")
    return false, errors.Wrap(ctx, ctx.Err(), "daemon shutdown during execution")
}
slog.Info("docker container exited", "exitCode", 0)
return false, nil
```

Replace with:

```go
execErr := p.executor.Execute(execCtx, content, logFile, containerName.String())

// Primary signal: goroutine was scheduled before Execute returned.
if cancelledByUser {
    slog.Info("prompt cancelled", "file", filepath.Base(promptPath))
    return true, nil
}

// Deterministic fallback: if Execute returned with an error but the goroutine has not
// been scheduled yet, re-read the prompt file to detect a user-triggered cancel.
// This is the ground truth — the CLI writes status=cancelled before container stop.
if execErr != nil {
    if pf, loadErr := p.promptManager.Load(ctx, promptPath); loadErr == nil &&
        pf.Frontmatter.Status == string(prompt.CancelledPromptStatus) {
        slog.Info("prompt cancelled", "file", filepath.Base(promptPath))
        return true, nil
    }
    if ctx.Err() != nil {
        slog.Info("daemon shutting down, leaving container running")
    } else {
        slog.Info("docker container exited with error", "error", execErr)
    }
    return false, errors.Wrap(ctx, execErr, "execute prompt")
}
if ctx.Err() != nil {
    slog.Info("daemon shutting down, leaving container running")
    return false, errors.Wrap(ctx, ctx.Err(), "daemon shutdown during execution")
}
slog.Info("docker container exited", "exitCode", 0)
return false, nil
```

**Key points:**
- The fallback only runs when `execErr != nil`. A successful Execute (exit 0) is never a cancel.
- `p.promptManager.Load(ctx, promptPath)` reads the file from disk — this is the same file the CLI wrote `status: cancelled` to. No network, no IPC.
- If Load fails (e.g., file already moved), the fallback is skipped silently and the normal error path runs. This is safe.
- Use `ctx` (parent context), not `execCtx` — `execCtx` may already be cancelled at this point.

No new imports required — `prompt.CancelledPromptStatus` is already used in `processor.go`.

## 3. Update `pkg/cancellationwatcher/watcher_test.go`: verify channel-before-stop ordering

Add a new test case that directly verifies the channel closes BEFORE `StopAndRemoveContainer` completes. Read the existing test file fully before adding.

Add the following `It` block inside the `Describe("Watcher", ...)` block, after the existing "closes the channel and stops the container" test:

```go
It("closes the channel BEFORE StopAndRemoveContainer completes (ordering guarantee)", func() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // StopAndRemoveContainer blocks until we release it — lets us check
    // whether the channel was already closed before the stop finishes.
    stopStarted := make(chan struct{})
    stopReleased := make(chan struct{})
    mockExecutor.StopAndRemoveContainerStub = func(_ context.Context, _ string) {
        close(stopStarted)
        <-stopReleased
    }

    cancelledPF := prompt.NewPromptFile(
        promptPath,
        prompt.Frontmatter{Status: string(prompt.CancelledPromptStatus)},
        []byte("# Test\n"),
        libtime.NewCurrentDateTime(),
    )
    mockLoader.LoadReturns(cancelledPF, nil)

    ch := w.Watch(ctx, promptPath, "test-container")

    // Let fsnotify set up before writing
    time.Sleep(100 * time.Millisecond)
    err := os.WriteFile(promptPath, []byte("---\nstatus: cancelled\n---\n\n# Test\n"), 0600)
    Expect(err).NotTo(HaveOccurred())

    // Wait until StopAndRemoveContainer has been entered (but not yet returned)
    Eventually(stopStarted, 2*time.Second).Should(BeClosed())

    // Channel MUST be closed before StopAndRemoveContainer finishes
    Expect(ch).To(BeClosed(), "channel must close before StopAndRemoveContainer completes")

    // Release the stop
    close(stopReleased)

    Eventually(func() int {
        return mockExecutor.StopAndRemoveContainerCallCount()
    }, 2*time.Second).Should(Equal(1))
})
```

This test is deterministic: it holds `StopAndRemoveContainer` open and asserts the channel is already closed before it finishes.

## 4. Add test to `pkg/processor/processor_cancel_test.go`: fallback via file re-read

Read the existing `processor_cancel_test.go` fully before editing. Add a second `It` block inside the existing `Describe("ProcessPrompt — cancellation", ...)` block that tests the fallback path where Execute returns before the cancel channel fires.

**Before adding**, check: does `processor_cancel_test.go` already have a Describe block? If yes, add the new `It` inside it. If no, wrap both tests in one.

```go
It("detects cancellation from file when Execute returns before cancel channel closes", func() {
    tempDir, err := os.MkdirTemp("", "processor-cancel-fallback-*")
    Expect(err).NotTo(HaveOccurred())
    defer func() { _ = os.RemoveAll(tempDir) }()

    logDir := filepath.Join(tempDir, "log")
    err = os.MkdirAll(logDir, 0750)
    Expect(err).NotTo(HaveOccurred())

    promptPath := filepath.Join(tempDir, "002-cancel-fallback-test.md")
    err = os.WriteFile(
        promptPath,
        []byte("---\nstatus: approved\n---\n# Cancel fallback test\n\nTest content"),
        0600,
    )
    Expect(err).NotTo(HaveOccurred())

    ctx := context.Background()

    mgr := &mocks.ProcessorPromptManager{}
    // First call (ProcessPrompt initial load): return approved status
    mgr.LoadReturnsOnCall(0,
        prompt.NewPromptFile(
            promptPath,
            prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
            []byte("# Cancel fallback test\n\nTest content"),
            libtime.NewCurrentDateTime(),
        ),
        nil,
    )
    // Second call (runContainer fallback re-read): return cancelled status
    mgr.LoadReturnsOnCall(1,
        prompt.NewPromptFile(
            promptPath,
            prompt.Frontmatter{Status: string(prompt.CancelledPromptStatus)},
            []byte("# Cancel fallback test\n\nTest content"),
            libtime.NewCurrentDateTime(),
        ),
        nil,
    )
    mgr.MoveToCancelledReturns(nil)

    // Execute returns immediately with an error (simulates SIGTERM from container stop)
    exec := &mocks.Executor{}
    exec.ExecuteReturns(fmt.Errorf("exit status 143"))

    // CancellationWatcher returns a channel that NEVER closes during the test.
    // This simulates the race: Execute returns before the cancel channel fires.
    fakeCancellationWatcher := &mocks.CancellationWatcher{}
    neverClosingCh := make(chan struct{}) // never closed — simulates the race
    fakeCancellationWatcher.WatchReturns(neverClosingCh)

    workflowExec := &mocks.WorkflowExecutor{}
    workflowExec.SetupReturns(nil)

    vg := &mocks.VersionGetter{}
    vg.GetReturns("v0.0.1-test")

    pp := newProcessorWithMockWatcher(
        logDir,
        exec,
        mgr,
        vg,
        fakeCancellationWatcher,
        workflowExec,
    )

    pr := prompt.Prompt{Path: promptPath, Status: prompt.ApprovedPromptStatus}

    testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    err = pp.ProcessPrompt(testCtx, pr)
    Expect(err).NotTo(HaveOccurred())

    // Processor must call MoveToCancelled (not MarkFailed / return error)
    Expect(mgr.MoveToCancelledCallCount()).To(Equal(1))
    _, cancelledPath := mgr.MoveToCancelledArgsForCall(0)
    Expect(cancelledPath).To(Equal(promptPath))
})
```

**Import check**: Add `"fmt"` to the import block in `processor_cancel_test.go` if not already present. Verify all other imports are present by reading the file's import block.

Note: `LoadReturnsOnCall` is generated by Counterfeiter on the `Load` method. Verify it exists in the mock by running `grep -n "LoadReturnsOnCall" mocks/processor-prompt-manager.go` before using it.

## 5. Add CHANGELOG entry

In `CHANGELOG.md`, add (or append to) an `## Unreleased` section at the top:

```
## Unreleased

- fix: Cancelled executing prompts now classified as `cancelled` (not `failed`) — fixed race between cancellationwatcher's `close(ch)` and `StopAndRemoveContainer`, plus added deterministic fallback in processor to re-read file after `Execute` returns
```

If `## Unreleased` already exists, append the bullet to it.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change any other behaviour in `runContainer` — only the post-Execute section
- Do NOT change the daemon-shutdown path (`ctx.Err() != nil` after execErr) — it must remain unchanged
- The fallback file-read in requirement 2 MUST use `ctx` (parent context), not `execCtx` (child context that may be cancelled)
- Errors wrapped with `errors.Wrap(ctx, ...)` / `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf` in production code (test code may use `fmt.Errorf` for test errors)
- Do NOT close `ch` twice — the watcher closes it exactly once; the fix is reordering, not adding a second close
- All existing tests must still pass — run `make test` iteratively after each change
- Counterfeiter mocks (never manual) — use the generated mocks in `mocks/`
- External test packages: `package cancellationwatcher_test` and `package processor_test`
- The existing test "closes the channel and stops the container when status flips to cancelled" must still pass — do not remove or modify it (only add the new ordering test)
- Copyright header required on any new files (none expected — editing existing files only)
</constraints>

<verification>
Run `make precommit` — must exit 0.

Additional spot checks:
```bash
# Fix 1: verify close(ch) is BEFORE StopAndRemoveContainer in watcher.go
grep -n "close(ch)\|StopAndRemoveContainer" pkg/cancellationwatcher/watcher.go
# Expected: close(ch) line number < StopAndRemoveContainer line number

# Fix 2: verify fallback load is present in runContainer
grep -n "promptManager.Load\|CancelledPromptStatus" pkg/processor/processor.go
# Expected: both appear in the runContainer function body

# Verify ordering test exists in watcher tests
grep -n "closes the channel BEFORE\|stopStarted\|stopReleased" pkg/cancellationwatcher/watcher_test.go

# Verify fallback test exists in processor tests
grep -n "fallback\|LoadReturnsOnCall\|neverClosingCh" pkg/processor/processor_cancel_test.go

# Run package-level tests
go test ./pkg/cancellationwatcher/... -v
go test ./pkg/processor/... -run "cancellation" -v
```
</verification>
