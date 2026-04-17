---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-04-17T13:37:10Z"
generating: "2026-04-17T13:37:11Z"
prompted: "2026-04-17T13:49:21Z"
branch: dark-factory/committing-status-git-retry
---

## Summary

- Add a `committing` prompt status between `executing` and `completed` to represent "container succeeded, awaiting git persistence"
- Retry git commit operations when `.git/index.lock` blocks the commit (exponential backoff, 3 retries)
- Prevent daemon crash when `CommitCompletedFile()` fails — leave the prompt as `committing` and retry on the next daemon cycle
- On daemon startup, recover any `committing` prompts left behind by a previous crash
- Display `committing` status distinctly in `dark-factory status` output

## Problem

When the git commit after a successful container run fails (most commonly because `.git/index.lock` is held by a concurrent process), the daemon crashes. By that point the prompt file has already been moved to `completed/` with `status: completed`, so on restart the daemon considers the work done. The actual code changes (vendor updates, source modifications) are never committed. Dirty files accumulate, eventually exceeding the 500-file threshold and blocking all subsequent prompts. This cascading failure has been observed repeatedly in repos with large vendor updates (~2000 files per service) across mdm, commerce, and google projects, each time requiring manual intervention to recover.

## Goal

After this work is done, a git commit failure during post-container processing no longer crashes the daemon, never leaves the system in an inconsistent state, and self-heals on retry without human intervention. The prompt lifecycle has a clear, observable intermediate state (`committing`) that makes the git-persistence phase visible and recoverable.

## Non-goals

- Does not change branch, clone, or worktree workflow commit flows (direct workflow only for now)
- Does not change the dirty-file threshold or its blocking behavior
- Does not add git-level locking or coordination between dark-factory processes (the retry handles contention, does not prevent it)
- Does not retry container execution — only the git commit step

## Desired Behavior

1. When a container exits successfully (code 0), the prompt transitions to `committing` and stays in the `in-progress/` directory. The prompt file is NOT moved to `completed/` until the git commit succeeds.

2. The daemon attempts to git-add and git-commit all work files. If `.git/index.lock` exists or the git operation fails, it retries up to 3 times with exponential backoff (2s, 4s, 8s), bounded by a 30-second overall timeout.

3. On successful git commit: the prompt moves to `completed/`, status becomes `completed`, and the prompt-move itself is committed. This is the same end state as today, just reached through a safer path.

4. On git commit failure after all retries: the prompt remains as `committing` in `in-progress/`. The daemon does NOT crash. On the next daemon cycle (5s ticker), it re-attempts the commit for any `committing` prompts.

5. On daemon startup, any prompts found in `in-progress/` with `status: committing` are treated as needing git commit recovery. The daemon attempts to commit dirty workspace files and, on success, completes the prompt normally.

6. In direct workflow, each `committing` prompt commits ALL dirty files in the workspace (there is no per-prompt file isolation). When multiple prompts are `committing`, they are processed sequentially: the first commit captures all current dirty files, subsequent prompts find a clean workspace and move directly to `completed/` (see Failure Mode row "Workspace has no dirty files").

7. `dark-factory status` displays `committing` prompts distinctly — they are not shown as idle, executing, or completed.

8. Retry attempts are logged: WARN level for "retrying git commit, index.lock held", INFO level for "git commit succeeded after N retries", ERROR level for "git commit failed after all retries, will retry next cycle".

## Constraints

- Backward compatible: existing prompts with `status: completed` remain valid and unchanged
- `committing` is an internal status — users never set it manually via CLI or frontmatter
- The prompt status lifecycle documented in `docs/prompt-writing.md` must be updated to include `committing`
- The architecture flow documented in `docs/architecture-flow.md` must reflect the new status in its lifecycle diagram
- `make precommit` must pass after all changes
- No new external dependencies

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `.git/index.lock` held during commit | Retry 3 times with backoff, then leave as `committing` | Next daemon cycle retries; lock holder eventually releases |
| Daemon crashes while prompt is `committing` | Prompt stays in `in-progress/` with `committing` status | Startup recovery detects and retries the commit |
| Daemon crashes between setting `committing` and saving to disk | Prompt still has `executing` status on disk | Normal retry-from-executing logic handles this (no change needed) |
| Git commit succeeds but prompt-move commit fails | Work is committed but prompt is still `committing` in `in-progress/` | Next cycle detects no dirty work files, moves prompt to `completed/` |
| Workspace has no dirty files when `committing` prompt is processed | Treat as "commit already happened" | Move prompt to `completed/` directly |
| Multiple `committing` prompts exist simultaneously | Process sequentially; first commit captures all dirty files, subsequent prompts find clean workspace and move to `completed/` directly | No file isolation needed — "no dirty files" path handles it |
| 30-second timeout exceeded during a single git operation | Context cancellation stops the hung operation | Prompt stays `committing`, retried next cycle |

## Acceptance Criteria

- [ ] `committing` is a valid prompt status recognized by the status parser
- [ ] A prompt transitions from `executing` to `committing` (not directly to `completed`) when the container succeeds
- [ ] The prompt file remains in `in-progress/` during the `committing` phase
- [ ] Git commit retries up to 3 times on failure with exponential backoff
- [ ] After all retries fail, the daemon continues running (no crash) and the prompt stays `committing`
- [ ] On next daemon cycle, `committing` prompts are re-attempted
- [ ] On daemon startup, `committing` prompts in `in-progress/` trigger commit recovery
- [ ] `dark-factory status` shows `committing` prompts with a distinct label
- [ ] `docs/prompt-writing.md` lifecycle table includes `committing`
- [ ] `docs/architecture-flow.md` status diagram includes `committing`

## Verification

```
make precommit
```

## Do-Nothing Option

Without this change, every git commit failure during post-container processing crashes the daemon, leaves uncommitted work in the workspace, and eventually triggers the dirty-file threshold that blocks all prompts. Recovery requires manual SSH/terminal intervention: remove `.git/index.lock`, manually `git add` + `git commit` the orphaned changes, and restart the daemon. This has been a recurring operational burden on repos with large vendor directories and will continue to happen on every concurrent git access during the commit window.
