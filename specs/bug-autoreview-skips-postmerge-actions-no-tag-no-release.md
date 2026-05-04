---
status: idea
kind: bug
---

# `autoReview` approval merges PR but skips `postMergeActions` → no master pull, no tag, no release

## Summary

When a project is configured with `pr: true + autoMerge: true + autoRelease: true + autoReview: true + allowedReviewers/useCollaborators`, dark-factory opens the PR and parks the prompt in `in_review`. Once an allowed reviewer approves, the `ReviewPoller` calls `WaitAndMerge` (PR is merged on the GitHub side via `gh pr merge --merge --delete-branch`) and then `MoveToCompleted` (prompt moved to `prompts/completed/`). However, the poller does NOT call `postMergeActions` — so the local clone never pulls the merged commit, the changelog `## Unreleased` is not promoted to `## vX.Y.Z`, no tag is created, no `git push origin vX.Y.Z` runs.

The same project, configured identically except with `autoReview: false`, produces a release tag automatically because the daemon-merges-it-itself path goes through `handleAutoMergeForClone → postMergeActions`. Two paths, asymmetric behavior, neither documented.

## Reproduction

dark-factory version: built from master at `pkg/review/poller.go` HEAD (`v0.148.1` ships this code).

1. Project: `~/Documents/workspaces/jira-task-creator`. Has `CHANGELOG.md` with `## Unreleased` entries.
2. `.dark-factory.yaml`:
   ```yaml
   workflow: worktree
   pr: true
   autoMerge: true
   autoRelease: true
   autoReview: true
   allowedReviewers: ["bborbe", "pr-review-of-ben"]
   ```
3. Drop a draft prompt in `prompts/`, approve it, run daemon. Container produces a commit + a `## Unreleased` CHANGELOG entry.
4. Daemon opens PR (e.g. `https://github.com/bborbe/jira-task-creator/pull/5`), flips prompt to `status: in_review`.
5. An allowed reviewer (`bborbe`) clicks Approve on the GitHub PR.
6. `ReviewPoller.pollOnce` (next polling tick) → `handleApproved` → `WaitAndMerge` → `MoveToCompleted`. Daemon log:
   ```
   level=INFO msg="approved" file=NNN-...md
   level=INFO msg="merged PR" url=https://github.com/.../pull/5
   level=INFO msg="moved approved prompt to completed"
   ```
7. **Missing log lines** (compared to the autoMerge-without-autoReview path):
   - `level=INFO msg="merged PR and updated default branch" branch=master`
   - `level=INFO msg="created tag" tag=v0.4.3`
   - `level=INFO msg="pushed tag"`
8. State on disk after: master branch in the local clone has NOT been fast-forwarded; `CHANGELOG.md` still has `## Unreleased`; `git tag -l` shows no new tag; `git rev-list @{u}..HEAD --count` is non-zero on origin (the merge commit GitHub created via squash/merge is not pulled locally).

## Expected vs Actual

**Expected** (per `docs/configuration.md` `autoRelease` semantics and `docs/release-process.md` "Binary Release"):
> When `autoRelease: true` and `CHANGELOG.md` exists, after each successful prompt: stage all changes, determine bump from changelog content, rename `## Unreleased` → `## vX.Y.Z`, commit `release vX.Y.Z`, tag, `git push` + `git push origin vX.Y.Z`.

**Actual:**
- For `autoReview: false` runs: this works — `handleAutoMergeForClone` (`workflow_helpers.go:181`) calls `postMergeActions` (`workflow_helpers.go:155`) which switches to default branch, pulls, and runs `handleDirectWorkflow` for the release tag if `AutoRelease && Releaser.HasChangelog`.
- For `autoReview: true` runs: `reviewPoller.handleApproved` (`pkg/review/poller.go:204-218`) only calls `prMerger.WaitAndMerge` and `promptManager.MoveToCompleted`. It does NOT call `postMergeActions`. There is no second code path that picks up the slack — the daemon's main loop has already moved on to other prompts when the poller's tick fires.

## Why this is a bug

The `autoRelease` config flag, the `CHANGELOG.md` presence, and the "binary release happens automatically" promise of `docs/release-process.md` are all worded as workflow-independent: the operator turns the flag on, dark-factory tags releases on merge. The autoReview path silently breaks that promise. There is no log line announcing "skipping release because autoReview", no validation error at config load, no documentation note.

