---
status: completed
container: dark-factory-034-add-status-cli
dark-factory-version: v0.6.1
---



# Add `dark-factory status` CLI subcommand

## Goal

Add a `status` subcommand that shows what dark-factory is doing by reading the filesystem and checking Docker. Works without the daemon running.

## Current Behavior

No CLI subcommands — `dark-factory` only runs the daemon. No way to check status without manually inspecting files.

## Expected Behavior

```bash
$ dark-factory status
Dark Factory Status
  Daemon:     running (pid 12345)
  Current:    023-fix-semver-sort.md (executing since 2m30s)
  Container:  dark-factory-023-fix-semver-sort (running)
  Queue:      2 prompts
    - fix-commit-completed-file.md
    - rename-prompt.md
  Completed:  22 prompts
  Ideas:      3 prompts
  Last log:   prompts/log/023-fix-semver-sort.log (1.2 KB)

$ dark-factory status --json
{"daemon":"running","current":"023-fix-semver-sort.md", ...}
```

When nothing is running:
```bash
$ dark-factory status
Dark Factory Status
  Daemon:     not running
  Current:    idle
  Queue:      0 prompts
  Completed:  22 prompts
  Ideas:      3 prompts
```

## Implementation

### 1. Add CLI argument parsing in `main.go`

Use `os.Args` (no framework needed for 2 subcommands):
- No args or `run` → current behavior (start daemon)
- `status` → show status and exit
- `status --json` → JSON output

### 2. Create `pkg/status/` package

```go
// Status represents the current dark-factory state.
type Status struct {
    DaemonRunning bool
    DaemonPID     int
    CurrentPrompt string
    ExecutingSince time.Duration
    ContainerName string
    ContainerRunning bool
    QueuedPrompts []string
    CompletedCount int
    IdeasCount    int
    LastLogFile   string
    LastLogSize   int64
}

//counterfeiter:generate -o ../../mocks/status-checker.go --fake-name StatusChecker . StatusChecker
type StatusChecker interface {
    Check(ctx context.Context) (*Status, error)
}
```

### 3. StatusChecker implementation

Reads from:
- `prompts/*.md` — count queued, find `status: executing` via frontmatter
- `prompts/completed/*.md` — count completed
- `prompts/ideas/*.md` — count ideas
- `prompts/log/*.log` — find latest log by mtime
- `docker ps --filter name=dark-factory --format '{{.Names}}'` — container status
- PID file or process list — daemon status (optional: check if port is open once REST is added)

### 4. Update factory

Add `CreateStatusChecker(promptsDir string) StatusChecker` to factory.

### 5. Tests

- Status with no prompts (empty dirs)
- Status with queued + executing + completed prompts
- Status with no daemon running
- JSON output format
- Container name detection

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow `~/.claude-yolo/docs/go-patterns.md` (interface + struct + New*)
- Follow `~/.claude-yolo/docs/go-factory-pattern.md` (Create* in factory)
- Follow `~/.claude-yolo/docs/go-composition.md` (inject deps)
- Coverage ≥80% for new packages
