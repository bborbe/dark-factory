---
status: draft
kind: bug
---

# `"nothing to do, waiting for changes"` log is emitted on every idle tick (every 5s by default)

## Summary

Dark-factory's daemon emits `"nothing to do, waiting for changes"` on every idle tick of the queue scanner — by default every `queueInterval: 5s`. For a daemon that sits idle most of the time, this produces ~12 lines/min, ~720/hour, ~17k/day in `.dark-factory.log`. The same idle window also emits `"waiting for changes"` from `pkg/processor/processor.go:191`, which compounds the noise. The two log lines say the same thing, fire at the same instant, and add no information beyond "the daemon is alive."

## Reproduction

dark-factory version: `v0.150.2` (built from master at HEAD).

1. Project with no queued prompts:
   ```bash
   cd ~/Documents/workspaces/dark-factory
   dark-factory daemon
   ```
2. Watch `.dark-factory.log`:
   ```
   time=2026-05-06T09:30:45.142+02:00 level=INFO msg="nothing to do, waiting for changes"
   time=2026-05-06T09:30:45.142+02:00 level=INFO msg="nothing to do, waiting for changes"
   time=2026-05-06T09:30:45.142+02:00 level=INFO msg="nothing to do, waiting for changes"
   time=2026-05-06T09:30:45.461+02:00 level=INFO msg="nothing to do, waiting for changes"
   time=2026-05-06T09:30:50.142+02:00 level=INFO msg="nothing to do, waiting for changes"
   time=2026-05-06T09:30:55.143+02:00 level=INFO msg="nothing to do, waiting for changes"
   time=2026-05-06T09:31:00.144+02:00 level=INFO msg="nothing to do, waiting for changes"
   ...
   ```
3. The burst of 3-4 lines at the same millisecond happens because the processor's internal idle log AND the factory's `onIdle` callback both fire on the same tick.
4. After 1 minute idle: 12 lines. After 1 hour idle: ~720 lines. The signal-to-noise ratio of `.dark-factory.log` collapses to ~0 once the daemon idles.

## Expected vs Actual

**Expected:**
- The first time the daemon enters the idle state, log `"nothing to do, waiting for changes"` once.
- While idle, suppress the line until either (a) the daemon transitions to busy and back to idle, or (b) a heartbeat interval elapses (e.g. 1-5 minutes) so the operator knows the daemon is still alive.
- No duplicate emissions at the same millisecond — the processor and the factory should not both log the same message on the same tick.

**Actual:** the message fires every `queueInterval` (5s default), often duplicated 2-4× per tick, with no deduplication.

## Why this is a bug

Per `docs/architecture-flow.md` and Go logging conventions: log lines should mark events. "I am idle" is a state, not an event. Emitting it on every poll turns the log into a stream of identical heartbeats that drown out real events (PR creation, prompt execution, errors). When grepping `.dark-factory.log` for an issue, the operator wades through hundreds of "nothing to do" lines per actual event line.

This also burns disk space and IO on a long-running daemon: a project that idles overnight (~14h) accumulates ~10k lines of pure noise.

## Code pointers

- `pkg/factory/factory.go:418-422` — the `onIdle` callback passed to `CreateProcessor` calls `slog.Info("nothing to do, waiting for changes")` unconditionally on every idle tick.
- `pkg/processor/processor.go:191` — internal `slog.Info("waiting for changes")` fires on the same tick, doubling the noise. Possibly also fires from another goroutine (watcher / spec watcher) explaining the 3-4× burst.
- `pkg/queuescanner/scanner.go` — owns the tick cadence (`queueInterval`).
- `pkg/config/config.go` — `QueueInterval` (default `5s`) controls the tick rate but should NOT be repurposed as the log-throttle rate; log throttling is a separate concern.

## Workaround

Filter the log line at the operator's tail step: `tail -f .dark-factory.log | grep -v "nothing to do, waiting for changes"`. Loses startup signal and heartbeat. Not committed; not discoverable.

## Goal

The idle log line is emitted at most once per state transition (busy → idle), plus an optional heartbeat at a long interval (default `1m`, configurable). Duplicate emissions on the same tick are eliminated. The daemon's log becomes readable: each line is an event, not a heartbeat.

## Constraints

