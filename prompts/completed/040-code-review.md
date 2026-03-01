---
status: completed
container: dark-factory-040-code-review
dark-factory-version: v0.9.1
---





# Fix compilation errors from code review

## Context

`make precommit` fails. Three compilation issues must be fixed together.

## Tasks

### 1. Fix `queueAllFiles` undefined

`pkg/server/queue_action_handler.go:49` references `queueAllFiles` which doesn't exist in `queue_helpers.go`.

Look at how `queueSingleFile` is defined in `queue_helpers.go` and add a matching `queueAllFiles` function that:
- Reads all `.md` files from `inboxDir`
- Calls `moveToQueue` for each one
- Returns `[]QueuedFile, error`

### 2. Fix `NewServer` signature mismatch

`pkg/factory/factory.go` calls `server.NewServer(addr, statusChecker)` with 2 args, but `server.go:49` requires 5 params: `addr`, `statusChecker`, `inboxDir`, `queueDir`, `promptManager`.

Fix `CreateServer` in `pkg/factory/factory.go` to pass all 5 args:
- `addr` (already computed)
- `statusChecker` (already created)
- `cfg.InboxDir`
- `cfg.QueueDir`
- A `prompt.Manager` instance (create via `prompt.NewManager(cfg.QueueDir, cfg.CompletedDir, git.NewReleaser())`)

Also fix `server_test.go` to match the 5-arg signature.

### 3. Generate missing mocks

Run `go generate ./...` to create:
- `mocks/server.go`
- `mocks/prompt-manager-server.go`
- `mocks/queue-command.go`
- `mocks/status-command.go`

## Verification

Run `make precommit` — must pass with zero errors.
