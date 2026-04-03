---
status: prompted
approved: "2026-04-03T09:20:52Z"
prompted: "2026-04-03T09:36:02Z"
branch: dark-factory/container-health-check
---

## Summary

- Daemon periodically checks that containers for `executing` prompts and `generating` specs are still alive
- If a container has disappeared (Docker daemon crash, `docker kill`, OOM), the prompt is reset to `approved` for automatic retry
- Uses existing container status detection — no new Docker dependencies
- Closes the gap between `ResumeExecuting` (startup-only) and runtime container loss

## Problem

When a container disappears during execution (Docker daemon restart, external `docker kill`, OOM kill), dark-factory doesn't notice. The prompt stays in `executing` (or spec in `generating`) state forever. The only recovery is restarting the daemon, which triggers `ResumeExecuting` at startup. If the daemon keeps running, the zombie prompt blocks the slot indefinitely and wastes overnight run time.

## Goal

After this work, the daemon detects disappeared containers within 30-60 seconds and resets affected prompts/specs to `approved` so they are automatically retried. No daemon restart or human intervention required.

## Non-goals

- Monitoring Docker daemon health itself (systemd/launchd handles that)
- Auto-retrying with backoff or retry limits (existing retry logic handles that after reset)
- Health checks for containers in other states (only `executing` and `generating`)
- Custom health check commands or endpoints inside the container
- Configurable poll interval (fixed 30s is sufficient; can be made configurable later)

## Desired Behavior

1. **Periodic health check**: The daemon detects within 30-60 seconds when an executing prompt's container is no longer running. All prompts in `executing` state are checked each cycle.

2. **Container gone → reset to approved**: When a container is confirmed not running, the prompt is reset to `approved` status — same behavior as `ResumeExecuting` at startup. A warning is logged with prompt name and container name.

3. **Spec generation health check**: Same check for specs in `generating` state. When the associated generation container is gone, the spec is reset to `approved` so the daemon regenerates.

4. **No-op when healthy**: If all containers are running, the health check completes silently (debug log only).

5. **Graceful degradation**: If `containerChecker.IsRunning()` itself fails (Docker daemon unreachable), log a warning and skip that prompt/spec. Do not reset on check failure — only on confirmed "not running".

6. **Context-aware**: Health check loop respects context cancellation for clean daemon shutdown. Does not interfere with normal prompt processing.

## Assumptions

- Container names are stored in prompt frontmatter (`container` field) during execution — already the case
- Resetting to `approved` is safe — the prompt will be picked up by the normal processing loop

## Constraints

- Reuse existing `containerChecker.IsRunning()` — no new Docker API calls or CLI invocations
- `containerChecker.IsRunning()` must be safe to call concurrently with prompt processing
- Generation containers follow the `dark-factory-gen-*` naming convention
- Must not interfere with normal prompt execution or the existing `ResumeExecuting` startup logic
- Health check must respect context cancellation (clean shutdown)
- All existing tests must pass, `make precommit` passes

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Container OOM-killed | Health check detects, resets prompt to approved | Prompt re-executes automatically |
| Docker daemon restarted | Container gone, health check detects, resets to approved | Prompt re-executes when Docker is back |
| `docker kill <container>` | Same as OOM | Same |
| Docker API unreachable during check | Log warning, skip cycle | Next cycle retries |
| Health check and normal completion race | Normal completion wins — health check sees non-executing status and skips | N/A |
| Multiple executing prompts, one dies | Only the dead container's prompt is reset | Others continue |

## Security

- No additional attack surface — reuses existing Docker socket access via `containerChecker`
- Container status check is read-only

## Do-Nothing Option

Zombie prompts require daemon restart to recover. Acceptable for rare Docker crashes but wastes hours during overnight runs when it happens. With 40 repos running, a Docker daemon hiccup can leave multiple prompts stuck.

## Acceptance Criteria

- [ ] Executing prompt with dead container is reset to `approved` within 60 seconds
- [ ] Generating spec with dead container is reset to `approved` within 60 seconds
- [ ] Warning log emitted when container disappearance is detected
- [ ] Docker API failure during check → warning log, no state change (prompt stays `executing`)
- [ ] Health check stops cleanly on daemon shutdown (no goroutine leak)
- [ ] All existing tests pass, `make precommit` passes

## Verification

```bash
make precommit
```

Manual verification:

1. Start daemon, approve a prompt, wait for `executing`
2. `docker kill <container>` externally
3. Within 30-60s, prompt resets to `approved` and re-executes