- Do NOT change `queueInterval` semantics — the scanner still polls at the same cadence; only the log emission is throttled.
- Do NOT silence the idle log entirely — operators need to confirm the daemon is alive after a long idle window.
- Do NOT emit the idle line from multiple sites for the same logical event — pick ONE source of truth (the `onIdle` callback in factory.go is the natural choice; the processor's internal `"waiting for changes"` line should be removed or merged into the same callback).
- Heartbeat interval should be configurable via `idleLogInterval` (default `1m`) so noisy and quiet projects can tune.
- Existing one-shot (`run`) mode behavior MUST not regress — `onIdle` in one-shot mode triggers context cancellation; that path stays unchanged. Only the daemon-mode log emission is throttled.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Daemon enters idle state for the first time | Emit `"nothing to do, waiting for changes"` once | None — single log entry |
| Daemon stays idle for 30s, no state change | No additional log entries | None |
| Daemon stays idle for `idleLogInterval` (default 1m) | Emit a heartbeat: `"still idle, waiting for changes"` (or similar — distinct from the first-time line so grep can filter) | None |
| Daemon transitions busy → idle → busy → idle | Each idle re-entry emits the first-time line once | None |
| `idleLogInterval: 0` set in config | Heartbeat disabled; only the first-entry line emits per idle window | Operator knows by the absence of recent events |
| One-shot `run` mode finishes, `onIdle` fires to cancel context | Context cancellation works as before; no log emission required | Existing exit path |

## Acceptance Criteria

- [ ] After daemon startup with no queued prompts, `.dark-factory.log` contains exactly ONE `"nothing to do, waiting for changes"` line within the first 30 seconds.
- [ ] After 1 minute of continuous idle, `.dark-factory.log` contains at most 2 lines matching `"waiting for changes"` (initial + 1 heartbeat).
- [ ] After 1 hour of continuous idle with default `idleLogInterval: 1m`, `.dark-factory.log` contains ≤ 61 such lines (initial + 60 heartbeats).
- [ ] No duplicate emission at the same millisecond — `pkg/processor/processor.go:191` no longer emits `"waiting for changes"`; only the factory's `onIdle` callback does.
- [ ] After a busy→idle transition (e.g. a prompt completes), the next idle tick emits the first-entry line once.
- [ ] Setting `idleLogInterval: 0` in `.dark-factory.yaml` disables the heartbeat — only the first-entry line per idle window appears.
- [ ] One-shot `run` mode still exits when `onIdle` fires; no log emission needed in that path.
- [ ] CHANGELOG.md `## Unreleased` entry added.

## Verification

```bash
cd ~/Documents/workspaces/dark-factory
make precommit
```

**Runtime replay:**

```bash
# Empty queue, fresh log
rm -f .dark-factory.log
dark-factory daemon &
DAEMON_PID=$!
sleep 90  # 1.5 minutes idle

# Count idle log lines
grep -c "waiting for changes" .dark-factory.log
# Expected: ≤ 2 (initial + 1 heartbeat after 1 min)

# Count total log lines
wc -l .dark-factory.log
# Expected: ~5-10 lines (startup config, lock, watcher started, processor started, 1-2 idle lines)

kill $DAEMON_PID
```

**Negative-control:** with `idleLogInterval: 0` in config:

```bash
sed -i.bak '$a\
idleLogInterval: 0' .dark-factory.yaml
rm -f .dark-factory.log
dark-factory daemon &
DAEMON_PID=$!
sleep 180  # 3 minutes idle
grep -c "waiting for changes" .dark-factory.log
# Expected: 1 (only the initial entry; no heartbeats)
kill $DAEMON_PID
mv .dark-factory.yaml.bak .dark-factory.yaml
```

**Acceptable evidence for `verifying → completed`:**

| Evidence | Acceptable? |
|----------|-------------|
| `.dark-factory.log` has ≤ 2 `"waiting for changes"` lines after 90s idle | Yes |
| `.dark-factory.log` has 1 `"waiting for changes"` line with `idleLogInterval: 0` after 3min idle | Yes |
| Code inspection confirms processor.go:191 no longer logs | Yes (necessary, not sufficient — must be paired with runtime replay) |
| "All tests pass" without runtime replay | No |

## Non-goals

- Adding structured-event log dedup framework — narrow fix only, no general solution.
- Changing other log lines (`"watching for queued prompts"`, container output, etc.).
- Adding a log-rotation feature for `.dark-factory.log`.

## See also

- `pkg/factory/factory.go:418-422` — the `onIdle` callback to wrap in a throttle.
- `pkg/processor/processor.go:191` — the duplicate emit site to remove.
- `pkg/processor/processor_verification_test.go:543-544` — existing test asserting `Equal(1)` for `"waiting for changes"` count after one-shot run; verify this test still applies (one-shot path doesn't emit the idle line, only daemon does).
- `pkg/config/config.go` — `QueueInterval` (existing) and the new `IdleLogInterval` field to add.
