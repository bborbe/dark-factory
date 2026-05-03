---
status: idea
---

## Summary

- `autoRelease: true` silently bypasses the **entire branch/PR pipeline** — both the **separation** step (`workflow: branch` should create a feature branch) AND the **delivery** step (`pr: true` should open a PR)
- The daemon commits and tags directly on master, as if `workflow: direct` was set
- No feature branch exists locally or on remote — the separation step never runs
- The combination `branch + true + false + true` is undocumented in `workflows.md` (only `branch + true + false + false` and `branch + true + true + true` are listed)
- No warning or error is logged — user expects PR review flow, gets direct release
- Reproducible across multiple prompts in the same project — every dark-factory commit landed linearly on master, never via merge commit

## Problem

Configured a project with the documented PR-review workflow:

```yaml
workflow: branch
pr: true
autoMerge: false   # human reviews + merges manually
autoRelease: true  # tag releases automatically
```

Per `docs/workflows.md` and the orthogonal-flags rationale, this should produce: feature branch + commit + push branch + open PR + await manual merge. After human merge, autoRelease handles tagging on master.

**Actual behavior (dark-factory v0.143.0-5-g73d1db8):** dark-factory bypassed branch/PR creation entirely and committed directly to master with a release tag.

### Why this is two bugs in one

`workflow:` controls **separation** (branch vs direct vs worktree vs clone). `pr:`, `autoMerge:`, `autoRelease:` are orthogonal **delivery** flags (per workflows.md:3). With `workflow: branch`, the branch step MUST run regardless of any delivery flag — that's the whole point of orthogonality.

Yet here, `autoRelease: true` skips:
1. **Separation:** `git checkout -b <promptBranch> origin/<defaultBranch>` (per workflows.md:34) — never runs
2. **Delivery:** `git push origin <promptBranch>` + `gh pr create` — never runs

Both phases are short-circuited in favor of `git commit + git tag + git push origin master`. Effectively `autoRelease: true` is being treated as a synonym for `workflow: direct + autoRelease: true`, which is wrong.

### Reproduction

1. Project: `~/Documents/workspaces/jira-task-creator`
2. `.dark-factory.yaml`:
   ```yaml
   workflow: branch
   pr: true
   autoRelease: true
   validationPrompt: docs/dod.md
   # autoMerge defaults to false
   ```
3. `dark-factory prompt approve <name>` and run daemon
4. After successful prompt completion, observe:
   - No feature branch created (locally or on remote)
   - No PR opened (`gh pr list` shows nothing for this prompt)
   - Master HEAD has the commit + a release tag
   - Daemon log shows: `committed and tagged version=vX.Y.Z` directly after `completion report` — no `pushing branch`, `creating PR`, or `gh pr create` lines

### Effective config logged

```
workflow=branch pr=true autoRelease=true autoMerge=false verificationGate=false
```

So the daemon read the config correctly. The bug is in the post-execution dispatch that chooses between "branch + push + PR" and "direct commit + tag".

### Daemon log evidence

```
10:58:08.972 INFO completion report status=success
10:58:08.985 INFO moved to completed file=008-migrate-tools-go.md
10:58:10.847 INFO committed and tagged version=v0.4.2
```

Missing log lines that should have appeared:
- `creating feature branch from origin/<defaultBranch>`
- `pushing branch origin/<promptBranch>`
- `creating pull request`
- `gh pr create ...`

### Git state proof

```
git log --oneline --graph -5
* c8d5c45 release v0.4.2          ← linear on master, no merge commit
* 1d93439 move prompt to completed
* f03dc8f release v0.4.1
*   eb7b28c Merge branch 'master' (#3 - last legitimate PR merge)
```

**Every dark-factory commit in this project's history is linear on master** — `release v0.2.0`, `v0.2.1`, `v0.3.0`, `v0.3.1`, `v0.4.0`, `v0.4.1`, `v0.4.2` — none of them via PR. Real PR merges (only ones produced by humans, marked `(#1)`, `(#2)`, `(#3)`) are clearly distinguishable.

`git branch -a` shows no `dark-factory/*` or feature branch ever existed on remote for these prompts. The separation step never created any branch.

## Goal

`workflow: branch + pr: true + autoMerge: false + autoRelease: true` should:

1. Create feature branch from `origin/<defaultBranch>`
2. Commit changes on the feature branch
3. Push the feature branch
4. Open a PR
5. **Stop and await manual merge** (autoMerge=false)
6. Only run release/tag logic AFTER the PR is merged (out-of-band signal: human merges → daemon detects on next cycle → tag on master)

OR — if this combination is intentionally invalid — reject it at config validation with a clear error message naming the conflict.

## Scope

### Must change

- Post-execution dispatch: when `pr: true && workflow != direct`, the PR-creation path must run regardless of `autoRelease`
- `autoRelease: true` must NOT short-circuit the PR path
- If autoMerge=false: tag/release logic should defer until PR is merged on master (or stay disabled until next prompt that runs against a merged master)

### Don't change

- Existing `branch + pr=true + autoMerge=true + autoRelease=true` flow (works correctly today)
- Existing `direct + autoRelease=true` flow (works correctly today)
- The combinations table semantics in `workflows.md`

## Failure modes affected

| Trigger | Current (buggy) | Expected |
|---------|-----------------|----------|
| Prompt completes, autoMerge=false, autoRelease=true | Direct commit to master + tag | Feature branch + PR opened, master untouched until manual merge |
| Prompt fails, autoRelease=true | (likely fine — release path doesn't fire) | (no change) |
| Multiple prompts queued, PR workflow | Each silently lands on master | Each opens its own PR awaiting review |

## Workaround

Set `autoRelease: false` to force the PR path. After manual merge, manually tag the release. Loses the autorelease convenience but restores the PR review gate.

## Open questions

- Is the bug in the workflow dispatch (chose `direct` path despite `workflow: branch`) or in the autoRelease handler (ran direct-commit logic regardless of workflow)?
- Should the combinations table in `workflows.md` document this case (`branch + true + false + true`) explicitly, or is it intentionally rejected?
- Does this affect `clone + pr=true + autoMerge=false + autoRelease=true` and `worktree + pr=true + autoMerge=false + autoRelease=true` the same way? (Reproduction was only with `workflow: branch`.)
- What's the right tag-release moment when autoMerge=false: (a) defer until master moves, (b) tag on the feature branch (wrong — pollutes tag space with unmerged refs), or (c) disable autoRelease entirely when autoMerge=false?
- Should config validation reject `pr: true + autoMerge: false + autoRelease: true` as logically inconsistent (no merge signal → no clean release point)?
