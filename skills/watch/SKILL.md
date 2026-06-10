---
name: watch
description: Watch dark-factory progress with sound alerts for completion, failure, and stuck prompts. Use when user wants to monitor daemon execution.
---

## Prerequisites
- Daemon must be running (`/dark-factory:daemon`)

## Steps

1. Run via Bash tool with `run_in_background: true` and `timeout: 600000`:
```bash
bash scripts/watch.sh [project-dir]
```
   - If project dir given, uses it
   - If cwd has `.dark-factory.yaml`, uses cwd
   - Otherwise, finds running daemon via `.dark-factory.lock` in `~/Documents/workspaces/`

2. Show sound legend:
   - 3x Sosumi = prompt failed, or container silent + stuck (>=15min elapsed, 0 log lines in last 3min) — check log, fix, retry
   - Basso = likely stuck — fired by either (a) >=15min on "executing since" (exec mode), or (b) >=10min elapsed with <5 container log lines in last 3min (gen + exec modes)
   - Glass = all complete

## Success Criteria
- Script exits 0 on queue completion (Glass sound played)
- Script exits non-zero on missing prerequisites
- Failed prompts detected and alerted (Sosumi sound, script breaks)
- Stuck prompts alerted via elapsed-time fallback (>15min "executing since") or container-log liveness probe (>=10min elapsed, <5 log lines/3min) — Basso, continues watching
- Silent + stuck containers (>=15min, 0 log lines/3min) treated as failure (3x Sosumi, script breaks)
- Liveness probe skips cleanly when docker unavailable, no active container, or `docker logs` fails mid-cycle

## Manual Test Matrix (liveness probe)

No shell test harness in this repo — verify by hand when touching the probe logic in `scripts/watch.sh`:

| RunningFor input | Expected ELAPSED_MIN |
|---|---|
| `59 seconds` / `About a minute` | 0 (no probe) |
| `14 minutes` | 14 |
| `About an hour` | 60 |
| `2 hours` | 120 |
| `3 days` / `2 weeks` | 1440 |

| ELAPSED_MIN | LOG_LINES | Expected behavior |
|---|---|---|
| <10 | any | no probe |
| >=10 | 5+ | silent (healthy) |
| >=10 | 1-4 | Basso (max one per poll), keep watching |
| >=15 | 0 | 3x Sosumi + break |
| >=10 | docker logs fails | WARN + skip probe this cycle |
