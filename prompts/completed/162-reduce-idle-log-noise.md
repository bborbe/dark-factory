---
status: completed
summary: 'Silenced idle daemon log noise: removed INFO log from processExistingQueued on empty queue, added ''waiting for changes'' INFO log after daemon startup scan, added ''no queued prompts'' INFO log in ProcessQueue one-shot mode, and added tests verifying the new behavior.'
container: dark-factory-162-reduce-idle-log-noise
dark-factory-version: v0.36.0-dirty
created: "2026-03-10T13:46:30Z"
queued: "2026-03-10T13:46:30Z"
started: "2026-03-10T13:46:33Z"
completed: "2026-03-10T13:54:42Z"
---
<summary>
- The daemon no longer logs "no queued prompts" every 5 seconds when idle
- One-shot mode still prints a single message when no prompts are found
- Daemon mode prints "waiting for changes" once after processing the initial queue
- Periodic ticker scans only log with --verbose (DEBUG level)
- Reduces log noise significantly for long-running daemon sessions
</summary>

<objective>
The daemon is noisy at INFO level when idle — it repeats "no queued prompts, exiting" every 5 seconds. Silence empty-queue scans so the daemon only logs at INFO when something happens or when transitioning to idle.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go`:
- `Process()` (line ~116): the main loop — calls `processExistingQueued` on startup (line ~130), then enters a `for/select` loop with a watcher channel (line ~144) and a 5-second ticker (line ~152). Both paths call `processExistingQueued`.
- `processExistingQueued()` (line ~185): scans for queued prompts. Has a `first` flag (line ~192) that gates the INFO log (line ~209). But `first` is local to each call, so it's `true` every time the ticker triggers.
- `ProcessQueue()` (line ~161): one-shot mode — calls `processExistingQueued` once and returns.
</context>

<requirements>
1. In `processExistingQueued()` in `pkg/processor/processor.go`: remove the `first` variable and the `if first` INFO log block (lines ~192, ~208-210, ~214). Change the remaining empty-queue path to only log at DEBUG level. The function should be silent at INFO when the queue is empty.

2. In `Process()` in `pkg/processor/processor.go`: after the initial `processExistingQueued` call (line ~130), add a single INFO log: `slog.Info("waiting for changes")`. This is the last INFO message the daemon prints when idle.

3. In `ProcessQueue()` in `pkg/processor/processor.go` (one-shot mode, line ~161): after calling `processExistingQueued`, if no error occurred, call `p.promptManager.ListQueued(ctx)` to check if the queue is empty. If empty, log `slog.Info("no queued prompts")` once. This avoids changing the `processExistingQueued` return signature.

4. Add/update tests to verify:
   - Daemon mode: no INFO log on empty ticker scans (only DEBUG)
   - One-shot mode: single INFO log when queue is empty
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do NOT change the ticker interval (5 seconds) — the periodic scan is a safety net
- The watcher channel path (line ~144) should still log at INFO when it finds and processes prompts
- Do NOT remove any existing DEBUG logs — they are useful with `--verbose`
</constraints>

<verification>
```
make precommit
```
Must pass with no errors.
</verification>

<success_criteria>
- Daemon idle output: "waiting for changes" once, then silence (no repeated "no queued prompts")
- One-shot empty output: "no queued prompts" once
- Processing prompts still logs at INFO as before
- All existing tests pass
</success_criteria>
