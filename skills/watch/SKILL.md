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
- Liveness probe skips cleanly when docker unavailable or no active container
