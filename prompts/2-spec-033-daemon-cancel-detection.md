---
status: created
spec: ["033"]
created: "2026-03-21T00:00:00Z"
---

<summary>
- The daemon watches the executing prompt's frontmatter via fsnotify while a container is running
- When a file write changes status to `cancelled`, the daemon gracefully stops the Docker container (`docker stop`)
- After stopping, the daemon removes the container (`docker rm`) for clean state
- The cancelled prompt file stays in `prompts/in-progress/` with `status: cancelled` — no further processing
- The daemon logs "prompt cancelled" and proceeds to pick up the next queued prompt
- Sequential processing is preserved: only one container runs at a time
</summary>

<objective>
Teach the processor to detect cancellation of the currently executing prompt by watching the frontmatter file via fsnotify during container execution. When cancelled, stop and remove the Docker container gracefully and proceed to the next queued prompt. This prompt depends on `1-spec-033-cancel-status-and-cli` (which adds `CancelledPromptStatus`).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — specifically `processPrompt` (line ~384) where `p.executor.Execute` is called blocking. The cancellation watcher must run concurrently with the executor.
Read `pkg/executor/executor.go` — `Execute` signature and `watchForCompletionReport` as a reference for the concurrent-goroutine pattern. Also see `removeContainerIfExists` (uses `docker rm -f`) and `watchForCompletionReport` (uses `docker stop`).
Read `pkg/prompt/prompt.go` — `prompt.Load`, `CancelledPromptStatus` (added by the preceding prompt).
Read `pkg/executor/command.go` — the `commandRunner` interface used by `watchForCompletionReport`.
</context>

<requirements>
1. In `pkg/processor/processor.go`, add a private method `watchForCancellation` using `fsnotify` (same pattern as `pkg/specwatcher/watcher.go`):
   ```go
   // watchForCancellation watches the prompt file for changes using fsnotify.
   // When the status changes to cancelled, it stops and removes the Docker container,
   // then cancels execCancel to unblock the executor.
   func (p *processor) watchForCancellation(
       ctx context.Context,
       execCancel context.CancelFunc,
       promptPath string,
       containerName string,
       cancelled *bool,
   ) {
       fsWatcher, err := fsnotify.NewWatcher()
       if err != nil {
           slog.Warn("failed to create cancel watcher", "error", err)
           return
       }
       defer fsWatcher.Close()

       if err := fsWatcher.Add(promptPath); err != nil {
           slog.Warn("failed to watch prompt file", "path", promptPath, "error", err)
           return
       }

       for {
           select {
           case <-ctx.Done():
               return
           case err, ok := <-fsWatcher.Errors:
               if !ok {
                   return
               }
               slog.Debug("cancel watcher error", "error", err)
           case event, ok := <-fsWatcher.Events:
               if !ok {
                   return
               }
               if event.Op&fsnotify.Write == 0 {
                   continue
               }
               pf, err := p.promptManager.Load(ctx, promptPath)
               if err != nil {
                   continue
               }
               if pf.Frontmatter.Status == string(prompt.CancelledPromptStatus) {
                   *cancelled = true
                   slog.Info("prompt cancelled, stopping container",
                       "file", filepath.Base(promptPath),
                       "container", containerName,
                   )
                   p.executor.StopAndRemoveContainer(ctx, containerName)
                   execCancel()
                   return
               }
           }
       }
   }
   ```
   Note: `fsnotify` is already a dependency (used by `pkg/specwatcher`). Watch the specific file, not the directory — react to `Write` events only.

2. In `pkg/executor/executor.go`, add a public method `StopAndRemoveContainer` (next to existing `removeContainerIfExists`):
   ```go
   // StopAndRemoveContainer gracefully stops a container (SIGTERM + 10s timeout) then removes it.
   func (e *executor) StopAndRemoveContainer(ctx context.Context, containerName string) {
       stopCmd := e.commandRunner.CommandContext(ctx, "docker", "stop", containerName)
       if err := stopCmd.Run(); err != nil {
           slog.Debug("docker stop", "container", containerName, "error", err)
       }
       e.removeContainerIfExists(containerName)
   }
   ```
   Add `StopAndRemoveContainer(ctx context.Context, containerName string)` to the `Executor` interface. This keeps all Docker operations in the executor package where `commandRunner` is already available and testable via counterfeiter.

3. In `pkg/processor/processor.go`, modify `processPrompt` to run `watchForCancellation` concurrently with `executor.Execute`:

   Replace the current executor call block (lines ~451-455):
   ```go
   // Execute via executor
   if err := p.executor.Execute(ctx, content, logFile, containerName); err != nil {
       slog.Info("docker container exited with error", "error", err)
       return errors.Wrap(ctx, err, "execute prompt")
   }
   ```
   With:
   ```go
   // Execute with cancellation watcher running concurrently
   execCtx, execCancel := context.WithCancel(ctx)
   var cancelledByUser bool
   go p.watchForCancellation(execCtx, execCancel, pr.Path, containerName, &cancelledByUser)

   execErr := p.executor.Execute(execCtx, content, logFile, containerName)
   execCancel() // always stop the watcher goroutine

   if cancelledByUser {
       slog.Info("prompt cancelled", "file", filepath.Base(pr.Path))
       return nil // proceed to next prompt; status is already set to cancelled
   }

   if execErr != nil {
       slog.Info("docker container exited with error", "error", execErr)
       return errors.Wrap(ctx, execErr, "execute prompt")
   }
   ```

   Note: the watcher goroutine exits when `execCtx` is cancelled (either by the user's cancel or by `execCancel()` after execution completes). The `cancelledByUser` boolean is written by the watcher goroutine and read after `execCancel()` returns — the goroutine sets it before calling `execCancel()`, so the happens-before relationship is satisfied.

4. Add tests in `pkg/processor/processor_test.go` or a new file `pkg/processor/cancel_test.go` (external package `processor_test`, Ginkgo/Gomega):
   - When the prompt status is set to `cancelled` while the executor is running (simulated via a mock executor that blocks until context is cancelled), `processExistingQueued` returns nil and does not call `handlePostExecution`.
   - Verify that after cancellation, the next queued prompt (if any) is picked up.
   - Use counterfeiter mocks for `executor.Executor` and `prompt.Manager` — do NOT introduce manual mocks.

5. Ensure `pkg/processor/processor_internal_test.go` covers `watchForCancellation`:
   - When context is already done, watcher exits immediately.
   - When prompt status becomes `cancelled`, watcher sets `cancelledByUser = true` and calls `execCancel`.
   - (Mock `stopAndRemoveContainer` by using a minimal docker stub or accepting that it fails gracefully in tests.)
</requirements>

<constraints>
- The daemon owns all Docker lifecycle operations — the CLI cancel command does NOT stop containers
- Existing prompt status transitions must not change — `cancelled` is handled after `execCancel()`, not as a fatal error
- All existing tests must continue to pass
- Sequential processing invariant preserved: only one prompt executes at a time; `cancelledByUser` path returns nil, allowing the outer loop to pick up the next prompt
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap(ctx, ...)` — never `fmt.Errorf`
- The `watchForCancellation` goroutine must exit cleanly when `execCtx.Done()` fires (no goroutine leak)
- `docker stop` / `docker rm` errors are logged at debug level and never surfaced as fatal — container may already be gone
- Copyright header required on any new files
- No `os/exec` in processor — all Docker operations go through `executor.StopAndRemoveContainer` which uses the existing `commandRunner` interface
</constraints>

<verification>
Run `make precommit` — must pass.
Run `go test ./pkg/processor/... -v` — all tests must pass.
</verification>
