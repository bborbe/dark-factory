---
status: completed
summary: Added removeContainerIfExists to docker executor that runs docker rm -f before each docker run to handle stale containers from interrupted runs, with test verifying command ordering.
container: dark-factory-081-remove-stale-container-before-run
dark-factory-version: v0.17.12
created: "2026-03-06T08:53:25Z"
queued: "2026-03-06T08:53:25Z"
started: "2026-03-06T08:53:25Z"
completed: "2026-03-06T09:01:15Z"
---

Fix: `docker run` fails with "container name already in use" when a previous run was interrupted (killed/crashed) and the container was not cleaned up. dark-factory should remove any existing container with the same name before starting a new one.

## Context

Read `pkg/executor/executor.go` before making changes.

The bug: `buildDockerCommand` uses `--rm` (auto-remove on exit) but if the process is killed mid-run, the container is left running or stopped but not removed. The next execution with the same container name fails with a Docker conflict error.

## Fix

In `Execute()`, before calling `buildDockerCommand`, add a cleanup step:

```go
// Remove any existing container with this name (handles interrupted previous runs)
if err := e.removeContainerIfExists(ctx, containerName); err != nil {
    slog.Warn("failed to remove existing container", "containerName", containerName, "error", err)
    // Non-fatal — proceed anyway, docker run will report the real error
}
```

Implement `removeContainerIfExists`:

```go
func (e *dockerExecutor) removeContainerIfExists(ctx context.Context, containerName string) error {
    cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerName)
    output, err := cmd.CombinedOutput()
    if err != nil {
        // docker rm -f returns non-zero if container doesn't exist — that's fine
        slog.Debug("docker rm -f", "containerName", containerName, "output", string(output))
    }
    return nil
}
```

Note: `docker rm -f` is idempotent — it silently succeeds even if the container doesn't exist (exit code may be non-zero but output is "Error: No such container"). Always return nil (non-fatal).

## Tests

Add a test to `executor_test.go` (or `executor_internal_test.go` if testing unexported method):
- When `Execute` is called, the command runner receives a `docker rm -f <containerName>` call before the `docker run` call.

Use Ginkgo v2. Match existing test style.

## Verification

Run `make precommit` — must pass.
