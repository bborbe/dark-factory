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
   - 3x Sosumi = prompt failed — check log, fix, retry
   - Basso = stuck >15min — may need intervention
   - Glass = all complete

## Success Criteria
- Script exits 0 on queue completion (Glass sound played)
- Script exits non-zero on missing prerequisites
- Failed prompts detected and alerted (Sosumi sound, script breaks)
- Stuck prompts (>15min) alerted (Basso sound, continues watching)
