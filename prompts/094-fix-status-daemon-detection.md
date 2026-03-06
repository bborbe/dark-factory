---
status: created
---

Fix: `dark-factory status` always shows "Daemon: not running" even when a dark-factory process is active.

## Context

Read `pkg/status/status.go` and `pkg/lock/locker.go` before making changes.

The daemon detection should check `.dark-factory.lock` for a PID and verify the process is alive.

## Fix

In `pkg/status/status.go` (or wherever the daemon status is determined):

1. Read `.dark-factory.lock` file
2. Parse PID from contents
3. Check if process is alive: `os.FindProcess(pid)` + `process.Signal(syscall.Signal(0))`
4. If alive: `Daemon: running (pid NNNNN)`
5. If lock file missing or PID dead: `Daemon: not running`

The lock file path is in the project root (current working directory): `.dark-factory.lock`

## Tests

Add test for daemon detection logic. Use Ginkgo v2, match existing style.

## Verification

Run `make precommit` — must pass.
