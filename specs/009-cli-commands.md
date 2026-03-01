---
status: completed
---

# CLI Commands: Status and Queue

## Problem

The only way to interact with dark-factory is to start the daemon. No way to check status or queue files without the daemon running or without reading files manually.

## Goal

Two subcommands that work independently of the daemon: `dark-factory status` to check current state, and `dark-factory queue` to move files from inbox to queue.

## Non-goals

- No `dark-factory stop` command (use SIGINT/kill)
- No `dark-factory logs` command (read log files directly)
- No `dark-factory init` command (manual config creation)

## Desired Behavior

### `dark-factory status`

1. Shows: daemon status, current executing prompt, queue count, completed count, ideas count, last log file with size
2. Default output: human-readable formatted text
3. `--json` flag: output as JSON (for scripting)
4. Works whether daemon is running or not (reads files directly)

### `dark-factory queue [filename]`

1. With filename argument: moves that specific file from inbox to queue
2. Without argument: moves all `.md` files from inbox to queue
3. Files are normalized during move (get NNN- prefix)
4. Status is set to `queued` in frontmatter
5. Reports what was queued: "queued: draft.md -> 005-draft.md"

### Command routing

1. No argument or `run`: start daemon (existing behavior)
2. `status`: run status command
3. `queue`: run queue command
4. Unknown command: error with usage message

## Constraints

- Commands load config from `.dark-factory.yaml` (same as daemon)
- Commands do NOT require daemon to be running
- Commands do NOT acquire the instance lock

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Config file missing | Use defaults (same as daemon) | None needed |
| File not found in inbox | Error: "file not found: name.md" | Check filename |
| Queue directory doesn't exist | Error: directory not found | Start daemon first (creates dirs) or mkdir |

## Acceptance Criteria

- [ ] `dark-factory status` shows formatted status output
- [ ] `dark-factory status --json` outputs valid JSON
- [ ] `dark-factory queue draft.md` moves file to queue with NNN prefix
- [ ] `dark-factory queue` moves all inbox .md files
- [ ] Unknown commands produce usage error
- [ ] Commands work without running daemon

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Manually `mv` files and `grep` for status. Works but error-prone (forgetting to set frontmatter status, wrong numbering).
