---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-03-31T17:49:18Z"
prompted: "2026-03-31T17:55:15Z"
branch: dark-factory/global-container-limit
---

## Summary

- A global config at `~/.dark-factory/config.yaml` limits how many dark-factory containers run system-wide
- Daemons across all projects share this limit — prevents resource exhaustion when many projects run simultaneously
- Before starting a container, the daemon counts running containers via `docker ps` filtered by the existing `dark-factory.project` label
- If the count equals or exceeds `maxContainers`, the daemon waits and retries until a slot frees up
- Default `maxContainers: 3` when no global config file exists

## Problem

Running dark-factory across many projects simultaneously (e.g., 40 Go libraries each with their own daemon) can spawn dozens of containers. Each container runs Claude Code which consumes significant CPU, memory, and API quota. There is no system-wide limit — each daemon is unaware of containers from other projects.

Real-world scenario: initializing dark-factory in 40 `sm-octopus/lib/` repos and running `go mod update` across all of them. Without a global limit, 40 containers start simultaneously and the machine becomes unresponsive.

## Goal

After this work, dark-factory enforces a system-wide container limit. When 3 containers are already running (across any projects), a 4th daemon waits instead of starting another container. As containers finish, waiting daemons pick up slots. The limit is configurable via `~/.dark-factory/config.yaml`.

## Non-goals

- Per-project concurrency (separate spec: `parallel-execution.md`)
- Dynamic limit adjustment based on system resources
- Priority between projects (FIFO based on who polls first)
- Global config for anything other than `maxContainers` (keep minimal, extend later)
- Container resource limits (CPU/memory per container)

## Desired Behavior

1. **Global config file**: `~/.dark-factory/config.yaml` with optional field `maxContainers` (integer, min 1, default 3). If the file does not exist, default to `maxContainers: 3`.

   ```yaml
   maxContainers: 5
   ```

2. **Container counting**: Before starting a container, the daemon counts all running dark-factory containers system-wide. The existing Docker labels (`dark-factory.project`, `dark-factory.prompt`) are sufficient — no new labels needed.

3. **Wait loop**: If running container count >= `maxContainers`, the daemon logs a waiting message and periodically retries until a slot frees up.

4. **Slot acquisition**: When the count drops below `maxContainers`, the daemon proceeds to start its container. There is no reservation — two daemons may race, briefly exceeding the limit by 1. This is acceptable since the limit is a soft guideline for resource management, not a hard security boundary.

5. **No lock files**: Docker's running container list is the single source of truth. No file-based semaphores, no shared locks, no cleanup on crash. Exited/crashed containers are not counted — only running containers matter.

6. **Config loading**: The global config is loaded once at daemon startup. Changing the config requires restarting the daemon. The loader checks `~/.dark-factory/config.yaml` — if the file does not exist or is empty, all values use defaults.

7. **Status display**: `dark-factory status` shows the global limit and current system-wide container count:
   ```
   Dark Factory Status
     Project:    /path/to/project
     Containers: 2/3 (system-wide)
     Daemon:     running (pid 41936)
   ```

8. **Config display**: `dark-factory config` includes a `global:` section showing the resolved global config:
   ```yaml
   global:
     maxContainers: 3
   ```
   When `~/.dark-factory/config.yaml` does not exist, the defaults are still shown.

## Security

The global config file (`~/.dark-factory/config.yaml`) is trusted, user-owned, and read-only by dark-factory. No elevation or external input involved.

## Constraints

- Must not break existing behavior when no global config exists (default 3 is permissive enough)
- Must not introduce inter-process locking or shared state beyond Docker's own container list
- Container counting must use the existing `dark-factory.project` Docker labels
- The global config is independent of per-project `.dark-factory.yaml`
- `docker ps` must be available (dark-factory already requires Docker)
- All existing tests must pass
- `make precommit` must pass

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `~/.dark-factory/config.yaml` does not exist | Default `maxContainers: 3` | Expected, not an error |
| `~/.dark-factory/config.yaml` has invalid YAML | Startup error with clear message | User fixes file |
| `maxContainers: 0` or negative | Startup validation error | User fixes value |
| Docker not responding | `docker ps` fails, daemon logs error and retries | Docker recovers or user restarts |
| Two daemons race for last slot | Both start, briefly at limit+1 | Acceptable, self-corrects |
| Container stuck (not exiting) | Counts toward limit, blocks others | User manually stops container |

## Acceptance Criteria

- [ ] `~/.dark-factory/config.yaml` is read at daemon startup with `maxContainers` field
- [ ] Missing config file defaults to `maxContainers: 3`
- [ ] Daemon waits when system-wide container count >= limit
- [ ] Daemon proceeds when a slot frees up
- [ ] `dark-factory status` shows `Containers: N/M (system-wide)`
- [ ] No file-based locks or inter-process coordination introduced
- [ ] Invalid YAML or `maxContainers < 1` produces a clear startup error
- [ ] `dark-factory config` shows global `maxContainers` value
- [ ] Existing tests pass, `make precommit` passes

## Verification

```bash
make precommit
```

Manual verification:

1. No `~/.dark-factory/config.yaml`. Start 4 daemons in different projects. Observe: 3 containers run, 4th waits.
2. Create `~/.dark-factory/config.yaml` with `maxContainers: 1`. Restart daemons. Observe: only 1 container at a time.
3. Kill a running container. Observe: waiting daemon picks up slot within ~10s.
4. Run `dark-factory status` while containers are running. Observe: `Containers: 2/3 (system-wide)` line.

## Do-Nothing Option

Users manually stagger daemon starts or use `concurrency: 1` per project. With 40 projects, this means manually babysitting which daemons run. The global limit automates this with zero per-project config overhead.
