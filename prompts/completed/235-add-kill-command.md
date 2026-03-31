---
status: completed
summary: Added `dark-factory kill` command that reads the PID from the lock file, sends SIGTERM with a 5-second SIGKILL fallback, and wires it into main.go and the factory.
container: dark-factory-235-add-kill-command
dark-factory-version: v0.80.0-1-g2b37ac1
created: "2026-03-31T21:25:00Z"
queued: "2026-03-31T19:26:35Z"
started: "2026-03-31T20:32:30Z"
completed: "2026-03-31T20:42:22Z"
---

<summary>
- Users can stop a running daemon with `dark-factory kill`
- Reads PID from lock file, sends SIGTERM, waits for exit
- Prints confirmation or error if no daemon is running
- Help text updated with the new command
- SIGKILL fallback if process doesn't exit within 5 seconds
</summary>

<objective>
Add `dark-factory kill` command that stops the running daemon by reading the PID from the lock file and sending SIGTERM.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-factory-pattern.md` — factory wiring.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega test conventions.

Key files to read before making changes:
- `main.go` — command routing, `printHelp()`
- `pkg/lock/locker.go` — `FilePath()` function and lock file format
- `pkg/factory/factory.go` — factory wiring for commands
</context>

<requirements>

## 1. Add kill command

Create `pkg/cmd/kill.go`:
- Read PID from lock file via `lock.FilePath(".")`
- If lock file does not exist or is empty, print "no daemon running" and return (not an error)
- Check if process is alive (`os.FindProcess` + `Signal(0)`)
- If not alive, print "no daemon running (stale lock file)", remove lock file, return
- Send `syscall.SIGTERM` to the process
- Wait up to 5 seconds for the process to exit (poll with `Signal(0)`)
- If exited, print "daemon stopped (pid NNN)"
- If still running after 5s, send `syscall.SIGKILL` and print "daemon killed (pid NNN)"
- Constructor returns interface, struct unexported (follow existing command patterns)

## 2. Wire command

In `main.go`:
- Add `case "kill"` at the top-level command routing (same level as "daemon", "status", "run")
- Route to `factory.CreateKillCommand(cfg).Run(ctx, args)`
- Update `printHelp()` — add `kill` line near `daemon`
- Update `ParseArgs` — add `"kill"` to the top-level command whitelist (find the `case "run", "daemon", "status", "list", "config":` line)

In `pkg/factory/factory.go`:
- Add `CreateKillCommand(cfg) → cmd.NewKillCommand(...)`

## 3. Tests

- `pkg/cmd/kill_test.go` — test: no lock file → "no daemon running", stale PID → removes lock, valid PID mock
- Follow existing test patterns

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Use `github.com/bborbe/errors` for error wrapping
- New code must have >= 80% test coverage
- Existing tests must still pass
- Follow existing command patterns (constructor returns interface, struct unexported)
- Use SIGTERM first, SIGKILL as fallback — never SIGKILL first
</constraints>

<verification>
```bash
make precommit
```
</verification>
