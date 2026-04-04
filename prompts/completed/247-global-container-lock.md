---
status: completed
summary: Added pkg/containerlock with file-based flock ContainerLock interface, extended ContainerChecker with WaitUntilRunning, wired lock into processor via prepareContainerSlot/startContainerLockRelease helpers, and updated all factory wiring and tests.
container: dark-factory-247-global-container-lock
dark-factory-version: v0.94.2-1-g97d524b
created: "2026-04-04T14:31:55Z"
queued: "2026-04-04T14:31:55Z"
started: "2026-04-04T14:31:57Z"
completed: "2026-04-04T14:58:05Z"
---

<summary>
- Multiple daemons no longer race past the maxContainers limit
- A system-wide lock prevents the race between counting and starting containers
- Existing single-daemon behavior is unchanged
- Lock is acquired only for the brief moment between counting and starting, not held during execution
- Tests verify mutual exclusion between concurrent lock holders
</summary>

<objective>
Prevent multiple concurrent dark-factory daemons from exceeding the maxContainers limit by introducing a global file lock that serializes the "count running containers -> start container" sequence. Currently, N daemons can all poll `docker ps` simultaneously, all see "room available", and all start a container — resulting in N containers instead of the configured max.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read coding guidelines in `docs/` — especially error wrapping (`github.com/bborbe/errors`), Ginkgo/Gomega test patterns, and factory wiring conventions.

Key files:
- `pkg/processor/processor.go` — `waitForContainerSlot()` polls `ContainerCounter.CountRunning()` and proceeds when `count < maxContainers`. This is where the race lives: multiple daemons pass this check simultaneously.
- `pkg/executor/checker.go` — `ContainerCounter` interface (`CountRunning()`), `ContainerChecker` interface (`IsRunning()` — will be extended with `WaitUntilRunning()`), and their Docker implementations.
- `pkg/executor/executor.go` — `Execute()` starts the container and blocks until it exits (uses `CancelOnFirstFinish` with docker run + log watcher). The gap between `waitForContainerSlot` returning and `docker run` starting is the race window.
- `pkg/processor/processor.go` — `processPrompt()` calls `waitForContainerSlot()` then proceeds through prompt loading, workflow setup, and finally `p.executor.Execute()` at the end.
- `pkg/globalconfig/globalconfig.go` — `DefaultMaxContainers = 3`, global config path resolves to `$HOME/.dark-factory/config.yaml` at runtime.

The race window: daemon A calls `CountRunning()` -> sees 2/3 -> proceeds. Daemon B calls `CountRunning()` at the same moment -> also sees 2/3 -> also proceeds. Both start containers -> 4/3.

Current `processPrompt()` flow (lines 569-632):
```go
// current: no locking
if err := p.waitForContainerSlot(ctx); err != nil {
    return errors.Wrap(ctx, err, "wait for container slot")
}
// ... load prompt, setup workflow, enrich content ...
execErr := p.executor.Execute(execCtx, content, logFile, containerName)
```

Desired `processPrompt()` flow:
```go
// new: acquire lock before slot check
if err := p.containerLock.Acquire(ctx); err != nil {
    return errors.Wrap(ctx, err, "acquire container lock")
}
if err := p.waitForContainerSlot(ctx); err != nil {
    p.containerLock.Release(ctx)
    return errors.Wrap(ctx, err, "wait for container slot")
}
// ... load prompt, setup workflow, enrich content ...
// release lock in goroutine once container is running
go func() {
    defer p.containerLock.Release(ctx)
    p.containerChecker.WaitUntilRunning(ctx, containerName, 30*time.Second)
}()
execErr := p.executor.Execute(execCtx, content, logFile, containerName)
```
</context>

<requirements>
1. Create a new package `pkg/containerlock/` with a `ContainerLock` interface:
   ```go
   type ContainerLock interface {
       Acquire(ctx context.Context) error
       Release(ctx context.Context) error
   }
   ```

2. Implement using `syscall.Flock` (works on both Linux and macOS/Darwin) on a well-known lock file. The lock file path should be resolved at runtime using `os.UserHomeDir()` + `.dark-factory/container.lock`. Create the directory and file if they don't exist. Wrap all errors with `errors.Wrapf(ctx, err, ...)` from `github.com/bborbe/errors`.

3. Add a `WaitUntilRunning(ctx context.Context, containerName string, timeout time.Duration) error` method to the `ContainerChecker` interface in `pkg/executor/checker.go`. It should poll `docker inspect` every 2 seconds until the container exists and is running, or timeout is reached. This is needed so the lock holder knows when the container has started.

4. In `processPrompt()` in `pkg/processor/processor.go`, wrap the critical section:
   - Acquire the container lock BEFORE `waitForContainerSlot()`
   - Release on any error between acquire and Execute
   - Start a goroutine that calls `WaitUntilRunning(containerName, 30s)` then releases the lock
   - Call `Execute()` as before (it blocks until container exits)

   This means the lock is held only during: slot check + prompt load + workflow setup + container startup (~seconds), NOT during the entire container execution (~minutes).

5. Wire the `ContainerLock` through `pkg/factory/factory.go` into the processor constructor. Use a single shared lock file path for all processor instances. If `containerLock` is nil (e.g., in tests), skip locking.

6. Add tests in `pkg/containerlock/containerlock_test.go` using Ginkgo/Gomega and external test package (`package containerlock_test`):
   - Lock file is created in the expected directory
   - Acquire/Release round-trip works
   - Two goroutines cannot both hold the lock simultaneously (second blocks until first releases)
   - Acquire respects context cancellation
   - Aim for >= 80% statement coverage on the new package

7. Add a counterfeiter-generated fake for `ContainerLock` so processor tests can mock it.

8. Update `CHANGELOG.md` with an Unreleased entry.
</requirements>

<constraints>
- Do NOT hold the lock during container execution — only during the check-and-start window (seconds, not minutes)
- Do NOT change the `ContainerCounter` interface
- Do NOT change the container polling logic in `waitForContainerSlot` — only wrap it with the lock
- Do NOT use external dependencies if `syscall.Flock` is sufficient
- Do NOT commit — dark-factory handles git
- Lock file path must be deterministic and shared across all dark-factory instances on the machine
</constraints>

<verification>
Run `make precommit` — must pass with exit code 0.
Verify the lock file path resolves to `$HOME/.dark-factory/container.lock` at runtime.
Verify the lock is acquired before counting and released after container start (not after container exit).
</verification>
