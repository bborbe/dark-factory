---
status: draft
---

# Auto-Merge and Auto-Release for PR Workflow

## Problem

When `workflow: pr` or `workflow: worktree` is configured, dark-factory creates a PR and moves on. The PR then requires manual merge, manual changelog finalization, and manual tag+push. This breaks the autonomous pipeline — a human must intervene between every prompt and its release.

## Goal

After creating a PR, dark-factory optionally watches it until mergeable, merges it to the default branch, and performs a full release (changelog update, tag, push). Two config flags control this: `autoMerge` and `autoRelease`. The default branch is detected dynamically (works with `main`, `master`, or any custom name).

## Non-goals

- No support for non-GitHub remotes (relies on `gh` CLI)
- No custom merge strategies (always `--merge`, not squash or rebase)
- No PR approval workflow (waits for GitHub mergeability, not human approval)
- No configurable poll interval or timeout (hardcoded 30s / 30min)
- No retry on transient GitHub API failures

## Desired Behavior

### Config

Two new fields in `.dark-factory.yaml`:

```yaml
autoMerge: false    # poll PR until mergeable, then merge
autoRelease: false  # after merge: update changelog, tag, push
```

Validation:
- `autoMerge: true` requires `workflow: pr` or `workflow: worktree`
- `autoRelease: true` requires `autoMerge: true`

### Default Branch Detection

A new `DefaultBranch()` method queries GitHub for the repo's default branch:

```
gh repo view --json defaultBranchRef --jq '.defaultBranchRef.name'
```

This replaces all hardcoded `master` references, including the existing `MergeOriginMaster` method (renamed to `MergeOriginDefault`).

### Auto-Merge Flow (when `autoMerge: true`)

After PR creation (step 8 in spec 010):

1. Poll `gh pr view <url> --json mergeStateStatus` every 30 seconds
2. If `MERGEABLE` → merge with `gh pr merge <url> --merge --delete-branch`
3. If `CONFLICTING` → fail immediately
4. If timeout (30 min) → fail
5. Switch to default branch and `git pull`
6. If `autoRelease: true` and `CHANGELOG.md` exists → run direct workflow (rename `## Unreleased` to version, commit, tag, push)

### When `autoMerge: false` (default)

Behavior unchanged from spec 010 — create PR, switch back to original branch, move on.

## Constraints

- Requires `gh` CLI installed and authenticated (same as spec 010)
- `autoRelease` only works when `CHANGELOG.md` with `## Unreleased` section exists
- PR merge polling is blocking — no other prompts execute while waiting
- Default branch detection requires GitHub remote (not purely local git)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| PR has merge conflicts | `WaitAndMerge` returns error immediately, switch back to original branch | Resolve conflicts manually, re-queue prompt |
| Poll timeout (30 min) | Error returned, switch back to original branch | Check CI status, merge manually |
| `gh` not authenticated | Error during merge or default branch detection | Run `gh auth login` |
| No `CHANGELOG.md` | `autoRelease` silently skips release (commit-only) | Expected behavior for projects without changelog |
| Network failure during poll | Error returned | Retry by re-queueing prompt |
| Default branch detection fails | Error returned, processing stops | Ensure GitHub remote is configured |

## Acceptance Criteria

- [ ] `autoMerge: true` polls PR until mergeable and merges
- [ ] `autoMerge: true` with conflicting PR fails immediately (no 30-min wait)
- [ ] `autoRelease: true` updates changelog, tags, and pushes after merge
- [ ] `autoRelease: true` without `CHANGELOG.md` skips release gracefully
- [ ] Default branch detected dynamically (no hardcoded `master`)
- [ ] `MergeOriginMaster` renamed to `MergeOriginDefault` using dynamic detection
- [ ] Config validation: `autoMerge` requires PR/worktree workflow
- [ ] Config validation: `autoRelease` requires `autoMerge`
- [ ] On merge failure, original branch is restored for next prompt
- [ ] Works with both `workflow: pr` and `workflow: worktree`

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Keep PR workflow as fire-and-forget. Humans merge PRs, finalize changelogs, and push tags manually. Works for review-heavy workflows but breaks autonomous operation.
