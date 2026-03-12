---
status: draft
---

## Summary

- When dark-factory starts and finds prompts in `executing` state, it checks if the corresponding Docker container is still running
- If the container is alive, dark-factory re-attaches to it (tails logs, arms stuck-container watcher, waits for completion)
- If the container is gone, falls back to current behavior (reset to `approved` for re-execution)
- Works for both prompt execution and spec prompt generation
- No container restart or re-creation — purely re-attach to existing work

## Problem

When dark-factory is stopped (Ctrl+C, terminal close, crash) while a Docker container is executing a prompt, the container continues running. On restart, dark-factory calls `ResetExecuting` which sets all executing prompts back to `approved`, causing them to re-run from scratch. This wastes the work already done by the still-running container — potentially 30+ minutes of execution thrown away.

## Goal

After this work, restarting dark-factory while a container is still running does not discard in-progress work. Dark-factory detects the running container, re-attaches to it, and resumes monitoring until completion. The prompt lifecycle continues seamlessly as if dark-factory was never interrupted.

## Non-Goals

- Persisting dark-factory state to disk — container existence is the state
- Restarting dead containers — if the container is gone, reset to approved (current behavior)
- Resuming partial prompt content inside the container — only re-attach to monitoring
- Multi-container resume (parallel execution) — sequential execution only
- Resume across different machines or Docker hosts

## Desired Behavior

1. On startup, before `ResetExecuting`, check each prompt with `executing` status for a matching running container
2. Container matching uses a deterministic naming convention or label that includes the project name and prompt identifier
3. If a matching container is running, skip `ResetExecuting` for that prompt and re-attach: tail logs, arm stuck-container watcher, arm health check
4. If no matching container is found, reset the prompt to `approved` (existing behavior)
5. Re-attached execution produces the same completion flow (report parsing, status update, git operations) as a fresh execution
6. For PR workflow: verify the clone directory still exists before re-attaching — if missing, reset to approved
7. Log clearly whether each executing prompt was resumed or reset

## Assumptions

- Docker containers are named or labeled deterministically based on project + prompt identity
- The container's log stream can be tailed from the current position (not just from start)
- The clone directory for PR workflow survives dark-factory restarts (it's on the filesystem, not in memory)
- Only one prompt can be in `executing` state at a time (sequential processing)

## Constraints

- Container naming must be deterministic and collision-free across projects
- The `DARK-FACTORY-REPORT` completion marker must still be detectable even if early log output was missed
- Re-attach must not send duplicate git operations (commit, push, PR) if the original execution already completed them — but YOLO has no git access, so this is safe
- Must not change behavior when no executing prompts exist on startup (common case)
- The runner must expose enough information to reconstruct the execution context (container name, clone path, prompt path)

## Security

- No additional attack surface — uses existing Docker socket access
- Re-attach only monitors, does not inject commands into the container
- Container identity verification prevents attaching to wrong containers

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---------|-------------------|----------|
| Container finished between check and re-attach | Detect completed/exited status, process result normally | Automatic |
| Clone directory deleted during downtime (PR workflow) | Skip re-attach, reset prompt to approved | Re-executes from scratch |
| Container name collision (two projects, same prompt name) | Label includes full project path to prevent collision | By design |
| Docker daemon restarted (all containers gone) | No running containers found, reset all to approved | Current behavior |
| Container running but log stream fails | Health check detects alive container, stuck-container watcher retries | Automatic |
| Re-attach to container that already printed REPORT marker | Tail from current position, wait for container exit, parse last report | Automatic |

## Do-Nothing Option

Every dark-factory restart discards in-progress work. A prompt that's 25 minutes into a 30-minute execution gets reset and re-runs from scratch. With unstable terminals or SSH connections, this happens frequently. Over a week with 3-4 restarts, 1-2 hours of compute are wasted.

## Acceptance Criteria

- [ ] On startup, executing prompts with a running container are resumed, not reset
- [ ] On startup, executing prompts without a running container are reset to approved
- [ ] Resumed execution produces the same completion flow as fresh execution
- [ ] Container naming is deterministic and collision-free
- [ ] PR workflow: clone directory existence is verified before resume
- [ ] Logs clearly indicate whether each prompt was resumed or reset
- [ ] No behavioral change when no executing prompts exist on startup
- [ ] Works for both prompt execution and spec prompt generation containers
- [ ] `make precommit` passes

## Verification

Run `make precommit` — must pass.
