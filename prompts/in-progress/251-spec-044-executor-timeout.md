---
status: approved
spec: ["044"]
created: "2026-04-04T20:50:00Z"
queued: "2026-04-04T21:49:26Z"
---

<summary>
- Containers running longer than `maxPromptDuration` are killed automatically
- The kill is clean: graceful stop before force-kill fallback
- The timeout runs alongside the container and the stuck-container watcher
- The error returned on timeout includes the duration for diagnostics
- When `maxPromptDuration` is 0 (the zero value), no timeout goroutine is started — behaviour is unchanged
- The executor receives the timeout budget from project configuration
- Reattached containers also respect the timeout so resumed containers can be stopped too
- Tests cover: timeout fires, zero duration (no timeout), and clean cancellation
</summary>

<objective>
Wire `maxPromptDuration` into the executor so containers that exceed their wall-clock budget are killed cleanly and return a descriptive error. The executor must not change behaviour when the duration is 0 (timeout disabled).
</objective>

<context>
Read CLAUDE.md for project conventions.

**This prompt builds on Prompt 1 (spec-044-model)**: `Config.ParsedMaxPromptDuration()` and the config fields already exist before this prompt runs.

Read these files before making any changes:
- `pkg/executor/executor.go` — `dockerExecutor` struct, `NewDockerExecutor`, `Execute`, `Reattach`, `watchForCompletionReport`, `run.CancelOnFirstFinish` usage
- `pkg/factory/factory.go` — `createDockerExecutor`, `CreateProcessor`, and the `CreateSpecGenerator` function (both call `executor.NewDockerExecutor`)
- `pkg/config/config.go` — `ParsedMaxPromptDuration()` method added in Prompt 1
</context>

<requirements>
**Step 1: Add `maxPromptDuration` to `dockerExecutor` in `pkg/executor/executor.go`**

Add a `maxPromptDuration time.Duration` field to `dockerExecutor`:
```go
type dockerExecutor struct {
    // ... existing fields ...
    maxPromptDuration time.Duration // NEW: 0 = disabled
}
```

Place it as the last field in the struct. Do NOT remove or reorder existing fields.

Update `NewDockerExecutor` to accept and store it:
```go
func NewDockerExecutor(
    containerImage string,
    projectName string,
    model string,
    netrcFile string,
    gitconfigFile string,
    env map[string]string,
    extraMounts []config.ExtraMount,
    claudeDir string,
    maxPromptDuration time.Duration, // NEW
) Executor {
    return &dockerExecutor{
        // ... existing fields ...
        maxPromptDuration: maxPromptDuration,
    }
}
```

**Step 2: Add `timeoutKiller` helper in `pkg/executor/executor.go`**

Add a standalone function that waits for the deadline then stops the container. Returns nil on ctx cancel (normal exit):
```go
// timeoutKiller waits for the given deadline and then stops the container cleanly.
// Returns nil if ctx is cancelled before the deadline (normal container exit).
// The error message includes the duration so callers can surface it as lastFailReason.
func timeoutKiller(
    ctx context.Context,
    duration time.Duration,
    containerName string,
    runner commandRunner,
) error {
    select {
    case <-ctx.Done():
        return nil
    case <-time.After(duration):
    }
    slog.Warn("container exceeded maxPromptDuration, stopping",
        "containerName", containerName,
        "duration", duration)
    // #nosec G204 -- containerName is generated internally from prompt filename
    stopCmd := exec.CommandContext(ctx, "docker", "stop", containerName)
    if err := runner.Run(ctx, stopCmd); err != nil {
        slog.Warn("docker stop failed after timeout, attempting force kill",
            "containerName", containerName, "error", err)
        // #nosec G204 -- containerName is generated internally
        killCmd := exec.CommandContext(ctx, "docker", "kill", containerName)
        if killErr := runner.Run(ctx, killCmd); killErr != nil {
            slog.Error("docker kill also failed after timeout",
                "containerName", containerName, "error", killErr)
        }
    }
    return errors.Errorf(ctx, "prompt timed out after %s", duration)
}
```

**Step 3: Update `Execute` in `pkg/executor/executor.go`**

Currently `Execute` calls `run.CancelOnFirstFinish` with two goroutines (container + completion watcher). When `maxPromptDuration > 0`, add a third:

