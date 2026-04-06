---
status: draft
created: "2026-04-06T18:00:00Z"
---

<summary>
- Executing prompts that exceed maxPromptDuration are now stopped by the health check loop
- The health check reads the `started` timestamp from prompt frontmatter and compares against maxPromptDuration
- Timed-out containers are stopped via `docker stop` and the prompt is marked failed with a descriptive reason
- This mechanism is immune to macOS App Nap because the health check runs in the main daemon process kept alive by docker I/O
- The `checkExecutingPrompts` function gains two new parameters: maxPromptDuration and a container stopper interface
- New tests verify the timeout detection and container stop behavior
</summary>

<objective>
Add maxPromptDuration enforcement to the health check loop so that prompts running longer than the configured limit are stopped and marked failed, even when macOS App Nap delays goroutine timers in the executor process.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/runner/health_check.go` -- `checkExecutingPrompts` function and `runHealthCheckLoop`
- `pkg/runner/runner.go` -- `healthCheckLoop` method and `runner` struct (shows how deps are wired)
- `pkg/runner/export_test.go` -- test export wrappers (must be updated for new params)
- `pkg/runner/health_check_test.go` -- existing tests (must still pass, add new ones)
- `pkg/prompt/prompt.go` -- `Frontmatter` struct (has `Started` field, RFC3339 string), `MarkFailed`, `SetLastFailReason`
- `pkg/config/config.go` -- `ParsedMaxPromptDuration()` returns `time.Duration`
- `pkg/executor/executor.go` -- `StopAndRemoveContainer` on `Executor` interface; `ContainerChecker` interface in `checker.go`
- `pkg/factory/` -- where `NewRunner` is called (must pass new param)

Also read:
- `~/Documents/workspaces/coding-guidelines/go-architecture-patterns.md` for interface patterns
- `~/Documents/workspaces/coding-guidelines/go-testing-guide.md` for Ginkgo test patterns
</context>

<requirements>
1. **Add a `ContainerStopper` interface** to `pkg/executor/checker.go` (or a new file `pkg/executor/stopper.go`):
   ```go
   //counterfeiter:generate -o ../../mocks/container-stopper.go --fake-name ContainerStopper . ContainerStopper

   type ContainerStopper interface {
       StopContainer(ctx context.Context, name string) error
   }
   ```
   Implement it with a `dockerContainerStopper` struct that runs `docker stop <name>`. Generate the counterfeiter mock.

2. **Update `checkExecutingPrompts` signature** in `pkg/runner/health_check.go` to accept two new parameters:
   - `maxPromptDuration time.Duration` -- the configured limit (0 = disabled)
   - `stopper executor.ContainerStopper` -- to stop timed-out containers

3. **Add timeout check logic** inside the `checkExecutingPrompts` loop, AFTER the existing `if running` check (container is running but may have exceeded the duration). The logic:
   ```go
   // After confirming container is running:
   if maxPromptDuration > 0 && pf.Frontmatter.Started != "" {
       started, err := time.Parse(time.RFC3339, pf.Frontmatter.Started)
       if err == nil && time.Since(started) > maxPromptDuration {
           slog.Warn("health check: prompt exceeded maxPromptDuration, stopping",
               "file", entry.Name(),
               "container", containerName,
               "started", pf.Frontmatter.Started,
               "maxPromptDuration", maxPromptDuration,
               "elapsed", time.Since(started),
           )
           if err := stopper.StopContainer(ctx, containerName); err != nil {
               slog.Warn("health check: failed to stop timed-out container",
                   "container", containerName, "error", err)
           }
           pf.MarkFailed()
           pf.SetLastFailReason(fmt.Sprintf("exceeded maxPromptDuration (%s)", maxPromptDuration))
           if err := pf.Save(ctx); err != nil {
               slog.Warn("health check: failed to save timed-out prompt",
                   "file", entry.Name(), "error", err)
           }
           if n != nil {
               _ = n.Notify(ctx, notifier.Event{
                   ProjectName: projectName,
                   EventType:   "prompt_timeout",
                   PromptName:  entry.Name(),
               })
           }
           continue
       }
   }
   ```

4. **Update `runHealthCheckLoop`** to accept and pass through `maxPromptDuration` and `stopper` to `checkExecutingPrompts`.

5. **Update `runner.healthCheckLoop`** method in `pkg/runner/runner.go`:
   - Add `maxPromptDuration time.Duration` and `stopper executor.ContainerStopper` fields to the `runner` struct
   - Pass them through in `healthCheckLoop()` call to `runHealthCheckLoop`

6. **Update `NewRunner`** constructor in `pkg/runner/runner.go` to accept `maxPromptDuration time.Duration` and `stopper executor.ContainerStopper` parameters.

7. **Update the factory** (find the `NewRunner` call site in `pkg/factory/`) to pass `cfg.ParsedMaxPromptDuration()` and a new `executor.NewDockerContainerStopper()` instance.

8. **Update `export_test.go`** -- both `CheckExecutingPromptsForTest` and `RunHealthCheckLoopForTest` must accept and pass through the new parameters.

9. **Update existing tests** in `health_check_test.go` -- all calls to `CheckExecutingPromptsForTest` and `RunHealthCheckLoopForTest` must pass the new params (use `0` for maxPromptDuration and `nil` for stopper to preserve existing behavior).

10. **Add new tests** in `health_check_test.go`:
    - "stops and marks failed when prompt exceeds maxPromptDuration": create an executing prompt with `started` timestamp 2 hours ago, set maxPromptDuration to 1 hour. Verify: stopper.StopContainer called, prompt status is "failed", lastFailReason contains "exceeded maxPromptDuration".
    - "does not stop when maxPromptDuration is 0 (disabled)": same setup but maxPromptDuration=0. Verify stopper not called.
    - "does not stop when prompt has no started timestamp": executing prompt with empty Started field. Verify stopper not called.
    - "does not stop when prompt is within maxPromptDuration": started 10 minutes ago, maxPromptDuration 1 hour. Verify stopper not called, prompt stays executing.

11. Run `go generate ./pkg/executor/...` to generate the counterfeiter mock for ContainerStopper.
</requirements>

<constraints>
- Do NOT commit -- dark-factory handles git
- Existing tests must still pass
- Follow the existing code pattern: Interface -> Constructor -> Struct -> Method
- Use `fmt.Sprintf` for the lastFailReason string (import "fmt" if needed)
- The `time.Since(started)` comparison uses wall-clock time which is correct for this use case
- Do NOT modify `pkg/executor/executor.go` -- that is handled by a separate prompt
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
