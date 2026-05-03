---
status: completed
approved: "2026-05-03T11:07:36Z"
generating: "2026-05-03T11:07:36Z"
verifying: "2026-05-03T11:49:40Z"
completed: "2026-05-03T12:14:20Z"
branch: dark-factory/bug-autorelease-overrides-pr-workflow
---

## Summary

- `autoRelease: true` silently bypasses the **entire branch/PR pipeline** — both the **separation** step (`workflow: branch` should create a feature branch) AND the **delivery** step (`pr: true` should open a PR)
- The daemon commits and tags directly on master, as if `workflow: direct` was set
- No feature branch exists locally or on remote — the separation step never runs
- The combination `branch + true + false + true` is logically inconsistent and undocumented in `workflows.md`
- No warning or error is logged — user expects PR review flow, gets direct release
- Reproducible across multiple prompts in the same project — every dark-factory commit landed linearly on master, never via merge commit
- Fix is two parts: (1) reject the inconsistent combination at config-load (fail-fast), (2) audit dispatch logic so `workflow != direct` always creates a branch regardless of delivery flags

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

### Why the combination is also semantically invalid

Independent of the dispatch bug, the combination `pr: true + autoMerge: false + autoRelease: true` has no defensible execution:

| What `autoRelease` requires | What `autoMerge: false` provides |
|-----------------------------|----------------------------------|
| Tag the merged code on master | Master never receives the change automatically |
| Push commits to remote default branch | Branch hasn't been merged yet |

Tagging an unmerged feature branch produces a release that doesn't reflect master. Tagging master without the merge means the tag points at a sha that's missing the change. Neither makes sense.

So this isn't "a combo we forgot to support" — it's logically impossible. Config-validation rejection (fail-fast) is the right answer for this combo.

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

### Bug reproduced itself during this triage

While writing this spec, the dark-factory daemon (running on its own repo) was processing prompt 362-spec-062. It picked up an uncommitted `docs/bug-workflow.md` from the working tree (unrelated to its prompt scope) and shipped it as part of v0.145.0 — direct to master, tagged. Documented in CHANGELOG v0.145.0 entry. This confirms the bug applies to the dark-factory repo itself, not just the reproduction project.

## Root Cause Hypothesis

Two suspects, both should be audited as part of the fix:

1. **Config validation:** the load-time validation does not reject the inconsistent combination `pr: true + autoMerge: false + autoRelease: true`. Precedent for the rejection pattern already exists at `workflows.md:87` (`workflow: direct + pr: true` is already rejected); this combo just isn't covered.

2. **Post-execution dispatch:** an early-return path on `autoRelease=true` appears to bypass the separation step regardless of `workflow:` value. The daemon log proves the branch step was never invoked, so something short-circuits before any branch-creation logic runs. The exact location needs code archaeology during the fix.

The fail-fast validation closes the immediate bug. The dispatch audit ensures no other delivery-flag combination can short-circuit the separation step in the future.

## Goal

1. **Fail-fast on the invalid combo:** `pr: true + autoMerge: false + autoRelease: true` is rejected at config load with a clear, actionable error message that lists the three valid resolutions (set `autoMerge: true`, set `autoRelease: false`, or set `pr: false`).
2. **Dispatch invariant:** for any `workflow` value that creates a separate branch (`branch`, `worktree`, `clone`), the separation step MUST always run before any delivery flag is evaluated. No early-return path may skip it. The fail-fast validation in #1 also applies to all three non-direct workflow values.
3. **No silent regressions:** existing valid combinations continue to work — `direct + autoRelease=true`, `branch + pr=true + autoMerge=true + autoRelease=true`, `branch + pr=true + autoMerge=false + autoRelease=false`, etc.

## Non-goals

- Defining a new "release after manual merge" auto-detection feature (would be a separate spec)
- Changing the existing `workflow: direct + pr: true` rejection (already correct)
- Adding new delivery flags or workflow modes
- Fixing v0.4.0–v0.4.3 history in jira-task-creator (these are now released; only forward behavior matters)

## Constraints

- The existing `direct + pr: true` rejection (workflows.md:87) must continue to work
- The dispatch fix must not change behavior of any combination currently in the workflows.md combinations table
- Error messages must be actionable — list specific config changes the user can make
- Validation must run at config-load (startup), not per-prompt — fail fast, not after the first prompt commits
- The four documented `*Source` traces in the effective-config log line must still emit correctly after validation passes
- No code-path may write to master without going through the branch+PR path when `pr: true` is set
- `autoRelease` semantics for `workflow: direct` are unchanged (commit + push + tag on current branch)

