---
status: draft
---

## Summary

- Dark-factory periodically checks if the running Docker container is still alive and healthy
- Poll interval is configurable (default 1 minute)
- If the container has died (exited, OOM, removed), the prompt is marked as failed and a notification fires
- Health check runs as a background goroutine during prompt execution
- No action is taken if the container is alive — the existing stuck-container detection handles unresponsive containers

## Problem

During prompt execution, the Docker container can die silently (OOM kill, Docker daemon restart, manual `docker rm`). The existing stuck-container detection only watches log output for a completion marker — it cannot detect a container that has disappeared entirely. When this happens, dark-factory hangs indefinitely waiting for a container that no longer exists.

## Goal

After this work, dark-factory detects within one poll interval (default 60s) when a Docker container has died during prompt execution. The prompt is marked as failed, a notification fires, and the pipeline moves on to the next prompt. No human polling required.

## Non-Goals

- Restarting failed containers — mark failed and move on
- Health checks for containers between prompt executions — only during active execution
- Custom health endpoints inside the container — use `docker inspect` status only
- Configurable health check commands (exec into container)

## Desired Behavior

1. When a prompt begins executing, a background health check starts polling the container status
2. The poll interval is configurable via `healthCheckIntervalSeconds` in `.dark-factory.yaml` (default 60)
3. If `docker inspect` shows the container has exited with non-zero status, the prompt is marked as failed
4. If the container no longer exists (removed), the prompt is marked as failed
5. A notification fires when the health check detects a dead container
6. The health check stops when the prompt execution completes (success or failure from other paths)
7. If `docker inspect` itself fails (Docker daemon unreachable), log a warning but do not fail the prompt — retry on next interval

## Assumptions

- Docker CLI or API is accessible from the dark-factory process
- Container names or IDs are known during execution (already tracked by the runner)
- The existing `run.CancelOnFirstFinish` pattern can accommodate an additional concurrent goroutine

## Constraints

- Health check must not interfere with the existing stuck-container log watcher
- Health check must respect context cancellation (clean shutdown)
- Poll interval must not be less than 10 seconds to avoid Docker API spam
- Health check failure must trigger the same notification path as other prompt failures

## Security

- No additional attack surface — uses existing Docker socket access
- Container status check is read-only (`docker inspect`)

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---------|-------------------|----------|
| Container OOM-killed | Health check detects exit, marks prompt failed | Next prompt runs automatically |
| Docker daemon restarted | Container gone, health check detects, marks failed | Restart dark-factory |
| Container manually removed | Health check detects missing container, marks failed | Next prompt runs |
| Docker socket unreachable | Log warning, retry next interval | Fix Docker access |
| Health check and stuck-container detect simultaneously | First cancellation wins via context | No conflict |

## Do-Nothing Option

Dark-factory hangs indefinitely when a container dies silently. The human must notice the idle terminal, manually check `docker ps`, kill the dark-factory process, and reset the prompt. With overnight runs, a dead container can waste 8+ hours.

## Acceptance Criteria

- [ ] Health check detects exited containers and marks the prompt as failed
- [ ] Health check detects removed containers and marks the prompt as failed
- [ ] Poll interval is configurable (default 60s, minimum 10s)
- [ ] Health check stops when prompt execution completes normally
- [ ] Notification fires when health check detects a dead container
- [ ] Docker API errors are logged as warnings, not treated as failures
- [ ] No interference with existing stuck-container log watcher
- [ ] `make precommit` passes

## Verification

Run `make precommit` — must pass.
