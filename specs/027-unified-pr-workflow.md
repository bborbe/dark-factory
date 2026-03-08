---
status: draft
---

## Summary

- Replaces three workflow modes (`direct`, `pr`, `worktree`) with two: `direct` and `pr`
- New `pr` = always worktree + feature branch + PR (merges old `pr` and `worktree` into one)
- Old `pr` (in-place branching) and `worktree` config values become errors
- `direct` unchanged
- Simplifies code, config, and mental model

## Problem

Dark-factory has three workflow modes but only `direct` works reliably. `pr` (in-place branching) and `worktree` (isolated execution) are broken and share 80% of their logic. Users must choose between two broken options that differ only in isolation strategy. The in-place `pr` mode has no real advantage over worktree — it leaves master dirty during execution and has weaker error recovery.

## Goal

After this work, `workflow: pr` always executes prompts in an isolated git worktree on a feature branch, leaving master clean throughout. Each prompt produces exactly one PR. No worktree remains on disk after completion (success or failure). The old in-place branching mode no longer exists — there are exactly two workflow modes: `direct` and `pr`.

## Non-goals

- No changes to `workflow: direct`
- No changes to `autoMerge`, `autoRelease`, `autoReview` behavior (they work on top of PR workflow)
- No parallel prompt execution
- No new git operations beyond what worktree workflow already does

## Assumptions

- Nobody depends on the old in-place `pr` behavior (it's broken anyway)
- Worktree path convention `../projectName-baseName` is safe (no collisions)
- `gh` CLI is available for PR creation (existing requirement)

## Desired Behavior

1. `workflow: pr` creates a git worktree at `../projectName-baseName` with branch `dark-factory/baseName`.
2. Prompt executes inside the worktree directory (Docker container mounts worktree path).
3. After successful execution: commit changes, push branch, create PR via `gh pr create`.
4. Worktree is removed after PR creation (success path).
5. Worktree is removed on execution failure (error path) — branch stays for debugging.
6. `workflow: direct` behavior unchanged.
7. Config values `worktree` (old) produce a clear error on startup: "workflow 'worktree' removed — use 'pr' instead".
8. `autoMerge: true` with `workflow: pr`: after PR creation, poll and merge, then pull default branch.

## Constraints

- `workflow: direct` must not change
- Existing `autoMerge`, `autoRelease`, `autoReview` config fields stay — they layer on top of `pr`
- Config validation rules for `autoMerge`/`autoRelease`/`autoReview` stay (require `workflow: pr`)
- Branch naming convention `dark-factory/{baseName}` stays
- `make precommit` must pass
- No force-push after PR creation — remove the PR URL amendment flow that force-pushes

## Security

No new external input surfaces. Worktree paths are derived from project name and prompt base name (both controlled by project owner). Both are validated to contain only alphanumeric characters, hyphens, and underscores — no path traversal risk.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `git worktree add` fails (path exists) | Prompt marked `failed`, error logged | Clean up stale worktree manually, requeue |
| Docker execution fails in worktree | Worktree removed, prompt marked `failed` | Fix code, requeue prompt |
| `gh pr create` fails (no gh token) | Worktree removed, prompt marked `failed` | Set `GH_TOKEN`, requeue |
| `autoMerge` poll times out | PR stays open, prompt marked `failed` | Merge manually or requeue |
| Dark-factory crashes mid-execution | Stale worktree remains on disk | `git worktree remove` manually, requeue |
| Config has `workflow: worktree` | Startup error with migration message | Change config to `workflow: pr` |

## Acceptance Criteria

- [ ] `workflow: pr` creates worktree, executes prompt, commits, pushes, creates PR
- [ ] Worktree cleaned up on success (after PR creation)
- [ ] Worktree cleaned up on failure (execution error)
- [ ] `workflow: direct` unchanged
- [ ] `workflow: worktree` config value produces clear error with migration hint
- [ ] No force-push after PR creation (PR URL amendment removed)
- [ ] `autoMerge` works with new `pr` workflow (poll, merge, pull default branch)
- [ ] No execution path for in-place (non-worktree) PR creation exists
- [ ] Single unified PR implementation (no separate worktree vs in-place code paths)
- [ ] Existing tests updated, new tests for error/cleanup paths
- [ ] `make precommit` passes

## Verification

```
make precommit
```

## Do-Nothing Option

Keep three broken workflow modes. All projects stay on `workflow: direct`. No PR review gate, no isolation. Human must manually create branches, run prompts, commit, push, and create PRs — defeating the purpose of Level 4 automation.
