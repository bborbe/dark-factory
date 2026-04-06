---
status: completed
summary: Reattach now accepts an explicit maxPromptDuration parameter; processor computes remaining wall-clock duration from the prompt's started timestamp and kills containers that have already exceeded the timeout without reattaching
container: dark-factory-262-fix-reattach-timeout-remaining-duration
dark-factory-version: v0.104.0
created: "2026-04-06T16:00:00Z"
queued: "2026-04-06T14:55:02Z"
started: "2026-04-06T15:20:38Z"
completed: "2026-04-06T15:39:56Z"
---

<summary>
- Daemon restart no longer grants a full timeout to containers that have already been running for hours
- Reattach computes remaining duration from the prompt's `started` frontmatter timestamp
- If the container has already exceeded maxPromptDuration, it is killed immediately without reattaching
- The fresh Execute path is unchanged — it still uses the full maxPromptDuration
- The Executor interface gains a new `maxPromptDuration` parameter on Reattach so callers control the timeout
</summary>

<objective>
Fix the reattach timeout bug: when the daemon restarts and reattaches to a running container, the timeout killer starts a fresh timer with the full maxPromptDuration instead of computing the remaining time from the prompt's `started` timestamp. A container that has been running for 2+ hours gets another full 60 minutes instead of being killed immediately.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making changes:
- `pkg/executor/executor.go` — `Executor` interface (~line 31), `Reattach` method (~line 203), `buildRunFuncs` (~line 171), `timeoutKiller` (~line 252)
- `pkg/processor/processor.go` — `resumePrompt` (~line 258), especially line 317 where `p.executor.Reattach` is called
- `pkg/prompt/prompt.go` — `Frontmatter` struct (~line 180, `Started` field is `string` in RFC3339), `PrepareForExecution` (~line 338)
- `mocks/executor.go` — counterfeiter-generated mock for Executor interface

Follow the time injection pattern from the coding guidelines:
- Import: `libtime "github.com/bborbe/time"`
- Use `libtime.CurrentDateTimeGetter` interface, never call `time.Now()` directly
</context>

<requirements>
1. **Add `maxPromptDuration time.Duration` parameter to `Reattach` in the `Executor` interface** in `pkg/executor/executor.go`:

   Change from:
   ```go
   Reattach(ctx context.Context, logFile string, containerName string) error
   ```
   To:
   ```go
   Reattach(ctx context.Context, logFile string, containerName string, maxPromptDuration time.Duration) error
   ```

   This lets the caller (processor) pass the remaining duration rather than using the struct field.

2. **Update `dockerExecutor.Reattach` implementation** in `pkg/executor/executor.go`:

   Change the method signature to accept `maxPromptDuration time.Duration`. In the body, use a local `buildRunFuncsWithDuration` call or override the timeout duration for this invocation. The simplest approach: extract a `buildRunFuncsWithTimeout` helper that takes an explicit duration, or pass the duration directly:

   ```go
   func (e *dockerExecutor) Reattach(ctx context.Context, logFile string, containerName string, maxPromptDuration time.Duration) error {
       // ... existing logFile prep ...

       cmd := exec.CommandContext(ctx, "docker", "logs", "--follow", containerName)
       cmd.Stdout = io.MultiWriter(os.Stdout, logFileHandle)
       cmd.Stderr = io.MultiWriter(os.Stderr, logFileHandle)

       slog.Info("reattaching to running container", "containerName", containerName,
           "maxPromptDuration", maxPromptDuration)

       if err := run.CancelOnFirstFinish(ctx, e.buildRunFuncsWithTimeout(cmd, logFile, containerName, maxPromptDuration)...); err != nil {
           return errors.Wrap(ctx, err, "reattach failed")
       }
       return nil
   }
   ```

3. **Refactor `buildRunFuncs` into two methods** in `pkg/executor/executor.go`:

   Create a new `buildRunFuncsWithTimeout` that accepts an explicit `timeout time.Duration` parameter and uses it instead of `e.maxPromptDuration`:

   ```go
   // buildRunFuncsWithTimeout returns the set of parallel functions with an explicit timeout.
   func (e *dockerExecutor) buildRunFuncsWithTimeout(
       cmd *exec.Cmd,
       logFile string,
       containerName string,
       timeout time.Duration,
   ) []run.Func {
       getter := e.currentDateTimeGetter
       funcs := []run.Func{
           func(ctx context.Context) error {
               return e.commandRunner.Run(ctx, cmd)
           },
           func(ctx context.Context) error {
               return watchForCompletionReport(ctx, logFile, containerName, 2*time.Minute, 10*time.Second, e.commandRunner, getter)
           },
       }
       if timeout > 0 {
           d := timeout
           funcs = append(funcs, func(ctx context.Context) error {
               return timeoutKiller(ctx, d, containerName, e.commandRunner, getter)
           })
       }
       return funcs
   }
   ```

   Then update the original `buildRunFuncs` to delegate:
   ```go
   func (e *dockerExecutor) buildRunFuncs(cmd *exec.Cmd, logFile string, containerName string) []run.Func {
       return e.buildRunFuncsWithTimeout(cmd, logFile, containerName, e.maxPromptDuration)
   }
   ```

   `Execute` still calls `buildRunFuncs` (no change). `Reattach` calls `buildRunFuncsWithTimeout` directly.

