---
status: completed
spec: [034-resume-executing-on-restart]
summary: Skip handlePromptFailure on daemon shutdown so prompt stays in executing state for resume-on-restart, and return early from processPrompt when ctx is cancelled to leave container running.
container: dark-factory-215-fix-rm-flag-prevents-resume
dark-factory-version: v0.67.1
created: "2026-03-21T20:33:08Z"
queued: "2026-03-21T20:33:08Z"
started: "2026-03-21T20:33:18Z"
completed: "2026-03-21T20:38:19Z"
---

<summary>
- Daemon shutdown no longer marks executing prompts as failed
- Prompt stays in `executing` state when daemon is killed, enabling resume on restart
- `processExistingQueued` skips `handlePromptFailure` when context is cancelled
- `processPrompt` returns early on shutdown, skipping post-execution cleanup
- Normal container failures still mark prompt as `failed` as before
</summary>

<objective>
Fix the bug that prevents resume-on-restart: when daemon receives SIGTERM, `handlePromptFailure` marks the prompt as `failed` even though the container is still running. After this fix, daemon shutdown skips `handlePromptFailure` so the prompt stays in `executing` state and `resumeOrResetExecuting` (in `pkg/runner/lifecycle.go`) can reattach on restart.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — find `processExistingQueued` (~line 388 where `handlePromptFailure` is called) and `processPrompt` (~line 599 where `execErr` is checked).
</context>

<requirements>
1. In `pkg/processor/processor.go`, function `processExistingQueued`, where `handlePromptFailure` is called after `processPrompt` returns error (~line 389): only call `handlePromptFailure` when `ctx.Err() == nil`. When `ctx.Err() != nil` (daemon shutting down), log "daemon shutting down, prompt stays executing" and return the error without marking the prompt as failed. The prompt stays in `executing` status with the container still running.

2. In `pkg/processor/processor.go`, function `processPrompt`, after the `Execute` call (~line 599, the `if execErr != nil` block): when `ctx.Err() != nil`, log "daemon shutting down, leaving container running" and return the wrapped error via `errors.Wrap(ctx, execErr, "execute prompt")`. This early return skips any post-execution cleanup. Note: check `ctx` (the parent context from the daemon), not `execCtx` (the child context used for cancel-watcher).
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- No `os/exec` in processor — all Docker ops go through executor
- Use `errors.Wrap(ctx, err, ...)` from `github.com/bborbe/errors`, not `errors.Wrapf`
- `--rm` flag on `docker run` is fine — when `docker run` CLI is killed, the container keeps running; `--rm` only triggers on container exit
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
