---
status: draft
created: "2026-04-06T18:00:00Z"
---

<summary>
- The executor's parallel goroutine strategy changes from CancelOnFirstFinish to CancelOnFirstError
- The timeoutKiller goroutine now survives normal prompt completion, acting as a true safety net
- Normal completion (watchForCompletionReport returns nil) no longer cancels the timeout goroutine
- Any error from any goroutine still triggers cancellation of all others
- Both Execute and Reattach methods use the updated strategy
</summary>

<objective>
Change the executor's parallel goroutine runner from `run.CancelOnFirstFinish` to `run.CancelOnFirstError` so that the timeoutKiller remains active as a safety net even after normal prompt completion, preventing indefinitely-running containers when macOS App Nap delays timer goroutines.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/executor/executor.go` -- `Execute` method (line ~168), `Reattach` method (line ~243), and `buildRunFuncsWithTimeout` (line ~186-216)
- `pkg/executor/executor_test.go` -- existing tests for timeout and completion behavior
- `pkg/runner/runner.go` -- uses `run.CancelOnFirstError` already (confirms the function exists in the `run` package)

The `github.com/bborbe/run` package provides both:
- `run.CancelOnFirstFinish` -- returns when ANY goroutine finishes (including nil return)
- `run.CancelOnFirstError` -- returns when any goroutine returns a non-nil error; nil returns are ignored
</context>

<requirements>
1. In `pkg/executor/executor.go`, method `Execute` (around line 168):
   Change:
   ```go
   if err := run.CancelOnFirstFinish(ctx, e.buildRunFuncs(cmd, logFile, containerName)...); err != nil {
   ```
   To:
   ```go
   if err := run.CancelOnFirstError(ctx, e.buildRunFuncs(cmd, logFile, containerName)...); err != nil {
   ```

2. In `pkg/executor/executor.go`, method `Reattach` (around line 243):
   Change:
   ```go
   if err := run.CancelOnFirstFinish(ctx, e.buildRunFuncsWithTimeout(cmd, logFile, containerName, maxPromptDuration)...); err != nil {
   ```
   To:
   ```go
   if err := run.CancelOnFirstError(ctx, e.buildRunFuncsWithTimeout(cmd, logFile, containerName, maxPromptDuration)...); err != nil {
   ```

3. Verify that `watchForCompletionReport` returns `nil` on normal completion (it does -- confirmed in source). With `CancelOnFirstError`, this nil return will NOT cancel the other goroutines, which is the desired behavior.

4. Verify that `timeoutKiller` returns a non-nil error (`errors.Errorf(ctx, "prompt timed out after %s", duration)`) when the timeout fires. With `CancelOnFirstError`, this error WILL cancel the other goroutines and propagate up as the result.

5. Verify that `cmd.Run()` (the first goroutine) returns an error on non-zero exit. With `CancelOnFirstError`, this will cancel timeout and watcher, which is correct.

6. **Important behavioral change to validate**: After `watchForCompletionReport` returns nil (completion report found, container stopped), `cmd.Run()` should also return (because the container was stopped). The `cmd.Run()` return triggers either nil (exit 0) or error (non-zero exit). With `CancelOnFirstError`:
   - If cmd.Run() returns nil -> both watchers returned nil -> `CancelOnFirstError` waits for all to finish (since no error). This is fine because cmd.Run() finishing means the container exited.
   - If cmd.Run() returns error -> cancels remaining goroutines. This is correct.

7. Update any test comments that reference `CancelOnFirstFinish` to say `CancelOnFirstError` instead (search `executor_test.go` for mentions).

8. If any test assertions depend on the old `CancelOnFirstFinish` behavior (e.g., expecting cancellation on nil return), update them for the new `CancelOnFirstError` semantics.
</requirements>

<constraints>
- Do NOT commit -- dark-factory handles git
- Existing tests must still pass
- Only change the two `CancelOnFirstFinish` calls in `executor.go` -- do not change the `CancelOnFirstError` call in `runner.go`
- Do NOT modify `pkg/runner/health_check.go` -- that is handled by a separate prompt
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
