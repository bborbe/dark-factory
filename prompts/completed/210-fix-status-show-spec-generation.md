---
status: completed
summary: 'Implemented spec generation awareness in dark-factory status: isContainerRunning now runs docker ps, new populateGeneratingSpec detects gen-* containers, Status struct has GeneratingSpec/GeneratingContainer fields, formatter shows ''generating spec <name>'' instead of idle, and tests cover both new behaviors.'
container: dark-factory-210-fix-status-show-spec-generation
dark-factory-version: v0.63.0
created: "2026-03-21T18:48:11Z"
queued: "2026-03-21T18:48:11Z"
started: "2026-03-21T18:48:13Z"
completed: "2026-03-21T18:54:25Z"
---

<summary>
- The status command shows which spec is being generated instead of "idle"
- Displays the spec name and container when spec generation is active
- Detects running spec generation containers via Docker
- Implements a previously stubbed container-running check
</summary>

<objective>
Make `dark-factory status` aware of spec generation containers. Currently it shows "idle" while a `gen-*` container runs for minutes, confusing users. After this change, status shows `Current: generating spec 034-resume-executing-on-restart.md` with container info.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/status/status.go` — the `Status` struct (line ~22), `GetStatus` (line ~92), `populateExecutingPrompt` (line ~332), `isContainerRunning` (line ~437, has a TODO stub).
Read `pkg/status/formatter.go` — `Format` method, `formatCurrentPrompt` for the display pattern.
Read `pkg/status/status_test.go` and `pkg/status/formatter_test.go` — existing test patterns.
Read `pkg/executor/executor.go` — reference for how `exec.CommandContext` with `docker` commands is used (the `#nosec G204` pattern).
</context>

<requirements>
1. In `pkg/status/status.go`, implement `isContainerRunning` (replace the TODO stub):
   - Run `docker ps --filter name=<containerName> --format {{.Names}}` via `exec.CommandContext`
   - Return `true` if output contains the container name, `false` otherwise
   - Errors are logged at debug level and return `false` (Docker may not be running)

2. In `pkg/status/status.go`, add a new method `populateGeneratingSpec`:
   - Run `docker ps --filter name=dark-factory-gen- --format {{.Names}}` to find any running `gen-*` container
   - If found, extract the spec name from the container name (strip `dark-factory-gen-` prefix)
   - Set new Status fields: `GeneratingSpec` and `GeneratingContainer`
   - Call this method in `GetStatus` after `populateExecutingPrompt`, only when `CurrentPrompt` is empty (spec generation and prompt execution are mutually exclusive in practice)

3. In `pkg/status/status.go`, add fields to `Status` struct:
   - `GeneratingSpec string` — the spec being generated (e.g., `034-resume-executing-on-restart.md`)
   - `GeneratingContainer string` — the container name

4. In `pkg/status/formatter.go`, update `Format`:
   - When `CurrentPrompt` is empty but `GeneratingSpec` is set, display:
     ```
     Current:    generating spec 034-resume-executing-on-restart.md
     Container:  dark-factory-gen-034-resume-executing-on-restart (running)
     ```
   - This replaces the "idle" line in this case

5. Add tests:
   - `formatter_test.go`: test formatting when `GeneratingSpec` is set
   - `status_test.go`: test `isContainerRunning` returns false when container not found (no Docker mock needed — just verify graceful failure)
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap(ctx, ...)` — never `fmt.Errorf`
- `exec.CommandContext` calls need `#nosec G204` with explanation comment
- Docker command errors must never be fatal — log at debug, return gracefully
- Copyright header required on any new files
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
