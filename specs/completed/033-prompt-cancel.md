---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-03-21T15:03:05Z"
prompted: "2026-03-21T15:12:28Z"
verifying: "2026-03-21T15:54:23Z"
completed: "2026-03-21T16:09:55Z"
branch: dark-factory/prompt-cancel
---

## Summary

- Users can cancel a running or approved prompt via `dark-factory prompt cancel <id>`
- The CLI sets the prompt status to `cancelled`; the daemon handles container cleanup
- When the daemon detects a `cancelled` status on the currently executing prompt, it stops and removes the Docker container
- Cancelled prompts can be re-queued with `dark-factory prompt requeue`
- A new `cancelled` status is added to the prompt lifecycle

## Problem

There is no way to stop a prompt that is currently executing or remove an approved prompt from the queue without manually editing frontmatter (which violates project conventions). If a user approves the wrong prompt, spots an error mid-execution, or wants to reprioritize, they must wait for the container to finish or manually kill it. This is error-prone and leaves the prompt in an inconsistent state.

## Goal

After this work, a user can cancel any `approved` or `executing` prompt with a single CLI command. The daemon detects the cancellation, stops the container gracefully, and moves on to the next queued prompt. Cancelled prompts can be re-queued later.

## Non-Goals

- Reverting code changes made by a cancelled container (user decides manually)
- Auto-requeueing cancelled prompts
- Cancelling `completed` prompts (they are immutable)
- Undoing partial git commits from the cancelled execution

## Desired Behavior

1. `dark-factory prompt cancel <id>` sets the prompt's frontmatter status to `cancelled`
2. The command only works on prompts with status `approved` or `executing`; it returns an error for `completed`, `failed`, `cancelled`, or any other status
3. When the daemon's processor detects that the currently executing prompt has status `cancelled`, it gracefully stops the Docker container (`docker stop`) and removes it (`docker rm`)
4. After stopping the container, the daemon logs "prompt <id> cancelled" and proceeds to the next queued prompt
5. The cancelled prompt file stays in `prompts/in-progress/` with `status: cancelled`
6. `dark-factory prompt requeue <id>` accepts `cancelled` prompts (same as `failed`), setting status back to `approved`
7. `dark-factory prompt list` and `dark-factory status` display `cancelled` prompts correctly

## Assumptions

- The daemon can detect frontmatter changes on the currently executing prompt (either via filesystem polling or fsnotify, same mechanism used for the watcher)
- The container name is deterministic and known from the prompt metadata (already true)
- `docker stop` sends SIGTERM and waits for the default timeout (10s) before SIGKILL

## Constraints

- The cancel CLI command must NOT stop containers or interact with Docker directly -- it only changes frontmatter status
- The daemon owns all Docker lifecycle operations (separation of concerns)
- Existing prompt status transitions must not change -- `cancelled` is additive
- All existing tests must continue to pass
- The `autoSetQueuedStatus` logic must not auto-promote `cancelled` prompts back to `approved`
- Sequential processing invariant preserved: only one prompt executes at a time

## Security

- No new attack surface -- cancel command operates on local files only
- Container stop uses the same Docker socket access already available to the daemon

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---------|-------------------|----------|
| Cancel a prompt that is not `approved` or `executing` | CLI returns error with current status | User must use correct command for the status |
| Cancel while container is mid-write to filesystem | Container receives SIGTERM, partial changes remain on disk | User reviews and reverts manually or re-queues |
| Daemon crashes after cancel but before container stop | On restart, daemon sees `cancelled` status, stops orphaned container | Existing stale-container cleanup handles this |
| Cancel an `approved` prompt before daemon picks it up | Daemon skips `cancelled` prompts during queue scan | No action needed |
| File ID does not match any prompt | CLI returns "prompt not found" error | User checks `dark-factory prompt list` |
| Race: daemon starts executing between user's status check and cancel write | Cancel still works -- daemon polls status during execution | Container stops on next poll cycle |

## Acceptance Criteria

- [ ] `dark-factory prompt cancel <id>` sets status to `cancelled` on an `approved` prompt
- [ ] `dark-factory prompt cancel <id>` sets status to `cancelled` on an `executing` prompt
- [ ] `dark-factory prompt cancel <id>` returns an error on `completed`, `failed`, or `cancelled` prompts
- [ ] Daemon stops and removes the Docker container when it detects `cancelled` on the executing prompt
- [ ] Daemon logs cancellation and proceeds to next queued prompt
- [ ] Cancelled prompt file remains in `prompts/in-progress/`
- [ ] `dark-factory prompt requeue` works on `cancelled` prompts
- [ ] `dark-factory prompt list` shows `cancelled` status
- [ ] `autoSetQueuedStatus` does not override `cancelled` status
- [ ] `make precommit` passes

## Verification

```
make precommit
```

## Do-Nothing Option

Users must manually `docker stop` the container and edit frontmatter to `failed`, then requeue. This is error-prone, violates the "never edit frontmatter manually" convention, and risks leaving the daemon in an inconsistent state. Not acceptable for regular use.
