---
status: created
spec: ["034"]
created: "2026-03-21T18:00:00Z"
branch: dark-factory/resume-executing-on-restart
---

<summary>
- New ContainerChecker interface and Docker implementation determine whether a named container is currently running
- On startup, executing prompts are selectively preserved or reset based on container liveness — not blindly reset
- Prompts whose container is still running are logged as "resuming" and left in executing state for the next prompt (prompt 2 handles the actual re-attach)
- Prompts whose container is gone are logged as "resetting", have stuck_container notifications fired, and are reset to approved — identical to the current behavior
- The runner no longer fires stuck_container for prompts that will be resumed
- No behavioral change when no executing prompts exist on startup (the common case)
- Container name is read from the prompt frontmatter, which is already persisted at execution start
</summary>

<objective>
Introduce a ContainerChecker interface that queries Docker for container liveness, then use it in runner startup to selectively reset or preserve executing prompts. Prompts with a live container are left in "executing" state (to be resumed by prompt 2); prompts without one are reset to approved as before.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read all `/home/node/.claude/docs/go-*.md` docs before starting.
Read `pkg/executor/executor.go` — Executor interface, container name format, existing `removeContainerIfExists`.
Read `pkg/runner/runner.go` — `Run()`, `notifyStuckContainers()`, `ResetExecuting` call (line ~134).
Read `pkg/prompt/prompt.go` — `Frontmatter.Container` field (line ~184), `ResetExecuting` function (line ~696), `ExecutingPromptStatus`.
Read `pkg/factory/factory.go` — `CreateRunner` (line ~234) and `CreateSpecGenerator` (line ~312) to understand wiring.
Read `mocks/executor.go` — the counterfeiter-generated mock, to understand the pattern.
</context>

<requirements>
1. **Create `pkg/executor/checker.go`** — a new file with:
   - Interface:
     ```go
     //counterfeiter:generate -o ../../mocks/container-checker.go --fake-name ContainerChecker . ContainerChecker

     // ContainerChecker checks whether a Docker container is currently running.
     type ContainerChecker interface {
         IsRunning(ctx context.Context, name string) (bool, error)
     }
     ```
   - Implementation `dockerContainerChecker` (private struct, zero fields):
     ```go
     // NewDockerContainerChecker creates a ContainerChecker backed by docker inspect.
     func NewDockerContainerChecker() ContainerChecker {
         return &dockerContainerChecker{}
     }
     ```
   - `IsRunning` implementation using `docker inspect --format='{{.State.Running}}' <name>`:
     - Run the command; capture stdout
     - If exit code non-zero → return `false, nil` (container does not exist)
     - If stdout == `"true"` → return `true, nil`
     - Otherwise → return `false, nil`
     - Use `exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Running}}", name)`
     - Capture stdout with `strings.Builder`; trim whitespace before comparing
     - Errors from `cmd.Run()` when container doesn't exist are expected — treat as `false, nil`
   - Add copyright header; add GoDoc comments.
   - Regenerate the mock: `go generate ./pkg/executor/...` (or run counterfeiter manually if needed)

