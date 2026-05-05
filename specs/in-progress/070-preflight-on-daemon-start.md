---
status: verifying
approved: "2026-05-05T17:59:10Z"
generating: "2026-05-05T17:59:10Z"
prompted: "2026-05-05T18:07:19Z"
verifying: "2026-05-05T20:27:07Z"
branch: dark-factory/preflight-on-daemon-start
---

## Summary

- Today the daemon waits idle until a prompt is queued, then runs preflight before executing.
- A broken baseline is therefore not surfaced until the operator approves a prompt — minutes or hours after the daemon started.
- This spec adds a preflight call at daemon startup, before the watcher loop begins.
- Cache rules are unchanged: if a successful preflight is already cached within `preflightInterval`, the startup call is a fast skip.
- Existing on-prompt-found preflight stays as-is; nothing else about preflight semantics changes.

## Problem

Preflight is the daemon's "is the baseline green?" check. Today it runs lazily — only when the daemon picks up its first queued prompt. If the operator starts the daemon and walks away, a broken baseline (failing tests, lint errors, dep mismatch) goes undetected until the next time work is queued. The operator may queue a prompt, leave for hours, and return to find that nothing executed because preflight failed at queue time.

Eager preflight at startup closes that gap: the daemon either reports green at start (so the next prompt executes immediately) or exits non-zero with a clear cause (so the operator fixes the tree before queueing anything).

## Goal

When the daemon starts, it runs the preflight check before entering the watcher loop. If preflight passes (or is cached), the daemon enters the watcher loop ready to execute. If preflight fails, the daemon exits non-zero with the existing terminal-failure semantics.

## Non-goals

- Changing `preflightInterval` cache duration.
- Changing the "preflight failure is terminal" policy (`docs/architecture-flow.md`).
- Removing the existing on-prompt-found preflight call — it stays as a safety net for cache expiry mid-run.
- Running preflight on file changes, git pulls, or other triggers — out of scope.
- Changing `run` (one-shot) mode behavior — it already runs preflight at start.

## Desired Behavior

1. When the daemon starts, it runs the configured `preflightCommand` before entering the watcher loop.
2. If a successful preflight result is cached within `preflightInterval`, the startup call reuses it (no command execution).
3. If preflight fails at startup, the daemon exits non-zero with the same log/notify behavior as a mid-run preflight failure.
4. If `preflightCommand` is empty (preflight disabled), the startup call is also a no-op.

## Constraints

- Do not change the cache key (HEAD SHA) or duration (`preflightInterval`).
- Do not weaken the terminal-failure policy — startup failure must still exit the daemon.
- Do not regress the on-prompt-found preflight call. It remains the safety net when the cache expires mid-run.
- Reuse the existing preflight runner — no parallel implementation.
- Apply only to `dark-factory daemon`. The `run` subcommand already runs preflight at start.
- The `--skip-preflight` CLI flag must skip the startup preflight too (same flag, same behavior).

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Baseline broken at daemon start | Daemon exits non-zero; log + notification fire (existing terminal-failure path) | Operator fixes tree, restarts daemon |
| Successful preflight cached within interval | Startup reuses cache; no command execution | N/A |
| `preflightCommand` empty | Startup preflight is a no-op | N/A |
| `--skip-preflight` flag set | Startup preflight skipped; daemon proceeds to watcher loop | N/A |
| Preflight command times out at startup | Same handling as mid-run timeout — terminal | Operator investigates the underlying command |

## Do-Nothing Option

Operators continue to discover broken baselines at queue time, often after long idle periods. The "preflight failure is terminal" policy already handles the failure correctly — the gap is purely operator-feedback latency. Cost: ongoing surprise when prompts queue and immediately fail; no broken behavior, just sustained drag.

## Acceptance Criteria

- [ ] On `dark-factory daemon` start, the daemon log shows a preflight attempt (or a "preflight cached" log line if the cache is fresh) BEFORE the watcher loop emits its first "waiting for changes" / "watching for prompts" log line.
- [ ] When preflight passes at start, the daemon enters the watcher loop normally and executes the next queued prompt without a second preflight (cache hit).
- [ ] When preflight fails at start, the daemon exits non-zero and logs the failure cause exactly like the existing mid-run failure path.
- [ ] When `--skip-preflight` is passed, the startup preflight is skipped (same as today's mid-run behavior).
- [ ] When `preflightCommand` is empty in config, the startup preflight is a no-op.
- [ ] Existing on-prompt-found preflight call remains and fires when the cache is expired mid-run.
- [ ] Scenario added under `scenarios/` (number assigned at scenario-write time): start the daemon with a known-broken baseline, assert daemon exits non-zero before any prompt is approved; start the daemon with a green baseline, assert the preflight log line appears at start and the next queued prompt executes without re-running preflight.
- [ ] CHANGELOG.md `## Unreleased` entry added.

## Verification

```bash
cd ~/Documents/workspaces/dark-factory
make precommit
# Plus the scenario above, run manually or via a helper script if added.
```

## See also

- `docs/architecture-flow.md` "Preflight Failure Policy" — terminal-failure invariant.
- `docs/configuration.md` "Preflight Baseline Check" — `preflightCommand`, `preflightInterval`, caching semantics.
- `docs/running.md` `daemon` vs `run` — `run` already runs preflight at start; this spec brings `daemon` in line.
