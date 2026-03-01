---
status: completed
---

# Instance Locking: Prevent Concurrent Runs

## Problem

Running two dark-factory instances in the same project causes race conditions: both pick the same prompt, both try to commit, git conflicts. Nothing prevents accidental double-launch.

## Goal

Only one dark-factory instance runs per project directory. A second instance fails immediately with a clear error showing which process holds the lock.

## Non-goals

- No distributed locking (single machine only)
- No lock timeout or auto-expiry
- No graceful takeover (old instance must exit first)

## Desired Behavior

1. On startup, acquire an exclusive file lock on `.dark-factory.lock` in the project root
2. Lock file stores the PID of the owning process
3. If lock is already held, startup fails with error: "dark-factory already running (PID: NNNN)"
4. Lock is released on clean shutdown (SIGINT/SIGTERM)
5. Stale lock files (process no longer running) are handled by the OS flock mechanism

## Constraints

- Uses OS-level `flock` (not advisory file presence checks)
- Lock file permissions: 0600
- Lock file location is not configurable (always project root)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Lock already held | Startup failure with PID info | Wait for other instance or kill it |
| Process crashes without cleanup | OS releases flock automatically | Next startup succeeds |
| Lock file exists but no flock held | Acquire succeeds (stale file) | None needed |

## Acceptance Criteria

- [ ] First instance starts successfully, creates `.dark-factory.lock`
- [ ] Second instance fails immediately with PID of first
- [ ] Lock released on clean shutdown
- [ ] Stale lock files don't block new instances

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Hope users don't accidentally run two instances. In practice, this happens when switching terminals or restarting without checking.