2. **Modify `pkg/runner/runner.go`**:
   - Add `containerChecker executor.ContainerChecker` field to the `runner` struct.
   - Add `containerChecker executor.ContainerChecker` parameter to `NewRunner` (after `projectName`, before `n notifier.Notifier`).
   - Replace the two-step startup sequence in `Run()`:
     ```go
     // OLD:
     r.notifyStuckContainers(ctx)
     if err := r.promptManager.ResetExecuting(ctx); err != nil { ... }
     ```
     with a single call:
     ```go
     if err := r.resumeOrResetExecuting(ctx); err != nil {
         return errors.Wrap(ctx, err, "resume or reset executing prompts")
     }
     ```
   - **Remove `notifyStuckContainers`** entirely (its logic is subsumed by the new method).
   - Add the new method `resumeOrResetExecuting(ctx context.Context) error` to runner:
     ```go
     func (r *runner) resumeOrResetExecuting(ctx context.Context) error {
         entries, err := os.ReadDir(r.inProgressDir)
         if err != nil {
             if os.IsNotExist(err) {
                 return nil
             }
             return errors.Wrap(ctx, err, "read in-progress dir")
         }
         for _, entry := range entries {
             if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
                 continue
             }
             path := filepath.Join(r.inProgressDir, entry.Name())
             pf, err := r.promptManager.Load(ctx, path)
             if err != nil || pf == nil {
                 continue
             }
             fm := pf.Frontmatter
             if prompt.PromptStatus(fm.Status) != prompt.ExecutingPromptStatus {
                 continue
             }
             containerName := fm.Container
             running, err := r.containerChecker.IsRunning(ctx, containerName)
             if err != nil {
                 slog.Warn("failed to check container liveness, resetting prompt",
                     "file", entry.Name(), "container", containerName, "error", err)
                 running = false
             }
             if running {
                 slog.Info("resuming prompt, container still running",
                     "file", entry.Name(), "container", containerName)
                 // Leave prompt in executing state — prompt 2 handles re-attach
             } else {
                 slog.Info("resetting prompt, container not found",
                     "file", entry.Name(), "container", containerName)
                 _ = r.notifier.Notify(ctx, notifier.Event{
                     ProjectName: r.projectName,
                     EventType:   "stuck_container",
                     PromptName:  entry.Name(),
                 })
                 pf.MarkApproved()
                 if err := pf.Save(ctx); err != nil {
                     return errors.Wrap(ctx, err, "reset executing prompt")
                 }
             }
         }
         return nil
     }
     ```
   - Also apply the same logic to `r.specsInProgressDir` — spec files may track executing containers in future, but for now the directory scan can be a no-op if no spec files have `executing` status (the prompt.Manager's Load works for spec files too, but they use a different struct; skip specs for this prompt — just scan `inProgressDir`).

3. **Update `pkg/factory/factory.go`**:
   - In `CreateRunner`, pass `executor.NewDockerContainerChecker()` as the new `containerChecker` argument before `n` (the notifier). The signature of `runner.NewRunner` now has this extra parameter.
   - In `CreateOneShotRunner`, do the same: pass `executor.NewDockerContainerChecker()` to `runner.NewOneShotRunner` if it also calls `resumeOrResetExecuting` — check `pkg/runner/oneshot.go` first; if `OneShotRunner` also calls `ResetExecuting`, apply the same change there.

4. **Tests in `pkg/runner/runner_test.go`**:
   - Add a test for `resumeOrResetExecuting`:
     - **Case: no executing prompts** → no container check, no reset, returns nil
     - **Case: executing prompt, container running** → `IsRunning` returns `true` → prompt stays in executing state, no notification fired
     - **Case: executing prompt, container not running** → `IsRunning` returns `false` → prompt reset to approved, stuck_container notification fired
     - **Case: executing prompt, container name empty** → treat as not running → reset to approved
     - Use counterfeiter mocks for `ContainerChecker`, `promptManager`, and `notifier`.

5. **Regenerate mocks**: After adding the counterfeiter annotation, run:
   ```
   go generate ./pkg/executor/...
   ```
   This creates `mocks/container-checker.go`.

6. **Add copyright header** to `pkg/executor/checker.go`.
</requirements>

<constraints>
- Container naming must remain deterministic and collision-free — the existing name stored in `frontmatter.Container` is the canonical name
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `IsRunning` must never return an error when the container simply doesn't exist — that is a `false, nil` result
- The `resumeOrResetExecuting` method must be a no-op (return nil) when `r.inProgressDir` does not exist
- `#nosec G204` comment required on any `exec.Command` with a variable argument (containerName), with reason: "containerName is generated internally from prompt filename"
- Do not change any behavior for non-executing prompts
- Follow existing error-wrapping style: `errors.Wrapf(ctx, err, ...)` from `github.com/bborbe/errors`
</constraints>

<verification>
Run `make precommit` — must pass with exit code 0.
Spot-check: `grep -r "IsRunning" pkg/executor/` should show the interface and implementation.
Spot-check: `grep -r "resumeOrResetExecuting" pkg/runner/` should show the method.
Spot-check: `ls mocks/container-checker.go` should exist.
</verification>