4. **Regenerate the counterfeiter mock** by running:
   ```bash
   go generate ./pkg/executor/...
   ```
   This updates `mocks/executor.go` to match the new `Reattach` signature (4 parameters instead of 3).

5. **Add `maxPromptDuration time.Duration` to the processor** in `pkg/processor/processor.go`:

   Add a parameter to `NewProcessor`:
   ```go
   func NewProcessor(
       // ... existing params ...
       autoRetryLimit int,
       maxPromptDuration time.Duration,  // add as last parameter
   ) Processor {
   ```

   Add field to `processor` struct:
   ```go
   type processor struct {
       // ... existing fields ...
       autoRetryLimit    int
       maxPromptDuration time.Duration
   }
   ```

   Set it in the constructor body.

6. **Compute remaining duration and update `resumePrompt`** in `pkg/processor/processor.go`:

   Complete implementation of the remaining duration calculation (from step 5). The full block before the Reattach call:

   ```go
   // Compute remaining timeout from wall-clock started time
   remainingDuration := p.maxPromptDuration // default: full duration (also covers Started="" case)
   if p.maxPromptDuration > 0 && pf.Frontmatter.Started != "" {
       started, parseErr := time.Parse(time.RFC3339, pf.Frontmatter.Started)
       if parseErr != nil {
           slog.Warn("cannot parse started timestamp, using full timeout",
               "started", pf.Frontmatter.Started, "error", parseErr)
       } else {
           elapsed := time.Since(started)
           remainingDuration = p.maxPromptDuration - elapsed
           if remainingDuration <= 0 {
               slog.Warn("container exceeded maxPromptDuration, killing without reattach",
                   "container", containerName,
                   "started", pf.Frontmatter.Started,
                   "elapsed", elapsed)
               p.executor.StopAndRemoveContainer(ctx, containerName)
               pf.SetLastFailReason(fmt.Sprintf("prompt timed out after %s (detected on reattach)", elapsed))
               pf.MarkFailed()
               if saveErr := pf.Save(ctx); saveErr != nil {
                   return errors.Wrap(ctx, saveErr, "save prompt after timeout on reattach")
               }
               return nil
           }
           slog.Info("computed remaining timeout for reattach",
               "remaining", remainingDuration,
               "elapsed", elapsed,
               "maxPromptDuration", p.maxPromptDuration)
       }
   }
   ```

   Then update the Reattach call:
   ```go
   if err := p.executor.Reattach(ctx, logFile, containerName, remainingDuration); err != nil {
       return errors.Wrap(ctx, err, "reattach to container")
   }
   ```

7. **Update `NewProcessor` callers in `pkg/factory/factory.go`**:

   Find the `NewProcessor` call site (search for `processor.NewProcessor`). The factory already has `maxPromptDuration` from `cfg.MaxPromptDuration` (it passes it to `NewDockerExecutor`). Add `maxPromptDuration` as the last argument to `processor.NewProcessor(...)`.

   Do the same for any other callers of `NewProcessor` (check with `grep -rn 'processor.NewProcessor' --include='*.go'`).

8. **Update test call sites** that construct a `Processor` or call `Reattach` (note: ~73 `NewProcessor` calls in `processor_test.go`):
   - Search for `processor.NewProcessor` in test files and add the `maxPromptDuration` argument (use `0` or `time.Hour` as appropriate).
   - Search for `.Reattach(` in test files and add the duration argument.
   - Search for `ReattachReturns` and `ReattachCallCount` in test files — the mock arg count changes.

9. **Add `"fmt"` to imports** in `pkg/processor/processor.go` if not already present (needed for `fmt.Sprintf` in the timeout message).

10. **Update CHANGELOG.md**: Add under `## Unreleased` (create section if needed):
    - `fix: reattach timeout uses remaining wall-clock duration instead of full maxPromptDuration`
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- The Execute path must continue using the full maxPromptDuration (only Reattach changes)
- When maxPromptDuration is 0, no timeout is applied in either path
- Use `errors.Wrap(ctx, err, "message")` from `github.com/bborbe/errors` — never bare `return err`
- Use `errors.Errorf(ctx, ...)` — never `fmt.Errorf`
- Follow project conventions: Ginkgo v2/Gomega tests, doc comments on exports, structured logging
- Use `time.Since(started)` for elapsed computation — do NOT use the injected `CurrentDateTimeGetter` in the processor (processor doesn't have one; `time.Since` is acceptable here since this is a one-shot calculation, not a long-running timer susceptible to App Nap coalescing)
</constraints>

<verification>
```bash
# Verify Reattach signature has 4 params (ctx, logFile, containerName, maxPromptDuration)
grep -n 'Reattach(ctx context.Context' pkg/executor/executor.go

# Verify remaining duration computation exists in resumePrompt
grep -n 'remainingDuration' pkg/processor/processor.go

# Verify maxPromptDuration field exists in processor struct
grep -n 'maxPromptDuration' pkg/processor/processor.go

# Verify buildRunFuncsWithTimeout helper exists
grep -n 'func.*buildRunFuncsWithTimeout' pkg/executor/executor.go

# Verify mock is regenerated
grep -n 'func.*Reattach.*Duration' mocks/executor.go

# Verify no time.Now() in processor (should use time.Since which is acceptable)
grep -n 'time.Now()' pkg/processor/processor.go
# Expected: no results

# Run tests
go generate ./pkg/executor/...
make precommit
```
All must pass with no errors.
</verification>
