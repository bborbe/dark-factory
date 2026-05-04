---
status: generating
approved: "2026-05-04T20:14:01Z"
generating: "2026-05-04T20:14:01Z"
branch: dark-factory/bug-branch-workflow-checkout-fails-on-divergent-prompt-file
---

# `branch` workflow `git checkout` fails when prompt file content differs between master and the existing feature branch

## Summary

After spec 066 shipped (`Brancher.IsClean` ignores changes inside dark-factory's own prompt directories), the `branch` workflow advances past the cleanliness gate. It then crashes at the very next git step — `Brancher.Switch` (`pkg/git/brancher.go:111`) — because `git checkout <existing-feature-branch>` refuses when the working tree has uncommitted changes to a path whose target-branch content differs from the master content. The prompt file (modified by daemon bookkeeping writes that 066 deliberately permits in the master tree) is exactly such a path: master has it as `status: failed/queued`, the feature branch has it as `status: success/in_review` from a prior run.

Net effect: any retry against an already-existing feature branch fails. Same shape of bug as 066, different git command, different filter site. 066's fix was scoped to `IsClean` and explicitly did not touch other git operations.

## Reproduction

dark-factory version: built from master at `pkg/git/brancher.go` HEAD (`v0.148.3` ships post-066 fix).

1. Project: `~/Documents/workspaces/jira-task-creator` with `workflow: branch + pr: true`.
2. Run a prompt to completion once. Daemon creates feature branch `dark-factory/<name>`, commits, pushes, opens PR. Branch exists locally and remotely.
3. The prompt completes (success or failure — either way the prompt file's frontmatter is rewritten to a status that differs from what the feature branch saw at commit time).
4. Now `dark-factory prompt retry <name>` (or any path that re-queues the same prompt). Master's `prompts/in-progress/<NNN>-...md` is modified.
5. Start daemon. `setupInPlaceBranch`:
   - `IsCleanIgnoring(ctx, [prompts/...])` returns `(nil, nil)` — clean ✅ (066's fix)
   - `FetchAndVerifyBranch(ctx, branch)` succeeds — branch exists ✅
   - `Brancher.Switch(ctx, branch)` ❌ FAILS with `exit status 1`
6. Daemon log:

   ```
   level=ERROR msg="prompt failed" file=009-test-loglevel-handler.md error="exit status 1
   switch to branch
       (*brancher).Switch ... pkg/git/brancher.go:111
   switch to existing branch
       (*branchWorkflowExecutor).setupInPlaceBranch ... pkg/processor/workflow_executor_branch.go:73
   setup workflow
       ...
   "
   ```

7. The actual git error (not surfaced — `Brancher.Switch` swallows stderr): `error: Your local changes to the following files would be overwritten by checkout: prompts/in-progress/NNN-...md` or similar.

## Expected vs Actual

**Expected** (per `docs/workflows.md` `branch` row + spec 066's promise that "branch workflow retry succeeds without manual intervention"):
> After 066, daemon Setup advances past the cleanliness gate when only dark-factory bookkeeping paths are dirty, switches to the feature branch (creating it or reusing it), and proceeds to run the YOLO container. Existing-branch reuse is the documented happy path for retries.

**Actual:**
- 066 fixed `IsClean` only.
- `Brancher.Switch` calls `git switch <branch>` (or `git checkout <branch>`) with no filter, no stash, no force flag. Git's own conflict-detection refuses the switch when the working tree has uncommitted changes to a path that would change content on the target branch.
- The prompt file is exactly such a path on every retry, by design — daemon writes status to the master copy; the feature branch holds an older snapshot of the same file.
- Setup fails. Prompt cannot make progress without the user manually committing the master state (which 066 was supposed to make unnecessary) OR deleting the local feature branch (which forces a fresh creation but loses the prior feature commits).

## Why this is a bug

Spec 066's stated goal: "branch workflow retry succeeds without intervening commits, on the first daemon cycle, no human-in-the-loop." That goal is unmet. The fix shape chosen for 066 (filter `IsClean`) addressed the cleanliness CHECK but not the cleanliness CONSEQUENCE — git checkout still cares.

This is the same root cause as 066 (dark-factory's own prompt-file writes interacting badly with git assumptions in the parent repo), but in a different code path. Reviewers and verifier of 066 didn't catch it because the runtime replay was never performed at the time of approval — and when this bug triggers, it does so on the *next* setup, not the first. (The first setup creates the feature branch; the bug surfaces on retry.)

By the verification rules of `docs/bug-workflow.md`, 066 is technically not yet `completed` — it's in `verifying`. So this is the bug the runtime replay would have caught.

## Workaround

After every retry that targets an already-existing feature branch:

```bash
# Either:
git branch -D dark-factory/<prompt-name>
git push origin --delete dark-factory/<prompt-name>   # if remote also needs reset
# now retry — daemon creates fresh branch from default, succeeds

# OR:
git add prompts/in-progress/<NNN>-...md
git commit -m "retry: dark-factory bookkeeping"
# now retry — Switch succeeds because the local change is committed
```

Either workaround defeats the point of automated retry.

## Code pointers

- `pkg/git/brancher.go:~109-115` — `Switch` method. Currently runs plain `git switch <branch>` (or equivalent). No stash, no force, no filter.
- `pkg/processor/workflow_executor_branch.go:73` — caller. The "switch to existing branch" branch of `setupInPlaceBranch`'s if/else.
- `pkg/processor/workflow_executor_branch.go:~67-70` — the alternative `CreateAndSwitch` path runs when `FetchAndVerifyBranch` errors (branch doesn't exist). That path probably *also* has a checkout step internally, but it works because the target branch is being created from the current HEAD — no content divergence.
- Spec 066 (`bug-branch-workflow-rejects-its-own-uncommitted-prompt-file-write`) — the prior fix that made this bug observable. 066's option-C (filter `IsClean`) was explicitly chosen over option-A (commit-on-write) and option-B (stash-around-Setup). Both A and B would have prevented this bug. Triage of this spec must reconsider whether C alone is the right shape.
- 066's "Constraints" section forbids option D (sidecar state dir) but does NOT forbid stashing or committing — those were rejected as fix-shapes for 066 only, not as principles. Re-evaluating them for this bug is in scope.

## Failure Modes

| Trigger | Expected behavior | Recovery / verification |
|---------|-------------------|--------------------------|
| Retry against existing feature branch with dirty prompt file in master | Setup advances through `Switch` to the feature branch; prompt file's master-side modification is preserved (committed on the feature branch alongside the agent's work) OR safely stashed before checkout and discarded after | Daemon log shows "switched to branch for in-place execution"; no "exit status 1" |
| Retry against existing feature branch with NO dirty prompt file (clean master) | Existing happy path — Setup switches cleanly | No regression |
| First-time creation of feature branch (branch does not exist) | `CreateAndSwitch` continues to work as today | No regression |
| Master tree dirty with a non-prompt-dir file (e.g. `pkg/foo/bar.go`) | `IsCleanIgnoring` aborts Setup with the offending path named (066's negative-control behavior) | Existing 066 negative-control still passes |
| Daemon killed mid-Switch; prompt frontmatter in intermediate state; daemon restarted | Resume picks up cleanly; same retry path lands without manual intervention | No new regression vs 066's resume scenario |
| Two different prompts queued back-to-back, each targeting a pre-existing branch | Both retry paths advance through Setup without manual commits between them | Daemon log shows two `switched to branch` events |

## Goal

After this fix, the goal claimed by spec 066 is actually achieved: `dark-factory prompt retry` followed by `dark-factory daemon` advances past Setup on the first cycle when the only working-tree dirt is in dark-factory's own state directories — even when the target feature branch already exists and has divergent content for the prompt file's path.

## Constraints

- Do NOT regress 066's `IsCleanIgnoring` filter — it remains correct and necessary. This fix is downstream of it.
- Do NOT regress `worktree`, `clone`, or `direct` workflows — they don't go through `Brancher.Switch` for in-place branch reuse, so they should be unaffected, but verify.
- Do NOT introduce `git checkout -f` (force discard) — that would clobber legitimate user-source dirt in non-prompt directories. The negative-control from 066 must continue to fail-fast on user-source dirt.
- Do NOT reuse 066's `IgnorePathPrefixes` field if it doesn't make semantic sense for the chosen fix shape (e.g., stash-around-checkout doesn't need the prefix list).
- Do NOT introduce a sidecar `.dark-factory/state/` (option D from 066) — still deferred to a future spec.
- Reuse the existing `Brancher`/`prompt.PromptFile` interfaces; do not introduce a parallel git-aware abstraction.
- The fix must be safe under daemon-killed-mid-operation: any stash/commit it creates must be recoverable on restart.
- The fix must be idempotent: running Setup twice in a row on the same branch must produce the same end state.

## Fix shape (resolved)

**Decision: Option C — discard the master-side bookkeeping dirt before `Switch`, then let the existing `pf.Save` write the runtime state onto the feature branch.**

In `setupInPlaceBranch` (`pkg/processor/workflow_executor_branch.go:51`), immediately before the `Brancher.Switch` call (line 67-73), discard any uncommitted changes inside dark-factory's own state directories on the current branch:

```
git checkout HEAD -- <ignorePathPrefixes...>
```

This is scoped to the same prefix list 066 uses for `IsCleanIgnoring`, so user-source paths are untouched. After the discard, the working tree is clean for the prompt-dir paths and `Switch` proceeds without conflict. The retry's runtime state (status, retry count, etc.) is held in memory in the `*prompt.PromptFile` already loaded by the processor; it survives the on-disk discard. After Setup returns, `processor.ProcessPrompt` calls `pf.PrepareForExecution(...)` + `pf.Save(ctx)` at `processor.go:329-330`, which writes the runtime state onto the feature branch — exactly where it belongs (the feature branch is the authoritative location for that prompt's execution metadata, master never needed it).

**Why C** (over A/B/D):

- **A (commit-before-Switch)**: rejected. Adds one extra `bookkeeping` commit per retry to the feature-branch history. Pollutes the diff a reviewer eventually sees on the PR. Cleanup requires interactive rebase, defeating the autoReview path's premise of a clean review surface.
- **B (stash-around-Switch)**: rejected. Stash-management complexity (collisions, partial pops, recovery on daemon kill mid-stash) was the same objection that ruled it out of 066's triage. Reapplying that ruling here.
- **C (discard-before-Switch + rely on `pf.Save`)**: chosen. Smallest blast radius. Reuses 066's prefix-list (no new config). The "re-apply on the feature branch" step is **not new code** — it's already the `pf.Save` call that runs after Setup. We just have to not crash before reaching it.
- **D (auto-resync from feature branch)**: rejected. Would clobber the retry's runtime intent (status: queued/approved set by `prompt retry`) with stale feature-branch content. Wrong direction.

**Implementation outline** (binding for fix prompts):

1. Add a new method to `Brancher` (or reuse an existing one if it fits): `DiscardUncommittedInPaths(ctx, prefixes []string) error`. Implementation: `git checkout HEAD -- <each-prefix>` (one call per prefix; or one call with multiple pathspecs).
2. In `setupInPlaceBranch`, after `IsCleanIgnoring` returns clean and BEFORE `Brancher.Switch`, call `DiscardUncommittedInPaths(ctx, e.deps.IgnorePathPrefixes)`. The same `IgnorePathPrefixes` field 066 added to `WorkflowDeps` is reused.
3. Boundary: if `IgnorePathPrefixes` is empty (e.g. dark-factory layout unknown), the discard is a no-op — Switch may still fail, but only on configurations that didn't get the 066 fix anyway.
4. Negative-control test: a dirty file in a non-dark-factory path (e.g. `pkg/foo/bar.go`) is NOT discarded — IsCleanIgnoring already catches it before Switch, so the discard never runs. Verify the test pins this ordering.
5. Daemon-kill safety: discarding bookkeeping dirt then crashing leaves the feature branch in its pre-retry state. The next daemon cycle re-runs the retry path and re-writes the bookkeeping. Idempotent.

### Alternatives considered (not selected — for future reference)

- **A (commit-before-Switch)**: rejected for history pollution; tracked only as a fallback if C produces unforeseen issues.
- **B (stash-around-Switch)**: rejected for crash-safety complexity. Same objection as 066.
- **D (auto-resync from feature branch)**: rejected for clobbering retry intent.

## Acceptance Criteria

- [ ] `Brancher` exposes a method (new or extended) that discards uncommitted changes restricted to a given list of path prefixes; the method is invoked from `setupInPlaceBranch` before `Brancher.Switch`.
- [ ] Replaying the reproduction (retry against a pre-existing feature branch with divergent prompt file) advances past Setup and produces an open PR end-to-end. Verified at runtime in `~/Documents/workspaces/jira-task-creator`, not via unit tests alone.
- [ ] Negative-control: with a dirty file in a non-prompt directory (e.g. `pkg/handler/list-sprints.go`), Setup aborts with the offending path named — `IsCleanIgnoring` from 066 still gates first, the discard never runs.
- [ ] First-run case (feature branch does NOT exist locally) continues to work: `CreateAndSwitch` path is unaffected.
- [ ] No regression in `worktree`, `clone`, or `direct` workflows. They do not go through `Brancher.Switch` for in-place reuse, so the new discard is unreachable from those paths.
- [ ] Unit test asserts `DiscardUncommittedInPaths` (or the equivalent renamed method) skips paths not in the prefix list.
- [ ] `docs/workflows.md` `branch`-row commentary documents that dark-factory discards its own prompt-dir bookkeeping dirt before checkout (one-line addition next to the 066-era IsCleanIgnoring note).
- [ ] Spec 066's `verifying → completed` transition is unblocked once this spec lands; verifier can run the 066 runtime replay against the rebuilt binary and produce the expected log lines.

## Verification

Per `docs/bug-workflow.md` §Verification, this is a runtime symptom — unit tests alone are not sufficient.

**Repro replay (must run after fix lands):**

```bash
# In jira-task-creator with workflow: branch + pr: true:
cd ~/Documents/workspaces/jira-task-creator

# Setup: ensure a feature branch exists from a prior run
# (any prompt that has already been processed once and has dark-factory/<name> branch locally)
git branch -a | grep dark-factory   # should show at least one feature branch

# Force the failure precondition
dark-factory prompt retry           # writes status to prompt file in master
git status --short                  # one M line for prompt file

# Run daemon — must NOT fail with "exit status 1" at Switch
dark-factory daemon &
DAEMON_PID=$!

# Watch log; expected (in order):
#   "found queued prompt"
#   "preflight: baseline check passed"
#   "executing prompt"
#   "syncing with remote default branch"
#   "switched to branch for in-place execution" branch=dark-factory/<name>
# MUST NOT see:
#   "switch to existing branch ... exit status 1"

# Cleanup
kill $DAEMON_PID
```

**Negative-control replays:**

1. Same setup, but with a real source-file change AND the prompt file dirty. Expected: Setup aborts on the source file (066's behavior preserved), not the prompt file.
2. First-run case: feature branch does NOT exist locally. Expected: `CreateAndSwitch` path runs as today, no regression.
3. Prompt that completed cleanly the first time, then a different unrelated prompt is retried. Expected: only the dirty prompt's setup is affected; other prompts not in scope.

**Acceptable evidence for `verifying → completed`:**

| Evidence | Acceptable? |
|----------|-------------|
| Daemon log shows `switched to branch` after retry on a pre-existing feature branch | Yes |
| Negative-control shows source-file dirt still aborts Setup | Yes |
| Unit test asserting the chosen fix-shape's contract (e.g. stash created and popped, or reset+reapply, or pre-switch commit) | Necessary but not sufficient |
| "All tests pass" without runtime replay | No |

## Open Questions

1. ~~Which fix shape (A/B/C/D)?~~ **Resolved: option C** (see "Fix shape (resolved)" section above).
2. Does this bug also affect `Brancher.CreateAndSwitch` (the "branch doesn't exist yet" path)? CreateAndSwitch creates the new branch FROM the current HEAD, so the working tree's modifications carry over and there's no divergence to refuse. Probably safe, but verify during fix-prompt generation.
3. Does this bug affect `worktree` workflow? `git worktree add` is also a checkout under the hood. The worktree is a different directory, so the master-side dirt doesn't directly conflict — but verify.
4. Is the right longer-term fix actually option D from 066 (sidecar state dir)? This is the second bug from the same architectural root. If a third surfaces, that's a strong signal the filter approach is patching symptoms. Track but defer.

## See also

- Spec 066 (`bug-branch-workflow-rejects-its-own-uncommitted-prompt-file-write`) — the prior fix that made this bug observable. Same architectural family.
- Spec 065 (`bug-pr-create-missing-head-flag-in-isolated-workflows`) — sibling bug; verification of 065 AC 4 (branch end-to-end) was the path that surfaced this.
- Spec 063 (`bug-autorelease-overrides-pr-workflow`) — earlier dispatch-path bug.
- `pkg/git/brancher.go:109-115` — `Switch` method, the chokepoint.
- `pkg/processor/workflow_executor_branch.go:73` — the call site where the failure manifests.
- `docs/workflows.md` `branch` row — the contract this bug violates.
