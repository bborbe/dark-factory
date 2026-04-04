---
status: verifying
approved: "2026-04-04T20:24:24Z"
generating: "2026-04-04T20:48:55Z"
prompted: "2026-04-04T20:53:45Z"
verifying: "2026-04-04T23:42:08Z"
branch: dark-factory/prompt-timeout-auto-retry
---

## Summary

- Containers that exceed a configurable time limit are killed automatically instead of running indefinitely.
- Failed prompts are retried up to a configurable limit with retry state tracked in frontmatter.
- After exhausting retries, the prompt is marked with a new `permanently_failed` status and the daemon moves on to the next prompt instead of stopping.
- Configurable timeout, retry limit, and failure tracking via project config and prompt metadata.

## Problem

When a container hangs or runs excessively long (agent retry loops, network issues, infinite waits), the daemon blocks forever on that prompt. The only recovery is manual intervention: check docker logs, kill the container, fix the prompt, retry. This breaks unattended operation and wastes compute. There is no timeout, no automatic retry, and a single failure stops the entire queue.

## Goal

After this work, dark-factory can run fully unattended for extended periods. Stuck containers are killed after a deadline, retried automatically, and — if retries are exhausted — skipped so the remaining queue continues processing. Human attention is only needed for prompts that fail repeatedly, not for transient issues.

## Assumptions

- Docker stop/kill reliably terminates containers (with force-kill as fallback).
- Wall-clock duration is a sufficient proxy for "stuck" — no idle detection needed.
- Frontmatter writes are atomic enough that a crash mid-write won't corrupt the file (single-field YAML update).

## Non-goals

- Partial timeout (e.g., "kill if idle for X minutes") — only wall-clock duration.
- Per-prompt timeout overrides in prompt frontmatter — project-level config only for now.
- Retry with modified prompt content — retries re-run the same prompt as-is.
- Backoff between retries — retries happen immediately.

## Desired Behavior

1. When a container's wall-clock runtime exceeds `maxPromptDuration`, the daemon kills the container and marks the prompt as `failed` with `lastFailReason` set to a human-readable timeout message including the duration. When `maxPromptDuration` is 0 or negative, timeout is disabled.

2. When `autoRetryLimit` is greater than 0 and a prompt fails (timeout or any other reason), the daemon increments `retryCount` in the prompt frontmatter and re-queues the prompt for execution, up to `autoRetryLimit` times.

3. When a prompt has been retried `autoRetryLimit` times and fails again, the daemon marks it with a new status `permanently_failed` instead of `failed`. This is a new status value that must be added to the existing status set.

4. When a prompt is `permanently_failed`, the daemon skips it and continues processing the next prompt in the queue. The daemon does NOT stop.

5. Notifications fire for both timeout kills and permanent failures. Notification includes prompt filename, failure reason, and retry count.

6. `dark-factory prompt retry` and `dark-factory prompt requeue` reset `retryCount` to 0 so the auto-retry budget is refreshed. This is new behavior — the current `requeue` implementation only calls `MarkApproved()`. Note: `retry` is an alias for `requeue --failed`.

## Constraints

- Existing `.dark-factory.yaml` files without the new fields must continue working (defaults apply).
- Existing prompt frontmatter without `lastFailReason` must parse without error (zero-value default). Note: `retryCount` already exists in prompt frontmatter.
- The `failed` status must still exist and work as before when `autoRetryLimit` is 0.
- When `autoRetryLimit` is 0 (default), failed prompts behave as they do today (prompt marked `failed`, daemon behavior unchanged).
- Config defaults allow zero-config upgrade: both `maxPromptDuration` and `autoRetryLimit` default to 0 (disabled). Projects opt in by setting values.
- All existing status consumers (queue view, notification formatting, CLI list) must handle the new `permanently_failed` status.
- Container kill must be clean: stop with grace period before force-kill, so the agent can flush logs.
- See `docs/configuration.md` for existing config field conventions (env var names for secrets, duration strings, defaults).

## Security

Container kill uses existing Docker API access — no new trust boundaries. Frontmatter writes are bounded to the prompt file. No user input, HTTP endpoints, or secrets involved.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Container kill fails (docker stop hangs) | Force-kill after grace period (e.g., 30s), then mark failed | If force-kill also fails, log error and mark permanently_failed |
| Frontmatter write fails during retry | Log error, do not retry (treat as permanent failure), continue queue | Human fixes file, uses `prompt retry` |
| Config has invalid duration string | Reject at daemon startup with clear error message | Human fixes config |
| `maxPromptDuration: 0` or negative | Treat as "no timeout" (disabled) | Intentional escape hatch |
| `autoRetryLimit: 0` (default) | No auto-retries, fail immediately (current behavior) | Intentional |
| Daemon restarts mid-retry | `retryCount` is persisted in frontmatter, so retry budget survives restarts | Automatic |

## Acceptance Criteria

- [ ] Container running longer than `maxPromptDuration` is killed and prompt marked failed
- [ ] Failed prompt is auto-retried up to `autoRetryLimit` times when `autoRetryLimit > 0`
- [ ] `retryCount` and `lastFailReason` are persisted in prompt frontmatter after each failure
- [ ] Prompt exceeding retry limit is marked `permanently_failed`
- [ ] Daemon continues processing queue after a `permanently_failed` prompt
- [ ] `dark-factory prompt requeue` and `dark-factory prompt retry` reset `retryCount` to 0
- [ ] Missing config fields default to 0 (disabled): `maxPromptDuration: ""`, `autoRetryLimit: 0`
- [ ] `maxPromptDuration: 0` disables the timeout
- [ ] Notification sent on timeout kill and on permanent failure (verified via unit test asserting notifier is called)
- [ ] `make precommit` passes

## Verification

```
make precommit
```

## Do-Nothing Option

Without this, stuck containers block the queue indefinitely. Operators must monitor manually and kill/retry by hand. Acceptable for single-prompt runs but not for unattended multi-prompt daemon sessions, which is the primary use case.
