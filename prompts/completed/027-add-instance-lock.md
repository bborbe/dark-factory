---
status: completed
container: dark-factory-027-add-instance-lock
---


# Add instance lock to prevent concurrent dark-factory runs

## Goal

Prevent two dark-factory instances from running on the same `prompts/` directory. Concurrent runs cause duplicate prompt processing, git conflicts, and duplicate version tags.

## Expected Behavior

```bash
# First instance starts normally
$ dark-factory
2026/03/01 15:00:00 dark-factory: acquired lock prompts/.dark-factory.lock
2026/03/01 15:00:00 dark-factory: watching prompts for queued prompts...

# Second instance fails immediately
$ dark-factory
2026/03/01 15:00:05 dark-factory: error: another instance is already running (pid 12345)
exit status 1
```

On shutdown (normal or crash), lock is automatically released by kernel.

## Implementation

### 1. Create `pkg/lock/` package

```go
//counterfeiter:generate -o ../../mocks/locker.go --fake-name Locker . Locker
type Locker interface {
    Acquire(ctx context.Context) error
    Release(ctx context.Context) error
}
```

Implementation:
- `Acquire()`: open `prompts/.dark-factory.lock`, call `syscall.Flock(fd, LOCK_EX|LOCK_NB)`, write PID to file
- If `LOCK_NB` (non-blocking) fails with `EWOULDBLOCK`: read PID from file, return error "another instance running (pid N)"
- `Release()`: unlock flock, remove lockfile (called on graceful shutdown)
- Kernel auto-releases flock on process exit (crash-safe)

### 2. Update runner

Acquire lock at start of `Run()`, release on shutdown. Use `defer` for cleanup.

### 3. Update factory

Add `CreateLocker(promptsDir string) Locker` to factory. Inject into runner.

### 4. Add lockfile to .gitignore

Add `.dark-factory.lock` to `.gitignore` so the lockfile is never committed.

### 5. Tests

- Acquire succeeds on first call
- Acquire fails when already locked (simulate with two file descriptors)
- Release removes lockfile
- PID is written to lockfile
- Lock works with non-existent directory (creates it)

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow `~/.claude-yolo/docs/go-patterns.md` (interface + struct + New*)
- Follow `~/.claude-yolo/docs/go-factory-pattern.md` (Create* in factory)
- Follow `~/.claude-yolo/docs/go-composition.md` (inject deps)
- Coverage ≥80% for new packages
