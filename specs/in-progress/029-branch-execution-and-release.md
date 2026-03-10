---
status: verifying
approved: "2026-03-10T19:37:41Z"
prompted: "2026-03-10T19:49:39Z"
verifying: "2026-03-10T21:37:50Z"
---

## Summary

- Prompts with a branch execute on that branch instead of the default branch
- Both in-place and clone modes support existing remote branches
- Release only happens on the default branch — feature branches accumulate in Unreleased
- After the last prompt on a branch completes, the branch is auto-merged and released
- PRs are created once per branch, not per prompt

## Problem

Currently, the processor either commits directly on the default branch (direct workflow) or always clones fresh from the default branch (PR workflow). When a spec produces prompts 1→2→3 that share a branch, prompt 2 starts fresh from master and misses prompt 1's code changes. The `branch` frontmatter field exists on prompts but the processor only uses it to name the clone branch — it still clones from the default branch every time, so the second prompt cannot build on the first.

## Goal

After this work, prompts with a branch field execute on that branch and see cumulative changes from prior prompts. The six combinations of `pr`, `worktree`, and `branch` all work correctly. Release happens only on the default branch after all branch work is complete.

## Non-goals

- Config migration from `workflow:` enum (handled by prerequisite spec `shared-branch-per-spec`)
- Frontmatter schema changes for `branch`/`issue` (handled by prerequisite spec)
- Parallel execution of prompts

## Desired Behavior

1. **In-place branch switching**: When `worktree` is false and a prompt has a `branch`, execution happens on that branch. A non-existent branch is created from the default branch automatically. An existing branch is checked out and updated. After execution completes, the repository is always left on the default branch.

2. **Clone mode supports existing branches**: When `worktree` is true, the clone uses the prompt's branch. If the branch already exists on the remote (from a prior prompt), it is checked out with the prior commits intact rather than starting fresh from the default branch. This allows each prompt to build on the previous one's work.

3. **No release on feature branches**: When executing on a non-default branch (branch field is set), the processor commits and adds changes to `## Unreleased` in the changelog but does not version-bump or tag. This applies regardless of `pr` setting.

4. **Auto-merge and release after last branch prompt**: When `pr` is false and a prompt with a branch completes, the processor checks if any remaining queued or in-progress prompts share the same branch. If none remain, it merges the branch to the default branch and releases (rename `## Unreleased` to version, tag, push). If more remain, it skips — the next prompt continues on the same branch.

5. **Idempotent PR creation**: When `pr` is true, a PR is created after pushing the branch. If an open PR already exists for that branch, no duplicate is created — the existing PR URL is logged instead. The PR body includes the `issue` field if set.

6. **Auto-merge timing with PRs**: When `pr` is true and `autoMerge` is enabled, the PR is only merged after the last prompt on that branch completes (same detection as item 4). Earlier prompts push to the branch and update the existing PR but do not trigger merge.

## Assumptions

- `git` is available on the host system
- `gh` CLI is available and authenticated (required for PR creation and duplicate detection)
- The working directory is clean before prompt execution begins (no uncommitted changes)
- Prompts within a spec are executed sequentially, never in parallel

## Constraints

- No branch + no worktree + pr false: identical to current direct workflow — zero behavior change
- Worktree true + pr true: identical to current PR workflow — zero behavior change
- The processor must always return to the default branch after execution (both in-place and clone modes)
- Feature branches never get version tags — only `## Unreleased` entries
- Auto-merge to default branch fails gracefully on conflicts (error, branch preserved)
- Requires prerequisite spec `shared-branch-per-spec` to be completed first
- All existing tests must pass
- `make precommit` must pass

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| First prompt on branch fails mid-execution | Branch exists with partial work; next prompt retries on same branch | Manual cleanup or requeue |
| Remote branch deleted between prompts | Falls back to creating new branch from default | User re-queues affected prompts after restoring or re-running |
| In-place branch switch on dirty working tree | Error logged, prompt fails | User resolves, requeue |
| Auto-merge to default has conflicts | Error logged, branch preserved, no release | User resolves conflicts manually |
| `gh pr list` fails (no auth) | Log warning, attempt PR creation anyway | May get duplicate PR; user resolves |
| No remaining prompts with same branch but merge fails | Release skipped, error logged | User merges and releases manually |

## Acceptance Criteria

- [ ] In-place mode switches to branch before execution and restores default branch after
- [ ] Clone mode checks out existing remote branches (not just create new)
- [ ] Second prompt on same branch sees first prompt's code changes
- [ ] No version bump or tag on feature branches
- [ ] Changelog entries go to `## Unreleased` on feature branches
- [ ] After last prompt on branch: auto-merge to default (when pr is false)
- [ ] After auto-merge: release with version + tag (when pr is false)
- [ ] PR created once per branch, not per prompt (when pr is true)
- [ ] PR body includes issue reference when set
- [ ] Auto-merge with PR waits for last prompt on branch
- [ ] No branch + pr false: current direct behavior unchanged
- [ ] Worktree + pr true: current PR behavior unchanged
- [ ] All existing tests pass
- [ ] `make precommit` passes

## Security

- Branch names from prompt/spec frontmatter are user-supplied and passed to git commands. Validation from the prerequisite spec (ref format check) must be enforced before any git operation. This spec does not add new validation — it relies on the prerequisite's branch validation.
- In-place branch switching operates on the user's repository directly. A failed execution on a feature branch must not leave the repository on that branch — the restore-to-default-branch step is safety-critical.
- Auto-merge to default branch is a destructive operation. Merge conflicts must result in a clear error with the branch preserved, never in silent data loss.

## Verification

```bash
make precommit
```

Manual verification steps:

1. **In-place branch continuity**: Set `pr: false, worktree: false`. Create two prompts with `branch: dark-factory/test`. Run first prompt. Expected: branch created, code committed. Run second prompt. Expected: second prompt sees first prompt's changes. Repository on default branch after each.
2. **Clone branch continuity**: Same as step 1 with `worktree: true`. Expected: clone checks out existing remote branch for second prompt.
3. **No release on branch**: After step 1, check `git tag`. Expected: no new tag. Check CHANGELOG. Expected: entries under `## Unreleased`.
4. **Auto-release after last prompt**: After last prompt with shared branch completes (pr: false), check `git log` on default branch. Expected: merge commit present, `## Unreleased` renamed to version, tag created and pushed.
5. **Idempotent PR**: Set `pr: true`. Run two prompts on same branch. Expected: one PR created, second prompt logs existing PR URL.
6. **PR auto-merge timing**: Set `pr: true, autoMerge: true`. Run two prompts on same branch. Expected: PR not merged after first prompt, merged after second.
7. **Dirty working tree**: Leave uncommitted changes, run prompt with branch. Expected: error before branch switch, no partial state.
8. **Merge conflict**: Create conflicting changes on default branch, then run last prompt. Expected: error logged, branch preserved, no release.
9. **Backward compat (no branch)**: Run prompt without branch field, `pr: false`. Expected: identical to current direct workflow — commit + release on default branch.

## Do-Nothing Option

Users must merge each prompt's PR individually before the next prompt can run, or manually set branch fields and hope the cloner handles existing branches (it currently doesn't). Multi-prompt specs are slow and fragile. The workaround is to use direct workflow on master with no branching, sacrificing code review.
