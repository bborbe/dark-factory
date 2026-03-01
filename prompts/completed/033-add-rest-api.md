---
status: completed
container: dark-factory-033-add-rest-api
dark-factory-version: v0.6.1
---



# Add REST API for real-time status and monitoring

## Goal

Add an HTTP server to dark-factory that exposes status, metrics, and control endpoints. Enables remote monitoring, dashboards, and future webhook integrations.

## Prerequisites

- `add-status-cli` prompt should be implemented first (shares `StatusChecker` interface)

## Expected Behavior

```bash
# Health check
$ curl http://localhost:8080/health
{"status":"ok"}

# Full status (same data as CLI)
$ curl http://localhost:8080/api/v1/status
{
  "daemon": "running",
  "current_prompt": "023-fix-semver-sort.md",
  "executing_since": "2m30s",
  "container": "dark-factory-023-fix-semver-sort",
  "queue_count": 2,
  "queued_prompts": ["fix-commit-completed-file.md", "rename-prompt.md"],
  "completed_count": 22,
  "ideas_count": 3
}

# Queue (list queued prompts with details)
$ curl http://localhost:8080/api/v1/queue
[{"name": "fix-commit-completed-file.md", "title": "Fix CommitCompletedFile", "size": 1234}]

# Completed (list recent completed prompts)
$ curl http://localhost:8080/api/v1/completed?limit=10
[{"name": "023-fix-semver-sort.md", "completed_at": "2026-03-01T14:30:00Z"}]
```

## Implementation

### 1. Create `pkg/server/` package

```go
//counterfeiter:generate -o ../../mocks/server.go --fake-name Server . Server
type Server interface {
    ListenAndServe(ctx context.Context) error
}

type server struct {
    addr          string
    statusChecker status.StatusChecker
    promptManager prompt.Manager
}

func NewServer(
    addr string,
    statusChecker status.StatusChecker,
    promptManager prompt.Manager,
) Server {
    return &server{
        addr:          addr,
        statusChecker: statusChecker,
        promptManager: promptManager,
    }
}
```

### 2. Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check (`{"status":"ok"}`) |
| GET | `/api/v1/status` | Full status (reuses StatusChecker) |
| GET | `/api/v1/queue` | List queued prompts |
| GET | `/api/v1/completed` | List completed prompts (with `?limit=N`) |

### 3. HTTP handler pattern

Each endpoint gets its own handler file in `pkg/server/`:
- `health_handler.go`
- `status_handler.go`
- `queue_handler.go`
- `completed_handler.go`

Use `net/http` stdlib (no framework). JSON responses with proper Content-Type.

### 4. Start server alongside daemon

In `runner.Run()`, start HTTP server in a goroutine:
- Default port: `8080` (configurable via flag or env `DARK_FACTORY_PORT`)
- Graceful shutdown on context cancellation
- Server runs parallel to the watcher loop

### 5. Update factory

Add `CreateServer(addr string, ...) Server` to factory.

### 6. Update main.go

Add `--port` flag (default 8080). Pass to factory. Start server when running daemon mode.

### 7. Tests

- Health endpoint returns 200
- Status endpoint returns valid JSON matching StatusChecker output
- Queue endpoint lists queued prompts
- Completed endpoint respects limit parameter
- Server shuts down gracefully on context cancel
- Error responses (500) when StatusChecker fails

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow `~/.claude-yolo/docs/go-patterns.md` (interface + struct + New*)
- Follow `~/.claude-yolo/docs/go-factory-pattern.md` (Create* in factory, zero logic)
- Follow `~/.claude-yolo/docs/go-composition.md` (inject deps)
- Coverage ≥80% for new packages
- Use stdlib `net/http` only — no external HTTP frameworks
