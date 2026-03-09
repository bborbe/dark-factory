---
status: created
created: "2026-03-09T23:35:00Z"
---

<summary>
- One-shot mode logs a clear message when no prompts are queued instead of exiting silently
- User sees feedback confirming the queue is empty
</summary>

<objective>
Running `dark-factory run` with an empty queue exits silently after "processor started (one-shot)", giving no feedback. The user cannot tell if it worked or something went wrong.

After this change, one-shot mode logs an info-level message when no queued prompts are found before exiting.
</objective>

<context>
Read CLAUDE.md for project conventions.

- `pkg/processor/processor.go` — `processExistingQueued()` at line 206-208: when `len(queued) == 0`, logs at `Debug` level with `"queue scan complete"`. This is the exit point that needs an `Info` log.
</context>

<requirements>
1. In `pkg/processor/processor.go` `processExistingQueued()`, when the first scan finds zero queued prompts (line 206-208), add an `slog.Info("no queued prompts, exiting")` before returning.
2. Keep the existing `slog.Debug("queue scan complete", "queuedCount", 0)` for debug-level tracing — add the Info log alongside it, not replacing it.
3. Only log this on the first iteration (empty queue at start). When the loop drains all prompts and finds zero remaining, the existing debug log is sufficient.
</requirements>

<constraints>
- Change only the empty-queue branch in `processExistingQueued()` (lines 206-208).
- Do NOT change the daemon/watcher behavior — this is one-shot only.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
- `make precommit` passes
- `dark-factory run` with empty queue shows: `level=INFO msg="no queued prompts, exiting"`
</verification>