## Acceptance Criteria

- [ ] Starting daemon or `run` with `pr: true + autoMerge: false + autoRelease: true` exits non-zero with the explanation message before any prompt is processed
- [ ] Error message names all three valid resolutions
- [ ] Starting daemon with valid combos (5+ representative configs from workflows.md combinations table) succeeds
- [ ] With `workflow: branch + pr: true + autoMerge: true + autoRelease: true`: prompt completes → feature branch created → PR opened → auto-merged → master tagged. Verified by `git log --oneline --graph` showing a merge commit, not a linear commit
- [ ] With `workflow: direct + autoRelease: true`: prompt completes → direct commit + tag on master (regression test)
- [ ] With `workflow: branch + pr: true + autoMerge: false + autoRelease: false`: prompt completes → branch + PR opened → daemon stops at "await manual merge" → no tag created
- [ ] Daemon log emits a clear line for each step taken (branch create, push, PR create, merge) — no silent transitions
- [ ] Validation unit test exists for the new rejection rule, including positive cases (valid combos)
- [ ] Scenario file exists exercising the rejection (config-load failure) end-to-end
- [ ] Scenario file exists exercising the happy `branch + pr=true + autoMerge=true + autoRelease=true` path end-to-end (real branch, real PR, real merge on a test repo)

## Failure Modes

| Trigger | Current (buggy) | Expected |
|---------|-----------------|----------|
| Daemon start with `pr: true + autoMerge: false + autoRelease: true` | Daemon runs; first prompt commits direct to master | Daemon refuses to start; non-zero exit; explanatory error |
| Daemon start with `workflow: direct + pr: true` | Already rejected (workflows.md:87) | (unchanged) |
| Prompt completes, `branch + pr=true + autoMerge=true + autoRelease=true` | Should work but currently bypasses branch step (same dispatch bug) | Branch + PR + auto-merge + tag |
| Prompt completes, `branch + pr=true + autoMerge=false + autoRelease=false` | (Untested — may also be affected by dispatch bug) | Branch + PR opened, daemon stops; no tag |
| Prompt completes, `direct + autoRelease=true` | Works correctly today | (unchanged — regression test required) |
| Multiple prompts queued, PR workflow | Each silently lands on master | Each opens its own PR awaiting review (when autoMerge=false) |
| Daemon picks up uncommitted files unrelated to prompt scope | Bundled into the autoRelease commit + tag (observed during this triage) | Out of scope for this spec — separate working-tree-isolation concern |

## Do-Nothing Option

Cost: continued silent direct-to-master commits. Loses PR review gate, branch protection bypassed, audit trail polluted. The bug already produced 7 unreviewed releases in `jira-task-creator` (v0.2.0 through v0.4.3) and 1 in `dark-factory` itself (v0.145.0). Every additional dark-factory user adopting PR workflows hits this. Not viable.

## Verification

Run all of these on a clean test repo (not the dark-factory repo, to avoid the bug interfering with its own fix):

```bash
# 1. Validation rejection
echo 'pr: true
autoMerge: false
autoRelease: true' > .dark-factory.yaml
dark-factory daemon
# Expected: non-zero exit with the actionable error

# 2. Branch + PR + merge happy path
echo 'workflow: branch
pr: true
autoMerge: true
autoRelease: true' > .dark-factory.yaml
# ...approve a prompt, run daemon
git log --oneline --graph -5
# Expected: a merge commit appears (not linear)

# 3. Direct + autoRelease regression
echo 'workflow: direct
autoRelease: true' > .dark-factory.yaml
# ...approve a prompt, run daemon
# Expected: direct commit + tag on master (current behavior)

# 4. Scenarios pass
go test ./scenarios/...
```

`make precommit` must pass. New scenarios under `scenarios/` exercise the validation rejection and the happy path end-to-end against a real test repo with real `gh pr` calls.

## Open questions (for prompt-time discussion)

- (Resolved in Goal #2: rejection applies to `branch`, `worktree`, and `clone` uniformly. The inconsistency argument is independent of separation mode.)
- Should the validation message be one shared error for all three workflow values, or per-workflow? (Lean shared — message is about the delivery-flag combo, not the workflow.)
- Should `autoRelease` get a per-workflow override, e.g. `autoReleaseOn: master|branch` to make the intent explicit? (Out of scope for this bug — file as a separate feature spec if useful.)
- During the dispatch audit, is there a single short-circuit branch to remove, or does each delivery flag have its own bypass path that all need to be unified into a single dispatch table? (Code archaeology required.)
