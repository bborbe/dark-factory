---
status: completed
---

# HTTP Status API

## Problem

When dark-factory runs unattended, there's no way to check progress without reading files directly. No programmatic access for dashboards, scripts, or mobile checks.

## Goal

A local HTTP API that exposes factory status, queue state, inbox contents, and completed prompts. Disabled by default (port 0), opt-in via config.

## Non-goals

- No authentication (localhost only)
- No WebSocket/SSE for live updates
- No write operations beyond queue management
- No HTTPS (local traffic only)

## Desired Behavior

1. When `serverPort > 0`, start HTTP server on `127.0.0.1:{port}`
2. When `serverPort = 0`, no server starts (default)
3. Endpoints:
   - `GET /health` — returns 200 OK
   - `GET /api/v1/status` — daemon status, current prompt, queue/completed counts, last log file
   - `GET /api/v1/queue` — list of queued prompts with title and file size
   - `GET /api/v1/inbox` — list of markdown files in inbox
   - `GET /api/v1/completed?limit=N` — recent completed prompts (default 10, max 1000)
   - `POST /api/v1/queue/action` — queue a single file from inbox (body: `{"file": "name.md"}`)
   - `POST /api/v1/queue/action/all` — queue all inbox files
4. All responses are JSON
5. Server shuts down gracefully on SIGINT/SIGTERM

## Constraints

- Bind to `127.0.0.1` only (never `0.0.0.0`)
- Use `libhttp.NewServer` for sane defaults (timeouts, graceful shutdown, MaxHeaderBytes)
- Handlers use `libhttp.WithError` for centralized error handling
- Path traversal protection: `filepath.Base()` on all user-provided filenames
- `limit` query parameter capped at 1000

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Port already in use | Startup failure with "address already in use" | Change port or stop other process |
| Invalid JSON in POST body | 400 Bad Request with error message | Fix request |
| File not found in inbox | 404 with error message | Check filename |
| Path traversal attempt (`../../../etc/passwd`) | Sanitized to basename, file not found | None needed (attack blocked) |

## Security / Abuse Cases

- Path traversal in queue action: mitigated by `filepath.Base()`
- Request body size: limited by `http.MaxBytesReader`
- Query parameter abuse (limit=999999): capped at 1000
- Localhost binding prevents external access

## Acceptance Criteria

- [ ] Server starts on configured port, disabled when port=0
- [ ] `/health` returns 200
- [ ] `/api/v1/status` returns current daemon state as JSON
- [ ] `/api/v1/queue` lists queued prompts
- [ ] `/api/v1/inbox` lists inbox files
- [ ] `/api/v1/completed` returns recent completions with limit support
- [ ] `POST /api/v1/queue/action` moves file from inbox to queue
- [ ] Path traversal attempts are blocked
- [ ] Graceful shutdown on signal

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Check status by reading files directly (`ls prompts/queue/`, `grep status prompts/queue/*.md`). Works but not scriptable and no remote access.
