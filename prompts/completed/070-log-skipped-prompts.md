---
spec: ["015"]
status: completed
summary: Added logging for skipped prompts at appropriate log levels
container: dark-factory-070-log-skipped-prompts
dark-factory-version: v0.16.0
created: "2026-03-05T13:09:30Z"
queued: "2026-03-05T13:09:30Z"
started: "2026-03-05T13:09:30Z"
completed: "2026-03-05T13:15:40Z"
---
<objective>
Log why prompts in the queue are skipped instead of silently ignoring them.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read ALL markdown files in ~/Documents/workspaces/coding-guidelines/ for Go patterns.
Read pkg/processor/processor.go — `processExistingQueued` method, lines around `ValidateForExecution` and `AllPreviousCompleted`.
Read pkg/prompt/prompt.go — `ListQueued` function, the skip logic for executing/completed/failed status.
</context>

<requirements>
1. In `processExistingQueued` (`pkg/processor/processor.go`):
   - Change `slog.Debug("skipping prompt"...)` at the `ValidateForExecution` error to `slog.Warn`
   - Change `slog.Debug("skipping prompt"...)` at the `AllPreviousCompleted` check to `slog.Info` with message "prompt blocked"
   - These are the two places where dark-factory finds a file but cannot execute it

2. In `ListQueued` (`pkg/prompt/prompt.go`):
   - When a file is skipped due to executing/completed/failed status, add `slog.Debug("skipping prompt", "file", entry.Name(), "status", fm.Status)`
   - Refactor the condition to early-continue (skip status check first, then append) for clarity
   - Ensure `prealloc` linter is satisfied (pre-allocate slice if needed)

3. In `ListQueued` (`pkg/prompt/prompt.go`):
   - When a file has a read error, log it: `slog.Warn("skipping prompt", "file", entry.Name(), "error", err)`
</requirements>

<constraints>
- Only modify pkg/processor/processor.go and pkg/prompt/prompt.go
- Do not change any public API signatures
- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>

<success_criteria>
- Validation failures logged at Warn level (visible by default)
- Ordering blocks logged at Info level (visible by default)
- ListQueued skip reasons logged at Debug level
- make precommit passes
</success_criteria>
