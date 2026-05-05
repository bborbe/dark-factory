---
status: completed
approved: "2026-05-05T21:36:08Z"
generating: "2026-05-05T21:37:20Z"
prompted: "2026-05-05T21:41:23Z"
verifying: "2026-05-05T21:53:52Z"
completed: "2026-05-05T22:23:00Z"
branch: dark-factory/bug-autoreview-path-unreachable-when-automerge-takes-precedence
---

# autoReview path is unreachable: `autoMerge` takes precedence in `handleAfterIsolatedCommit`, and `WaitAndMerge` switches on the wrong field

## Summary

When the operator configures `autoReview: true`, config validation requires `autoMerge: true` (`pkg/config/config.go:469`). But the workflow code in `handleAfterIsolatedCommit` checks `if deps.AutoMerge` first and returns; the `if deps.AutoReview` branch that sets `prompt.InReviewPromptStatus` is dead code. Result: prompts never transition to `in_review`, the `ReviewPoller` (and its now-fixed `handleApproved` → `PostMergeActions` from spec 071) never runs.

A second bug compounds it: `prMerger.WaitAndMerge` queries `gh pr view --json mergeStateStatus` but switches on `"MERGEABLE"`/`"CONFLICTING"` — those are values of the `mergeable` field, not `mergeStateStatus`. So when GitHub returns `mergeStateStatus: "CLEAN"` (the success state), the switch falls into the `default` branch and polls forever. The daemon hangs silently after `created PR`.

## Reproduction

dark-factory version: HEAD at v0.150.1.

1. Sandbox repo with valid GitHub remote and an open PR-eligible branch.
2. `.dark-factory.yaml`:
   ```yaml
   workflow: worktree
   pr: true
   autoMerge: true
   autoRelease: true
   autoReview: true
   allowedReviewers:
     - bborbe
   pollIntervalSec: 15
   ```
3. Drop a draft prompt that produces a CHANGELOG entry. Approve and run daemon.
4. Container completes; daemon opens PR. Daemon log:
   ```
   level=INFO msg="created PR" url=https://github.com/.../pull/N
   ```
5. Open the PR on GitHub. `gh pr view N --json mergeStateStatus,mergeable,reviewDecision,autoMergeRequest`:
   ```json
   {"autoMergeRequest":null,"mergeStateStatus":"CLEAN","mergeable":"MERGEABLE","state":"OPEN","reviewDecision":"APPROVED"}
   ```
6. Daemon log shows NO further activity. Process is alive but silent. No "PR created, waiting for review" line. No poller activity.
7. Inspect the prompt file in `prompts/in-progress/`:
   ```yaml
   ---
   status: approved
   ---
   ```
   `status` is still `approved`, not `in_review`. The poller's `listInReview` returns nothing.

## Expected vs Actual