This is the third bug in the same dispatch family (063 = autoRelease bypassing PR creation; 065 = PR-create missing `--head`; 066 = branch-workflow rejecting its own bookkeeping write). All three are about asymmetric behavior across workflow / delivery-flag combinations — invariants the docs imply but the code only honors on the most-traveled path.

## Workaround

After every autoReview-merged prompt, the operator must manually:

```bash
cd ~/Documents/workspaces/<project>
git fetch
git pull origin master                      # pull the squash-merge commit
# Edit CHANGELOG.md: rename "## Unreleased" → "## vX.Y.Z"
sed -i '' "s/^## Unreleased\$/## v0.4.3/" CHANGELOG.md
git add CHANGELOG.md
git commit -m "release v0.4.3"
git tag v0.4.3
git push
git push origin v0.4.3
```

This defeats the point of `autoRelease`.

## Code pointers

- `pkg/review/poller.go:204-218` — `handleApproved` does merge + move-to-completed, nothing else.
- `pkg/processor/workflow_helpers.go:155-178` — `postMergeActions` is the missing call: switches to default branch, pulls, then `handleDirectWorkflow` for the tag if `AutoRelease && HasChangelog`.
- `pkg/processor/workflow_helpers.go:181-211` — `handleAutoMergeForClone` (the working path) shows the correct sequence: `WaitAndMerge` → `MoveToCompleted` → `postMergeActions`.
- `pkg/factory/factory.go:971` (`NewReviewPoller` call site) — the poller does NOT receive `Releaser`, `Brancher`, or `AutoRelease bool`. Adding postMergeActions requires threading these through.

## Failure Modes

| Trigger | Expected behavior | Recovery / verification |
|---------|-------------------|--------------------------|
| `pr+autoMerge+autoRelease+autoReview` config; reviewer approves PR | After review-poller merges, master is pulled locally, CHANGELOG `## Unreleased` is bumped, tag created, tag pushed | Daemon log shows "merged PR and updated default branch", "created tag", "pushed tag" — same lines as the autoMerge-only path |
| Same config, but `CHANGELOG.md` absent | Merge happens, master is pulled, no tag (no changelog → no version) | Daemon log shows pull-only; same as `handleDirectWorkflow` early-return at `workflow_helpers.go:128` |
| Same config, `autoRelease: false` | Merge happens, master is pulled (so the local clone is consistent with origin), no tag | Both autoMerge-only and autoReview paths converge here today; verify no regression |
| `autoReview: false` (existing happy path) | autoMerge path continues to work — pull + tag + push as today | Existing scenarios pass; no regression |
| Approval comes between two daemon ticks | Same outcome — postMergeActions runs once per merged PR | Idempotent: re-running on an already-released commit is a no-op |
| PR has conflicts; reviewer approves anyway | `WaitAndMerge` returns conflict error before `postMergeActions` runs; prompt stays `in_review`, no tag created | Existing error path preserved |
| Reviewer is not in `allowedReviewers` | Approval ignored; no merge, no postMergeActions | Existing filter behavior preserved |

## Goal

After this fix, a project configured with `pr+autoMerge+autoRelease+autoReview` produces the SAME end state as a project configured with `pr+autoMerge+autoRelease` (no autoReview) once the human review approval is in: master fast-forwarded, CHANGELOG bumped, tag created and pushed, prompt in `prompts/completed/`. The only behavioral difference between the two is *who* triggers the merge (daemon vs reviewer click).

## Constraints

- Do NOT change `WaitAndMerge` behavior — merge happens via `gh pr merge --merge --delete-branch` as today.
- Do NOT change `allowedReviewers` / `useCollaborators` semantics — the filter for "is this approval valid" is unchanged.
- Do NOT change the prompt-status transitions (`in_review` → `completed` on approve, `in_review` → `failed` on retry-limit changes-requested).
- Do NOT introduce a separate "release after autoReview" flag — `autoRelease: true` already means "release on merge"; the poller must honor it.
- Do NOT auto-tag a release without `CHANGELOG.md` present (matches existing autoMerge-only path).
- Do NOT regress the autoMerge-only path (autoReview: false) — `handleAutoMergeForClone → postMergeActions` continues to work as today.
- Do NOT stuff `postMergeActions` logic inline into the poller — extract or reuse the existing function so both call sites share one implementation.
- Reuse existing `Releaser` / `Brancher` interfaces via the `WorkflowDeps`-equivalent dependency-injection pattern; do not introduce a parallel git-aware abstraction in `pkg/review/`.

