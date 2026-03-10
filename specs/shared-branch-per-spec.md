---
status: draft
---

## Summary

When multiple prompts belong to the same spec, they currently each get an independent branch cloned from the default branch. This means prompt 2 cannot see prompt 1's changes unless prompt 1 was already merged. This spec makes all prompts from one spec share a single branch so each prompt builds on the previous one's work.

## Problem

In PR workflow, `setupCloneWorkflowState()` generates `dark-factory/<prompt-basename>` as the branch name and always clones from the default branch. When a spec produces prompts 1→2→3, prompt 2 starts fresh from master — it misses prompt 1's code changes. The prompts fail or duplicate work because they can't see prior changes.

The `branch` frontmatter field already exists on prompts, but nothing sets it automatically for spec-linked prompts.

## Goal

After this work, all prompts linked to the same spec execute on a shared branch. Each prompt sees the cumulative code changes from all prior prompts in that spec. A single PR is created (or updated) for the spec's branch.

## Non-goals

- Changing direct workflow behavior (only PR workflow is affected)
- Parallel execution of spec prompts (they remain sequential)
- Merging spec PRs automatically mid-sequence (merge happens after last prompt or via autoMerge)
- Changing how standalone prompts (no spec field) work

## Desired Behavior

1. **Branch naming**: When a prompt has a `spec` field, derive branch name as `dark-factory/spec-<specID>` (e.g., `dark-factory/spec-028`). When multiple specs are listed, use the first one. When no spec field, keep current behavior (`dark-factory/<prompt-basename>` or explicit `branch` field).

2. **Cloner handles existing branches**: `Clone()` must support checking out an existing remote branch instead of always creating a new one with `checkout -b`. Logic: try `checkout -b <branch>` first; if it fails (branch exists), fetch and `checkout <branch>` + `pull` instead.

3. **Processor branch resolution**: In `setupCloneWorkflowState()`, resolve branch name with priority: explicit `branch` field > spec-derived branch > default `dark-factory/<basename>`.

4. **PR creation is idempotent**: When pushing to a branch that already has an open PR, skip PR creation. Use `gh pr list --head <branch> --state open` to check. If a PR exists, log its URL and continue. If no PR, create one.

5. **Auto-merge timing**: When autoMerge is enabled, only merge the PR after the last prompt of the spec completes. Detection: after completing a prompt, check if any remaining in-progress or queued prompts share the same spec. If none remain, proceed with merge. If more remain, skip merge for now.

6. **Clone path for shared branches**: Use `dark-factory/<spec-id>` as clone dir name (not prompt basename) so consecutive prompts from the same spec don't conflict.

## Constraints

- Standalone prompts (no spec field) must work exactly as before — zero behavior change
- Prompts with explicit `branch` frontmatter override spec-derived branch
- The shared branch is always based off the default branch initially (first prompt of spec)
- Subsequent prompts fetch and check out the existing branch (with prior prompt's commits)
- `make precommit` must pass
- All existing tests must pass

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| First prompt of spec fails | Branch exists with partial work; next prompt retries on same branch | Manual cleanup or requeue |
| Remote branch deleted between prompts | Clone falls back to creating new branch from default | Prior work is lost — same as current behavior |
| `gh pr list` fails (no gh auth) | Log warning, create PR anyway (may get duplicate PR error) | User resolves manually |
| Spec has prompts with mixed spec fields | Each group shares its own branch based on first spec ID | Works correctly — independent groups |

## Acceptance Criteria

- [ ] Prompts with `spec: ["028"]` use branch `dark-factory/spec-028`
- [ ] Second prompt in a spec sees first prompt's code changes
- [ ] Cloner can check out existing remote branches (not just create new)
- [ ] PR is created once per spec branch, not per prompt
- [ ] autoMerge waits until last spec prompt before merging
- [ ] Standalone prompts (no spec) behave identically to before
- [ ] Prompts with explicit `branch` field override spec-derived branch
- [ ] All existing tests pass
- [ ] `make precommit` passes

## Verification

```bash
# Unit tests pass
make precommit

# Integration check: create two prompts with same spec, verify same branch
# (manual or via test)
```

## Do-Nothing Option

Keep current behavior. Spec prompts that build on each other must be merged one-by-one before the next runs, or users must manually set the `branch` field on each prompt. This works but is slow and error-prone for multi-prompt specs.