Replace the existing `run.CancelOnFirstFinish` call with:
```go
funcs := []func(ctx context.Context) error{
    func(ctx context.Context) error {
        return e.commandRunner.Run(ctx, cmd)
    },
    func(ctx context.Context) error {
        return watchForCompletionReport(ctx, logFile, containerName, 2*time.Minute, 10*time.Second, e.commandRunner)
    },
}
if e.maxPromptDuration > 0 {
    d := e.maxPromptDuration
    funcs = append(funcs, func(ctx context.Context) error {
        return timeoutKiller(ctx, d, containerName, e.commandRunner)
    })
}
if err := run.CancelOnFirstFinish(ctx, funcs...); err != nil {
    return errors.Wrap(ctx, err, "docker run failed")
}
```

**Step 4: Update `Reattach` in `pkg/executor/executor.go`**

Apply the same pattern to `Reattach` — add the timeout killer when `maxPromptDuration > 0`:
```go
funcs := []func(ctx context.Context) error{
    func(ctx context.Context) error {
        return e.commandRunner.Run(ctx, cmd)
    },
    func(ctx context.Context) error {
        return watchForCompletionReport(ctx, logFile, containerName, 2*time.Minute, 10*time.Second, e.commandRunner)
    },
}
if e.maxPromptDuration > 0 {
    d := e.maxPromptDuration
    funcs = append(funcs, func(ctx context.Context) error {
        return timeoutKiller(ctx, d, containerName, e.commandRunner)
    })
}
if err := run.CancelOnFirstFinish(ctx, funcs...); err != nil {
    return errors.Wrap(ctx, err, "reattach failed")
}
```

**Step 5: Update `pkg/factory/factory.go`**

Update `createDockerExecutor` to accept and pass through `maxPromptDuration`:
```go
func createDockerExecutor(
    containerImage string,
    projectName string,
    model string,
    netrcFile string,
    gitconfigFile string,
    env map[string]string,
    extraMounts []config.ExtraMount,
    claudeDir string,
    maxPromptDuration time.Duration, // NEW
) executor.Executor {
    return executor.NewDockerExecutor(
        containerImage, projectName, model, netrcFile, gitconfigFile, env, extraMounts, claudeDir,
        maxPromptDuration, // NEW
    )
}
```

Add `maxPromptDuration time.Duration` as a new parameter to `CreateProcessor` (line 467) and thread it through to `createDockerExecutor` (line 451).

`CreateProcessor` is called from two locations — update both:
- `CreateRunner` (line 264): `CreateProcessor(inProgressDir, completedDir, ...)`
- `CreateOneShotRunner` (line 331): `CreateProcessor(inProgressDir, completedDir, ...)`

Both must pass `cfg.ParsedMaxPromptDuration()` as the new argument.

Also update `CreateSpecGenerator` (line 379) which calls `executor.NewDockerExecutor` directly — add the `maxPromptDuration` parameter there too.

**Step 6: Add tests in `pkg/executor/executor_internal_test.go`**

Follow the existing test patterns (Ginkgo v2, `Describe`/`Context`/`It`).

Test `timeoutKiller`:
1. **Timeout fires**: use a very short duration (e.g. `10*time.Millisecond`), assert `docker stop` is called and the returned error contains `"timed out after"`
2. **Context cancelled before deadline**: cancel ctx before duration expires, assert no docker command is run and nil is returned
3. **Docker stop fails, docker kill called**: configure the mock runner to fail on `docker stop`, assert `docker kill` is attempted

Use a mock `commandRunner` (already exists as `fakeCommandRunner` or create one following existing test patterns in `executor_internal_test.go`).
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- When `maxPromptDuration == 0`, no timeout goroutine is added — `Execute` and `Reattach` behave exactly as before this change
- The timeout killer must NOT be the only mechanism — the existing `watchForCompletionReport` goroutine remains untouched
- `run.CancelOnFirstFinish` semantics: the first goroutine that returns cancels all others. The timeout killer returns a non-nil error on timeout, which correctly propagates as the execution error
- The error from `timeoutKiller` must include the duration in human-readable form (`duration.String()`) — the processor in Prompt 3 uses this as `lastFailReason`
- Container kill must be clean: `docker stop` first (default 10s SIGTERM grace), `docker kill` only as fallback if stop fails
- Use `errors.Errorf(ctx, ...)` for the timeout error — never `fmt.Errorf`
- Do NOT change the `watchForCompletionReport` function signature or behaviour
</constraints>

<verification>
```bash
# Confirm maxPromptDuration field exists in dockerExecutor
grep -n "maxPromptDuration" pkg/executor/executor.go

# Confirm timeoutKiller function exists
grep -n "func timeoutKiller" pkg/executor/executor.go

# Confirm factory passes the duration
grep -n "maxPromptDuration\|ParsedMaxPromptDuration" pkg/factory/factory.go

make precommit
```
Must pass with no errors.
</verification>