**Expected:** with `autoReview: true + autoMerge: true`, the daemon transitions the prompt to `in_review`, the poller waits for human approval, then upon approval calls `handleApproved` (→ `WaitAndMerge` → `MoveToCompleted` → `PostMergeActions` per spec 071's fix).

**Actual:** the daemon falls into `handleAutoMergeForClone` (which calls `WaitAndMerge` directly) and hangs forever in `WaitAndMerge` because it only matches `mergeStateStatus == "MERGEABLE"`, while GitHub returns `"CLEAN"`. Spec 071's fix never runs because the autoReview poller branch never sees the prompt.

## Code pointers

- `pkg/processor/workflow_helpers.go:264-285` — `handleAfterIsolatedCommit`: `if deps.AutoMerge` block returns BEFORE `if deps.AutoReview` block. With validation forcing both true, the autoReview branch is dead code.
- `pkg/config/config.go:461-475` — `validateAutoReview`: requires `AutoMerge` when `AutoReview` is set.
- `pkg/git/pr_merger.go:48-79` — `WaitAndMerge`: queries `--json mergeStateStatus` but switches on `"MERGEABLE"`/`"CONFLICTING"` (those are values of `mergeable` field, not `mergeStateStatus`). `mergeStateStatus` legitimate values include `CLEAN`, `BEHIND`, `BLOCKED`, `DIRTY`, `HAS_HOOKS`, `UNKNOWN`, `UNSTABLE`. The success case `CLEAN` falls into `default` → infinite poll.
- `pkg/review/poller.go:127-148` — `listInReview` is correct; it returns nothing because no prompt ever has `status: in_review`.

## Why this is a bug

Spec 071 fixed `handleApproved` to call `PostMergeActions`. That fix is structurally correct (unit-tested) but unreachable end-to-end because the prompt never reaches `in_review` status. Two related defects combine:

1. **Routing bug** — `if deps.AutoMerge` short-circuits before `if deps.AutoReview`. The intended flow ("opens PR, waits for human review, merges after approval") never runs.
2. **WaitAndMerge field mismatch** — even when the autoMerge path IS the right path (e.g. autoReview off), `mergeStateStatus: "CLEAN"` (the universally good state) doesn't match `"MERGEABLE"`. The daemon hangs.

Both symptoms manifest after PR creation: no log, no merge, no tag, no completion.

## Workaround

None automated. Operator can:
- Manually merge the PR via `gh pr merge --merge --delete-branch <url>` and manually run release ceremony.
- Set `autoReview: false` and rely on autoMerge alone (but then WaitAndMerge mismatch still bites).
- Set `autoMerge: false + autoReview: false` and use `pr: true` with manual merge.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `autoReview: true + autoMerge: true` (only valid combo) | Prompt transitions to `in_review`; poller handles approval | Daemon picks up approved review and runs `PostMergeActions` (spec 071) |
| `autoMerge: true + autoReview: false`, PR returns `mergeStateStatus: "CLEAN"` | `WaitAndMerge` calls `mergePR` and merges | Match `CLEAN` (and likely `HAS_HOOKS`, `UNSTABLE` per policy) as merge-ready states |
| `autoMerge: true + autoReview: false`, PR returns `mergeStateStatus: "BLOCKED"` | Wait until unblocked or timeout | Existing polling, just for the right field |
| `autoMerge: true + autoReview: false`, PR returns `mergeStateStatus: "DIRTY"` (conflicts) | Fail fast | Recognize conflict states explicitly |

## Acceptance Criteria

- [ ] With `autoReview: true + autoMerge: true`, after PR creation the prompt's frontmatter status is `in_review` (not `approved`).
- [ ] With `autoReview: true + autoMerge: true`, the daemon log emits `PR created, waiting for review` (or equivalent) and the poller polls the PR's review state.
- [ ] When a reviewer approves the PR, `handleApproved` runs (`merged PR` log), `MoveToCompleted` runs, `PostMergeActions` runs (`merged PR and updated default branch`, `created tag`, `pushed tag` log lines per spec 071).
- [ ] `WaitAndMerge` recognizes `mergeStateStatus: "CLEAN"` as merge-ready (matches `gh pr view`'s actual return values, not `mergeable` enum values).
- [ ] `WaitAndMerge` recognizes `mergeStateStatus: "DIRTY"` (conflict) as terminal failure.
- [ ] `WaitAndMerge` continues polling on `BLOCKED`, `BEHIND`, `UNKNOWN` (existing behavior preserved on those states).
- [ ] CHANGELOG.md `## Unreleased` entry added.

## Constraints

- Do NOT remove `validateAutoReview`'s `AutoMerge` requirement — the intended semantic is "autoReview means: open PR, wait for human review, then auto-merge". Both flags work together.
- Do NOT regress the autoMerge-only path (`autoReview: false`).
- Do NOT change the `Releaser`/`Brancher` injection added by spec 071 — that wiring is correct.

## Verification

Per `docs/bug-workflow.md` §Verification, this is a runtime symptom — unit tests alone are not sufficient.

**Repro replay (must run after fix lands):**

```bash
# In a sandbox repo with autoReview config above and a CHANGELOG.md:
cd ~/Documents/workspaces/dark-factory-sandbox

# Drop a small prompt that produces a CHANGELOG entry
dark-factory prompt approve <name>
dark-factory daemon &
DAEMON_PID=$!

# Expected within seconds:
#   level=INFO msg="created PR" url=...
#   level=INFO msg="PR created, waiting for review" url=...

# Inspect prompt status
grep "^status:" prompts/in-progress/*.md   # must show "in_review"

# Have a second account approve the PR (PR author cannot self-approve)
# Wait one pollIntervalSec

# Expected within ~pollIntervalSec seconds of approval:
#   level=INFO msg="merged PR" url=...
#   level=INFO msg="merged PR and updated default branch" branch=master
#   level=INFO msg="created tag" tag=vX.Y.Z
#   level=INFO msg="pushed tag"

git fetch --tags
git tag -l | tail -3
grep -A2 "^## v" CHANGELOG.md | head -5

kill $DAEMON_PID
```

**Acceptable evidence for `verifying → completed`:**

| Evidence | Acceptable? |
|----------|-------------|
| Daemon log shows `PR created, waiting for review` after PR creation | Yes |
| Prompt frontmatter shows `status: in_review` after PR creation | Yes |
| Daemon log shows `created tag` after autoReview approval | Yes |
| Unit test asserting routing prefers autoReview | Necessary but not sufficient |
| "All tests pass" without runtime replay | No |

## Non-goals

- Changing the autoReview semantic (still requires autoMerge as the merge mechanism after review).
- Refactoring `WaitAndMerge` into a separate `pkg/git/merge_state.go` — narrow fix only.
- Bitbucket Server `WaitAndMerge` — file separately if it has the same field-mismatch issue.

## See also

- Spec 071 (`bug-autoreview-skips-postmerge-actions-no-tag-no-release`) — fixed `handleApproved` to call `PostMergeActions`. This bug spec ensures the fix is actually reachable.
- `pkg/processor/workflow_helpers.go:264-285` — routing.
- `pkg/git/pr_merger.go:48-79` — WaitAndMerge field mismatch.
- `pkg/config/config.go:461-475` — validation rule.
- GitHub GraphQL `mergeStateStatus` enum: https://docs.github.com/en/graphql/reference/enums#mergestatestatus
