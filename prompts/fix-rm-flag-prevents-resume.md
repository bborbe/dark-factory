---
spec: ["034"]
status: draft
---

<summary>
- Containers survive daemon shutdown so resume-on-restart can reattach
- Docker `--rm` flag removed; explicit container cleanup added on success and failure
- Daemon shutdown leaves executing prompt in `executing` state with container still running
- Normal container failures still mark prompt as `failed` and clean up the container
- Existing container cleanup at start of Execute preserved for stale containers
</summary>

<objective>
Fix the bug that prevents resume-on-restart from working: `docker run --rm` kills the container when the daemon shuts down, and `handlePromptFailure` marks the prompt as `failed`. After this change, daemon shutdown leaves both the container running and the prompt in `executing` state, enabling `resumeOrResetExecuting` to reattach on restart.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/executor/executor.go` — find `buildDockerCommand` (the `--rm` flag) and `StopAndRemoveContainer`.
Read `pkg/processor/processor.go` — find `processPrompt` (lines around `execErr`/`handlePromptFailure`) and `handlePostExecution`.
</context>

<requirements>
1. In `pkg/executor/executor.go`, function `buildDockerCommand`: remove `"--rm"` from the docker args slice. The `"run"` arg stays, only `"--rm"` is removed.

2. In `pkg/executor/executor.go`, add a new method `RemoveContainer(ctx context.Context, containerName string)` to the `Executor` interface and `dockerExecutor`. It should call the existing `removeContainerIfExists` method. This is the explicit cleanup counterpart to removing `--rm`.

3. In `pkg/processor/processor.go`, function `processPrompt`, after the `Execute` call (around the `if execErr != nil` block): add a check for daemon shutdown. If `ctx.Err() != nil` (context was cancelled), log "daemon shutting down, leaving container running" and return the error WITHOUT calling container cleanup. The prompt stays in `executing` status because `handlePromptFailure` in the caller is only called when the error is NOT a shutdown.

4. In `pkg/processor/processor.go`, function `processExistingQueued`, where `handlePromptFailure` is called after `processPrompt` returns error: wrap the call so it only runs when `ctx.Err() == nil`. When `ctx.Err() != nil`, just return the error — the prompt stays `executing` and the container stays running.

5. In `pkg/processor/processor.go`, function `handlePostExecution` (success path): after all git operations complete successfully, call `p.executor.RemoveContainer` to clean up the stopped container.

6. In `pkg/processor/processor.go`, function `handlePromptFailure`: after marking the prompt as failed, call `p.executor.RemoveContainer` to clean up the container. Use the container name from the prompt's frontmatter. Load it via `p.promptManager.Load` (already done in that function).

7. Update all tests that mock the `Executor` interface to include the new `RemoveContainer` method. Check `pkg/executor/executor_internal_test.go` and any counterfeiter fakes.

8. In `pkg/executor/executor.go`, function `Execute`: the existing `removeContainerIfExists` call at the start is still needed — it handles stale containers from previous interrupted runs. Keep it as-is.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- No `os/exec` in processor — all Docker ops go through executor
- Use `errors.Wrap(ctx, err, ...)` from `github.com/bborbe/errors`, not `errors.Wrapf`
- `handlePromptFailure` needs the container name — extract from prompt frontmatter (`pf.Frontmatter.Container`)
- The `handlePostExecution` function uses `context.WithoutCancel` — container removal should use that same detached context
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
