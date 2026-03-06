---
status: completed
---

# Crash Recovery: Reset Stuck and Failed Prompts

## Problem

If dark-factory crashes mid-execution, a prompt is left with `status: executing` — it's stuck. Failed prompts (`status: failed`) require manual editing to retry. Both cases need human intervention after a restart.

## Goal

On startup, automatically reset stuck and failed prompts so the factory can retry without manual intervention.

## Non-goals

- No retry limit (infinite retries on restart)
- No notification of reset prompts (log only)
- No distinction between crash-stuck and legitimately executing (conservative: reset all)

## Desired Behavior

1. On startup, before entering the main processing loop:
   - Scan queueDir for all `.md` files
   - Any file with `status: executing` -> set to `status: queued` (crash recovery)
   - Any file with `status: failed` -> set to `status: queued` (retry)
2. Log each reset: "reset [executing|failed] prompt NNN-slug.md to queued"
3. Normal processing then begins

## Constraints

- Only scans queueDir (not inbox, not completed)
- Resets happen before watcher or processor starts
- Timestamps are preserved (only status field changes)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| No stuck/failed prompts | Startup continues normally | None needed |
| File read error during scan | Log error, skip file | Manual reset |
| Reset causes re-execution of prompt that already partially committed | Acceptable — YOLO is idempotent enough, and git will show the diff | Review commit |

## Acceptance Criteria

- [ ] `status: executing` prompts reset to `queued` on startup
- [ ] `status: failed` prompts reset to `queued` on startup
- [ ] Reset logged for each affected file
- [ ] Already-queued and already-completed files are untouched
- [ ] Processing begins normally after reset

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Require manual `status: queued` edits after every crash or failure. Breaks unattended operation — the core value proposition of dark-factory.