## Verification

Per `docs/bug-workflow.md` §Verification, this is a runtime symptom — unit tests alone are not sufficient.

**Repro replay (must run after fix lands):**

```bash
# In jira-task-creator with the autoReview config above and a CHANGELOG.md:
cd ~/Documents/workspaces/jira-task-creator
git log --oneline -1                       # baseline tip
git tag -l | tail -3                       # baseline tags

# Drop a small prompt that produces a CHANGELOG entry
# (e.g. "add comment to handler.go + ## Unreleased: docs comment in handler")
dark-factory prompt approve <name>
dark-factory daemon &
DAEMON_PID=$!

# Wait for PR to open and prompt to flip to in_review
# Approve the PR on GitHub as bborbe (allowed reviewer)
# Wait one pollInterval

# Expected within ~pollInterval seconds of approval:
#   level=INFO msg="merged PR" url=...
#   level=INFO msg="merged PR and updated default branch" branch=master
#   level=INFO msg="created tag" tag=v0.4.3
#   level=INFO msg="pushed tag"

# Confirm:
git fetch --tags
git tag -l | tail -3                       # new vX.Y.Z appears
git log --oneline @{u}..HEAD               # empty — local is in sync with origin
grep -A2 "^## v0.4.3" CHANGELOG.md         # changelog promoted

kill $DAEMON_PID
```

**Negative-control replays:**

1. Same config but remove `CHANGELOG.md`. Approve the PR. Expected: merge + master pull, NO tag created, daemon log explains "no changelog, skipping release tag" (or equivalent existing log line from `handleDirectWorkflow:128`).
2. `autoRelease: false`. Approve the PR. Expected: merge + master pull, no tag. Same outcome as autoMerge-only path with `autoRelease: false`.

**Acceptable evidence for `verifying → completed`:**

| Evidence | Acceptable? |
|----------|-------------|
| Daemon log shows `created tag` after autoReview approval | Yes |
| `git tag -l` shows the new tag locally and on origin | Yes |
| `CHANGELOG.md` has `## vX.Y.Z` (no `## Unreleased`) | Yes |
| Unit test asserting `handleApproved` calls `postMergeActions` | Necessary but not sufficient |
| "All tests pass" without runtime replay | No |

## Open Questions

1. Should `postMergeActions` be moved to a shared package (`pkg/release/`?) or stay in `pkg/processor/` and the review poller depend on it? Cross-package call from `pkg/review/` to `pkg/processor/` is awkward but cheaper than a refactor. Triage decision before approval.
2. The poller currently doesn't have `Releaser`/`Brancher` dependencies. Adding them is a constructor-signature change — affects `factory.go:971` and the counterfeiter mock. Acceptable if it's the right shape.
3. `autoMerge` is required for autoReview today (`pkg/config/config.go:validateAutoReview` requires it). Confirm: this fix preserves that invariant and does not enable autoReview-without-autoMerge as a side effect.
4. Should the fix also address the case where `autoMerge: false + autoReview: true`? Today that's rejected by validation, so no behavioral change needed; but if the validation ever loosens, the poller must continue to refuse to merge (which `WaitAndMerge` does explicitly via the merge command, so this is naturally safe — confirm during fix-prompt generation).

## See also

- Spec 063 (`bug-autorelease-overrides-pr-workflow`) — autoRelease bypassing PR creation.
- Spec 065 (`bug-pr-create-missing-head-flag-in-isolated-workflows`) — PR-create missing `--head`.
- Spec 066 (`bug-branch-workflow-rejects-its-own-uncommitted-prompt-file-write`) — branch workflow rejecting bookkeeping writes.
- `pkg/processor/workflow_helpers.go:155-211` — the working post-merge sequence to replicate.
- `pkg/review/poller.go:203-218` — the rejection site (handleApproved).
- `docs/release-process.md` — the contract this bug violates.
- `docs/configuration.md` (autoRelease section) — same.
