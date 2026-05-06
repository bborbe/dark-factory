---
status: verifying
approved: "2026-05-05T22:28:02Z"
generating: "2026-05-05T22:28:44Z"
prompted: "2026-05-05T22:41:33Z"
verifying: "2026-05-06T07:35:33Z"
branch: dark-factory/simplify-merge-gate-by-relying-on-mergestatestatus
---

# Simplify the merge gate by relying on `mergeStateStatus` instead of a custom review fetcher + `allowedReviewers`

## Summary

Today dark-factory has two parallel paths after PR creation:

- **autoMerge:true / autoReview:false** — `WaitAndMerge` polls `mergeStateStatus`, merges when `CLEAN`. This works (after spec 072) and is simple.
- **autoReview:true** — `ReviewPoller` polls `gh pr view --json reviews`, filters by `allowedReviewers`, on first APPROVED review calls `WaitAndMerge` + `MoveToCompleted` + `PostMergeActions`.

Both ultimately want the same outcome: "merge when GitHub considers the PR ready, then run release ceremony." The autoReview path adds:

1. A custom review fetcher that queries `.reviews[]` (which empirically returns `[]` even when GitHub has approved the PR — see spec 072 PR #6: `reviewDecision: APPROVED` but `reviews: []`, `latestReviews: []`, no timeline events).
2. A custom `allowedReviewers` allowlist — duplicates GitHub's branch protection role of "who can approve."
3. A fix-prompt generation flow on `CHANGES_REQUESTED` — separate concern, removed here and may return as a `autoFix`-style follow-up spec.

GitHub's `mergeStateStatus == CLEAN` already encodes "all required reviews received, all required checks passed, no conflicts, all branch protection satisfied." The custom poller is reinventing GitHub's merge gate.

This spec collapses the two paths into one. `WaitAndMerge` is the only merge gate. Branch protection enforces who can approve. `autoReview`, `ReviewPoller`, `FetchLatestReview`, `allowedReviewers`, `useCollaborators`, and the changes-requested fix-prompt generator are removed in one shot — no soft deprecation. dark-factory has a single user (bborbe); the migration cost is one config edit.

## Problem

- Custom review fetcher hits GitHub API quirks (`reviews: []` when approved, observed in spec 072 verification).
- Two code paths to maintain (`handleAutoMergeForClone` and `reviewPoller.handleApproved`).
- `allowedReviewers` config duplicates branch protection's purpose; users have to configure both for consistency.
- `autoReview` adds a 600+-line `ReviewPoller` for what amounts to "wait until GitHub says mergeable, then merge."

## Goal

`autoMerge: true` is the only "merge when ready" mode. `WaitAndMerge` polls until `mergeStateStatus == CLEAN`, then merges, then runs `PostMergeActions`. Branch protection (configured per-repo on GitHub) enforces review requirements. The `autoReview` config flag, the `ReviewPoller`, and `allowedReviewers` are removed.

## Non-goals

- Re-implementing the AI-driven fix-prompt-on-changes-requested workflow under a different flag. If wanted, file a follow-up spec for `autoFix` reading PR comment threads.
- Changing `mergeStateStatus` polling behavior (already correct after spec 072).
- Bitbucket Server merger — separate path, not currently affected.
- Soft deprecation / two-phase migration — single user, one config edit.

## Desired Behavior

1. With `autoMerge: true + pr: true`, daemon opens PR, polls `mergeStateStatus` until `CLEAN`, merges, runs `PostMergeActions`.
2. With `autoMerge: true` and branch protection requiring N reviews from team X: daemon polls and waits — `mergeStateStatus` stays `BLOCKED` until reviews land, then flips to `CLEAN`, daemon merges.
3. With `autoMerge: false + pr: true`: daemon opens PR and stops; operator merges manually or via separate tooling.
4. `autoReview: true`, `allowedReviewers`, `useCollaborators` config fields are removed. Config containing them fails to load with a clear error message naming each removed field and pointing to branch protection as the replacement.
5. `pkg/review/` package, `pkg/git/review_fetcher.go`, `pkg/git/bitbucket_review_fetcher.go`, the changes-requested fix-prompt generator, and the daemon's review-poller wiring in `pkg/factory/factory.go` `CreateReviewPoller` are deleted.

## Constraints

- Do NOT regress the `mergeStateStatus == CLEAN → merge → PostMergeActions` flow proven by spec 072.
- Do NOT remove `PostMergeActions` — it's the shared post-merge ceremony.
- Do NOT touch Bitbucket Server's `bitbucketPRMerger.WaitAndMerge` semantics.
- Removed config fields must surface a friendly error (per-field error message) at config-load time, not a generic "unknown field" yaml warning.
- The `validateAutoReview` function (`pkg/config/config.go:461-475`) is removed entirely.
- Existing scenarios that exercise the autoMerge path (e.g. scenario 002 `workflow-pr.md`) must still pass.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| PR opens, branch protection requires review, no review yet | Daemon polls, sees `mergeStateStatus: BLOCKED`, keeps polling | Reviewer approves on GitHub → state flips to CLEAN → daemon merges |
| PR opens, no branch protection | Daemon polls, sees `CLEAN` immediately, merges | No human gate (operator's choice — they didn't configure protection) |
| Legacy config has `autoReview: true` | Config load fails with `unknown field "autoReview" — autoReview was removed in <vX.Y.Z>; configure GitHub branch protection on the repo to enforce review requirements` | Operator removes the field and configures branch protection |
| Legacy config has `allowedReviewers: [...]` | Same error pattern, naming `allowedReviewers` and the migration | Same |
| Legacy config has `useCollaborators: true` | Same error pattern | Same |
| GitHub API returns inconsistent state (`mergeStateStatus: UNKNOWN` for >N polls) | Existing timeout in `WaitAndMerge` fires | Operator investigates GitHub-side issue |
| User wants AI-driven fix-prompt on changes-requested | File a follow-up `autoFix` spec; this spec removes the existing implementation | Out of scope here |

## Security / Abuse

Removing `allowedReviewers` shifts trust entirely to GitHub branch protection. A repo without branch protection now auto-merges any PR dark-factory opens — including PRs that previously would have been gated by `allowedReviewers`. This is a downgrade for repos that relied on `allowedReviewers` as the only gate. Mitigation: spec verification includes a check that branch protection IS configured on each active dark-factory repo before this spec lands; CHANGELOG entry calls this out explicitly.

## Do-Nothing Option

Keep both paths. Continue maintaining the review fetcher with its API quirks. Users with autoReview keep configuring `allowedReviewers` separately from branch protection. Cost: ongoing maintenance of two merge paths, periodic GitHub API surprises, doc burden explaining when to use which flag, and a 600+ LoC poller that exists only because GitHub's merge-gate signal was previously misunderstood.

## Acceptance Criteria

- [ ] `pkg/review/poller.go` and the entire `pkg/review/` package no longer exist.
- [ ] `pkg/git/review_fetcher.go` and `pkg/git/bitbucket_review_fetcher.go` no longer exist.
- [ ] `pkg/config/config.go` no longer contains `AutoReview`, `AllowedReviewers`, `UseCollaborators` fields or `validateAutoReview` function.
- [ ] Config load surfaces a per-field friendly error (naming the removed field + pointing to branch protection) when any of the three removed fields is present in `.dark-factory.yaml`.
- [ ] `pkg/factory/factory.go` no longer contains `CreateReviewPoller` or its call site in `CreateRunner`.
- [ ] `pkg/runner/runner.go` `Runner` interface and constructor no longer accept a `reviewPoller` parameter.
- [ ] All references to `MaxReviewRetries`, `PollIntervalSec` (when used only by the review poller — verify), and `pkg/review/...` are removed from `pkg/factory`, `pkg/runner`, and `cmd/`.
- [ ] `make precommit` exits 0.
- [ ] Scenario 002 (`workflow-pr.md`) and scenario 003 (`smoke-test-container.md`) — and any existing autoMerge end-to-end scenario — still pass without modification.
- [ ] The `autoMerge: true + pr: true` flow proven by spec 072 PR #7 (merged + tagged v1.1.1 via WaitAndMerge → PostMergeActions) is reproducible after this spec lands.
- [ ] CHANGELOG.md `## Unreleased` entry added: `BREAKING: removed autoReview, allowedReviewers, useCollaborators config fields. Use GitHub branch protection to gate merges.`

## Verification

```bash
cd ~/Documents/workspaces/dark-factory
make precommit
```

**Runtime replay** (mirrors the spec 072 verification):

```bash
# In dark-factory-sandbox with autoMerge: true + pr: true + autoRelease: true:
cd ~/Documents/workspaces/dark-factory-sandbox
git status                                # baseline
git tag -l | tail -3                      # baseline tags

# Drop a small prompt that produces a CHANGELOG entry; approve and run daemon
dark-factory prompt approve <name>
dark-factory run

# Expected log lines (same as spec 072 PR #7):
#   level=INFO msg="created PR" url=...
#   level=INFO msg="merged PR and updated default branch" branch=master
#   level=INFO msg="committed and tagged" version=vX.Y.Z

# Confirm:
git fetch --tags
git tag -l | tail -3                       # new vX.Y.Z appears
```

**Negative-control replay** — legacy config rejection:

```bash
# Add `autoReview: true` to .dark-factory.yaml temporarily
echo "autoReview: true" >> .dark-factory.yaml
dark-factory daemon
# Expected: daemon exits non-zero with:
#   "unknown field \"autoReview\" — autoReview was removed in vX.Y.Z; configure GitHub branch protection..."
git checkout -- .dark-factory.yaml
```

**Acceptable evidence for `verifying → completed`:**

| Evidence | Acceptable? |
|----------|-------------|
| `make precommit` exits 0 | Necessary, not sufficient |
| Spec 072 verification flow (autoMerge → PostMergeActions → tag) reproducible | Yes |
| Legacy `autoReview: true` config rejected with friendly error | Yes |
| Existing scenarios pass | Yes |
| Code inspection only ("the package is gone") | No — must run runtime replays |

## See also

- Spec 071 — fixed `handleApproved → PostMergeActions` (the autoReview path that this spec removes). The shared `PostMergeActions` function survives in `pkg/processor/workflow_helpers.go`.
- Spec 072 — fixed `WaitAndMerge` to recognize `CLEAN`, fixed routing precedence. After this spec, the routing fork disappears (only `if deps.AutoMerge` remains).
- `pkg/review/poller.go` — the ~600-line `ReviewPoller` to delete.
- `pkg/git/review_fetcher.go`, `pkg/git/bitbucket_review_fetcher.go` — to delete.
- `pkg/config/config.go` lines 461-475 (`validateAutoReview`) and the three field declarations — to delete.
- `pkg/factory/factory.go` `CreateReviewPoller` and its call site — to delete.
- `pkg/git/pr_merger.go` — `WaitAndMerge` is the single merge gate after this spec.
