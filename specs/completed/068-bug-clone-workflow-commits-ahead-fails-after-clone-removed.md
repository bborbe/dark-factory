---
status: completed
approved: "2026-05-04T21:25:10Z"
generating: "2026-05-04T21:25:23Z"
prompted: "2026-05-04T21:32:08Z"
verifying: "2026-05-05T11:36:08Z"
completed: "2026-05-05T12:10:09Z"
branch: dark-factory/bug-clone-workflow-commits-ahead-fails-after-clone-removed
---

# `clone` workflow `handleAfterIsolatedCommit` runs `CommitsAhead` against the parent repo after the clone is deleted, fails with `git exit 128`

## Summary

The `clone` workflow's Complete path commits inside the clone, chdirs back to the original repo, deletes the clone, and *then* calls `handleAfterIsolatedCommit` from the original repo's directory. `handleAfterIsolatedCommit` immediately calls `Brancher.CommitsAhead(branch)` — but the feature branch only existed inside the clone, which is now gone. The original repo has never seen the branch (no fetch happened, no push happened yet — `CommitsAhead` runs *before* push in the post-commit pipeline), so `git rev-list <default>..<branch>` exits 128 with `unknown revision`.

The agent's work succeeds end-to-end: file edits land, `make precommit` passes, the container exits 0, the YOLO report is `success`. But dark-factory then crashes at the post-commit step. No PR is opened, the prompt is marked `failed`, and the just-completed work is effectively lost (the clone is already deleted).

## Reproduction

dark-factory version: built from master at `pkg/processor/workflow_executor_clone.go` HEAD (`v0.148.4` ships this code).

1. Sandbox: `~/Documents/workspaces/dark-factory-sandbox` (real GitHub remote, throwaway repo).
2. Setup per `scenarios/002-workflow-pr.md`:

   ```bash
   go build -C ~/Documents/workspaces/dark-factory -o /tmp/dark-factory-v0.148 .
   WORK_DIR=$(mktemp -d)
   cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
   cd "$WORK_DIR/dark-factory-sandbox"
   cat > .dark-factory.yaml << 'YAML'
   workflow: pr
   maxContainers: 999
   YAML
   ```

3. Drop a tiny prompt that toggles a marker line in `math_abs.go` (per scenario 002).
4. `/tmp/dark-factory-v0.148 prompt approve toggle-comment`
5. `/tmp/dark-factory-v0.148 run`

6. Observe daemon log:
   ```
   level=INFO msg="cloning repo" src=...sandbox dest=.../dark-factory/dark-factory-sandbox-002-toggle-comment branch=dark-factory/002-toggle-comment
   level=INFO msg="docker container exited" exitCode=0
   level=INFO msg="completion report" status=success summary="Removed the dark-factory-sandbox scenario test marker line..."
   level=ERROR msg="prompt failed" file=002-toggle-comment.md error="exit status 128
       count commits ahead
       (*brancher).CommitsAhead at brancher.go:385
       handleAfterIsolatedCommit at workflow_helpers.go:224
       (*cloneWorkflowExecutor).Complete at workflow_executor_clone.go:117"
   ```

7. State after failure:
   - Original sandbox: clean. No new branch, no new commit. Master untouched.
   - Clone directory `$TMPDIR/dark-factory/dark-factory-sandbox-002-toggle-comment`: deleted (by `Cloner.Remove` at `workflow_executor_clone.go:112`).
   - `git ls-remote origin 'refs/heads/dark-factory/*'`: no `dark-factory/002-toggle-comment` ref. Push never happened.
   - The agent's edits are lost (only existed in the deleted clone).

## Expected vs Actual

**Expected** (per `docs/workflows.md` `clone` row + the `scenarios/002-workflow-pr.md` spec):

> "Validates that a prompt creates a clone, executes in isolation, pushes a feature branch, and opens a PR. ... Feature branch `dark-factory/*` pushed. PR opened on GitHub. Clone removed after completion."

The push step must precede clone removal so the feature branch survives, and the post-commit pipeline (`CommitsAhead → Push → PR-create`) must run from a directory that can resolve the branch.

**Actual:**

`cloneWorkflowExecutor.Complete` (`pkg/processor/workflow_executor_clone.go:99-127`) executes in this order:

1. `Releaser.CommitOnly(gitCtx, title)` — commit lands inside the clone (line 104)
2. `os.Chdir(e.originalDir)` — back to the parent repo (line 108)
3. `Cloner.Remove(gitCtx, e.clonePath)` — clone DELETED (line 112)
4. `handleAfterIsolatedCommit(...)` — invoked from parent repo cwd (line 117)
5. `handleAfterIsolatedCommit` calls `Brancher.CommitsAhead(branch)` at `pkg/processor/workflow_helpers.go:224` — fails with exit 128 because the parent repo has never heard of the feature branch.

The push step (`Brancher.Push(branch)` at `workflow_helpers.go:232`) is *after* `CommitsAhead` in the pipeline, so even if `CommitsAhead` were skipped, push would also fail (the parent repo has no commits on that branch to push — they only existed in the deleted clone).

The whole post-commit pipeline of `handleAfterIsolatedCommit` was written assuming worktree-style sharing of `.git/` (where the parent repo's branch list includes the worktree's feature branch). For clone, the assumption is wrong.

## Why this is a bug

`docs/workflows.md` documents `clone` as a first-class workflow alongside `worktree` and `branch`. `scenarios/002-workflow-pr.md` is in `status: active`. The release-process.md surface-scope table lists this scenario as the gate for changes to `pkg/git/`, prompt-execution flow, and `--auto-approve`. None of those work today for the `clone` workflow — the documented happy path is unreachable because the executor and `handleAfterIsolatedCommit` make incompatible assumptions about which directory holds the branch.

This is the fifth bug in the same architectural family (063, 065, 066, 067, and now this one): asymmetric behavior across workflow executors where one path was tested and others were not. The pattern is "borrowed code from worktree path, didn't account for clone-specific differences."

## Workaround

There is no operator-side workaround. Until this is fixed, `workflow: clone` (or its legacy alias `workflow: pr`) cannot complete a single prompt end-to-end. Use `workflow: branch` or `workflow: worktree` instead — both are verified working as of v0.148.4.

## Code pointers

- `pkg/processor/workflow_executor_clone.go:99-127` — `Complete`. The mis-ordering: chdir+remove BEFORE handleAfterIsolatedCommit.
- `pkg/processor/workflow_executor_worktree.go:99-127` — `Complete` for worktree. Same shape, but works because worktrees share `.git/` with the parent. Compare to understand the asymmetry.
- `pkg/processor/workflow_helpers.go:213-268` — `handleAfterIsolatedCommit`. Lines 224 (`CommitsAhead`), 232 (`Push`), and downstream `findOrCreatePR` all assume the branch is locally visible in cwd's `.git/`.
- `pkg/git/brancher.go:~380-390` — `CommitsAhead` runs `git rev-list <default>..<branch>`. Exit 128 when the branch ref is missing locally.
- `scenarios/002-workflow-pr.md` — the scenario that surfaces this bug. Currently marked `status: active` but cannot pass.

## Failure Modes

| Trigger | Expected behavior | Recovery / verification |
|---------|-------------------|--------------------------|
| Prompt completes successfully in clone (any non-zero diff) | Feature branch pushed to origin, PR opened, clone removed, prompt moved to `completed/` | Daemon log shows `created PR url=...`; `gh pr list` shows the PR |
| Prompt completes with no-diff success (372 path) | Clone removed, prompt moved to `completed/`, no push/PR (current `ahead == 0` skip in handleAfterIsolatedCommit applies) | Daemon log shows `no staged changes — skipping commit` and `no new commits on branch — skipping push/PR` |
| Container fails (exitCode != 0) | Existing CleanupOnError path removes the clone; prompt marked `failed` with container error, not a post-commit error | Existing scenario coverage |
| Operator runs scenario 002 on `~/Documents/workspaces/dark-factory-sandbox` | Scenario passes its checklist end-to-end (PR opened, clone removed, master clean) | The runtime replay required for verification — current behavior fails at `handleAfterIsolatedCommit:224` |
| `workflow: pr` legacy alias (deprecated) | Same successful behavior as `workflow: clone + pr: true` (already mapped at config load) | Existing deprecation warning still emitted; no regression in mapping |
| `clone + pr: false` (push without PR) | Branch pushed, no PR, prompt moved to `completed/` | Currently blocked by the same `CommitsAhead` failure |

## Goal

After this fix, `workflow: clone` (and its legacy alias `workflow: pr`) completes a prompt end-to-end: feature branch pushed to origin, PR opened on GitHub (when `pr: true`), clone removed, prompt moved to `completed/`, with no `exit 128` in the daemon log. Scenario 002 passes its full checklist.

## Constraints

- Do NOT regress `worktree` or `branch` workflow behavior. Both are verified working at v0.148.4 and must remain so.
- Do NOT delete or weaken `handleAfterIsolatedCommit`'s `ahead == 0` guard at `workflow_helpers.go:228` — it's load-bearing for the no-diff case (372).
- Do NOT introduce a new "clone-aware" branch in `handleAfterIsolatedCommit` if a smaller-blast-radius fix is possible inside `cloneWorkflowExecutor.Complete`.
- Do NOT skip the push step. The end state must include the feature branch on origin (verified by `git ls-remote origin`).
- Do NOT keep the clone alive after the prompt succeeds (the existing `Cloner.Remove` cleanup is correct in spirit; only the timing relative to the post-commit pipeline is wrong).
- Reuse existing `Brancher` / `Cloner` / `Releaser` interfaces. Do not introduce a parallel "clone post-commit" pipeline if the existing one can be reordered.
- Do NOT use options B (full pipeline inside clone) or D (parameterize handleAfterIsolatedCommit). The chosen shape is A+C: push from inside the clone, then fetch the just-pushed branch in handleAfterIsolatedCommit before CommitsAhead.
- Daemon-killed-mid-Complete safety: if the daemon dies between commit-in-clone and push, the next start-up must NOT think the prompt succeeded. Either the work is lost (acceptable — same as today's failure mode) or it's recoverable from clone state preservation (preferable but not required).

## Fix shape (resolved)

**Decision: combination of A + C — push from inside the clone before chdir/remove, then fetch the just-pushed branch in `handleAfterIsolatedCommit` before `CommitsAhead`.**

Why this combination:

- Pure shape A is insufficient on its own. After the executor pushes from inside the clone, the parent repo (where `handleAfterIsolatedCommit` runs) still has no local ref for the feature branch. `CommitsAhead` runs `git rev-list --count origin/<default>..<branch>` — `<branch>` resolves locally, and there's nothing local to resolve. Pure A would fix the push step but `CommitsAhead` would still fail with `unknown revision`.
- Adding a fetch step in `handleAfterIsolatedCommit` (shape C's contribution) brings `<branch>` into the parent repo's local view as `refs/remotes/origin/<branch>`. The downstream pipeline then resolves it correctly.
- Shape B (run the entire pipeline inside the clone) duplicates code with worktree's path and forces `handleAfterIsolatedCommit` to become worktree-specific. Rejected for blast radius.
- Shape D (parameterize `handleAfterIsolatedCommit` with "where is the branch") is the cleanest in principle but requires the largest interface change. Tracked as a possible future cleanup, not landed here.

**Implementation outline** (binding for fix prompts):

1. In `cloneWorkflowExecutor.Complete` (`pkg/processor/workflow_executor_clone.go:104-117`), after `Releaser.CommitOnly` and BEFORE `os.Chdir(originalDir)`, call `e.deps.Brancher.Push(gitCtx, e.branchName)`. This pushes from inside the clone where the branch is locally visible.
2. In `handleAfterIsolatedCommit` (`pkg/processor/workflow_helpers.go:213`), immediately before the `Brancher.CommitsAhead(branchName)` call (line 224), call `e.deps.Brancher.FetchBranch(gitCtx, branchName)` (or extend an existing helper). The fetch is scoped to the named branch — fast, idempotent, and a no-op if the branch is already in sync.
3. The existing `Brancher.Push` call inside `handleAfterIsolatedCommit` (line 232) becomes a no-op for the clone path (already pushed) but is harmless: `git push` on an already-pushed branch exits 0 with "Everything up-to-date". Worktree's path is unchanged because worktree shares `.git/` and the executor doesn't push.
4. For the no-diff case (spec 372), `Releaser.CommitOnly` no-ops, the new Push call sees `git push` with no commits and exits 0 silently (or skips via `ahead == 0` check earlier), and the existing `ahead == 0` skip in `handleAfterIsolatedCommit:228` continues to apply for the no-PR-needed case.

**New helper required:** `Brancher.FetchBranch(ctx, branch string) error` — runs `git fetch origin <branch>:refs/remotes/origin/<branch>` (or equivalent). If the helper already exists under another name, reuse it.

### Alternatives considered (not selected — for future reference)

- **B (full pipeline inside clone)**: rejected for code duplication with worktree path.
- **D (parameterize handleAfterIsolatedCommit)**: rejected for this spec — interface-change blast radius. Tracked as possible follow-up if more workflow-asymmetry bugs surface.

## Acceptance Criteria

- [ ] `cloneWorkflowExecutor.Complete` (or the post-commit pipeline it calls) pushes the feature branch to origin BEFORE the clone is removed, so the branch is reachable when `handleAfterIsolatedCommit` runs.
- [ ] Replaying scenario 002 (`scenarios/002-workflow-pr.md`) against `~/Documents/workspaces/dark-factory-sandbox` produces an open PR end-to-end. Verified at runtime, not via unit tests alone.
- [ ] No regression in `worktree` workflow: re-running the spec 065/066/067 runtime replays in `~/Documents/workspaces/jira-task-creator` against the new binary still produces PRs.
- [ ] No regression in `branch` workflow: same as above for branch-workflow replays.
- [ ] No regression in `direct` workflow: dark-factory continues to self-host releases (the next prompt processed against dark-factory itself produces a new release tag).
- [ ] No-diff case (372) works for clone: a prompt that produces no diff inside the clone results in `no staged changes — skipping commit`, the clone is removed, the prompt is moved to `completed/`, and no PR is opened.
- [ ] `scenarios/002-workflow-pr.md` is updated if its language is now stale (e.g. if push order changed; otherwise no change needed).
- [ ] `docs/workflows.md` `clone` row commentary documents the clone-specific push-before-remove ordering (or whichever invariant the chosen fix shape establishes).
- [ ] Unit test asserts that push precedes clone removal in the chosen fix shape (file path and mock-API specifics are decided at fix-prompt generation time, not bound here).

## Verification

Per `docs/bug-workflow.md` §Verification, this is a runtime symptom — unit tests alone are not sufficient.

**Repro replay (must run after fix lands):**

```bash
# Use the rebuilt binary
go build -C ~/Documents/workspaces/dark-factory -o /tmp/dark-factory-v0.148 .

# Set up the sandbox per scenario 002
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"
cat > .dark-factory.yaml << 'YAML'
workflow: pr
maxContainers: 999
YAML

# Drop the toggle-comment prompt (per scenario 002)
# ... (see scenarios/002-workflow-pr.md for the exact prompt body)

/tmp/dark-factory-v0.148 prompt approve toggle-comment
/tmp/dark-factory-v0.148 run

# Expected log lines (in order):
#   "cloning repo"
#   "docker container exited" exitCode=0
#   "completion report" status=success
#   "no new commits on branch — skipping push/PR"  (if no-diff)
#   OR
#   "pushed branch dark-factory/..."
#   "created PR url=..."
#   "clone removed"  (or equivalent)
# MUST NOT see:
#   "exit status 128 ... count commits ahead"

# Confirm:
gh pr list --state open --search "head:dark-factory/" | grep -c toggle-comment   # ≥1
git ls-remote origin 'refs/heads/dark-factory/*' | grep toggle-comment            # branch exists on origin

# Cleanup
gh pr close <pr-number>
git push origin --delete dark-factory/<prompt-name>
rm -rf "$WORK_DIR"
```

**Negative-control replay:**

1. Drop a prompt that produces no diff (e.g. an idempotent edit that doesn't change anything). Expected: clone created, container runs, no commit, no push, no PR, clone removed, prompt moved to `completed/`. Log shows `no staged changes — skipping commit`. The 372 fix continues to work for clone.
2. Drop a prompt where the YOLO container fails with non-zero exit. Expected: existing CleanupOnError path removes the clone; no `exit 128` post-mortem.

**Acceptable evidence for `verifying → completed`:**

| Evidence | Acceptable? |
|----------|-------------|
| Daemon log shows `created PR url=...` after scenario 002 replay | Yes |
| `gh pr list` shows the test PR open at the end of the replay | Yes |
| `git ls-remote origin` shows the feature branch present | Yes |
| Spec 065/066/067 runtime replays in jira-task-creator still pass on the new binary | Yes (regression check) |
| Unit test asserts push-before-clone-remove ordering | Necessary but not sufficient |
| "All tests pass" without runtime replay | No |

## Open Questions

1. ~~Which fix shape (A/B/C/D)?~~ **Resolved: A+C combination** (see "Fix shape (resolved)" section above).
2. ~~Does `Cloner` already have a "push then remove" helper?~~ **Resolved: no.** Per Explore findings, `Cloner` has only `Clone` and `Remove`. The fix uses `Brancher.Push` from inside the clone executor (no new `Cloner` helper needed) and adds a new `Brancher.FetchBranch` (or reuses an existing fetch helper if discovered during fix-prompt generation).
3. Is `workflow: clone + pr: false` (push without PR) a real use case, or always paired with `pr: true`? The fix should handle both, but priority depends on usage.
4. ~~Stopgap error message?~~ Out of scope. The fix lands quickly enough that a separate "clone workflow is broken" error path would itself be technical debt. Decline.
5. Coordinate landing order: this is a 5th bug in the same family. After it ships, consider whether option D from spec 066 (sidecar state dir / restructure of post-commit pipeline) is the right longer-term answer.

## See also

- Spec 063 (`bug-autorelease-overrides-pr-workflow`) — sibling dispatch-path bug.
- Spec 065 (`bug-pr-create-missing-head-flag-in-isolated-workflows`) — sibling chokepoint bug.
- Spec 066 (`bug-branch-workflow-rejects-its-own-uncommitted-prompt-file-write`) — sibling branch-workflow bug.
- Spec 067 (`bug-branch-workflow-checkout-fails-on-divergent-prompt-file`) — sibling branch-workflow bug.
- `pkg/processor/workflow_executor_clone.go:99-127` — the broken Complete path.
- `pkg/processor/workflow_executor_worktree.go:99-127` — the working analog (worktrees share `.git/`).
- `pkg/processor/workflow_helpers.go:213-268` — the post-commit pipeline that assumes branch-is-locally-visible.
- `scenarios/002-workflow-pr.md` — the scenario that surfaces this bug.

## Verification Result

**Verified:** 2026-05-05T12:02:29Z (HEAD `81bb4df`, post-v0.148.5)
**Binary:** `/tmp/dark-factory-068` (built fresh from source for verification)
**Scenario:** clone+pr replay against `~/Documents/workspaces/dark-factory-sandbox` with `workflow=clone, pr=true`
**Evidence:**
- Daemon log: `level=INFO msg="created PR" url=https://github.com/bborbe/dark-factory-sandbox/pull/3`
- No `exit 128` and no `count commits ahead` failure in the daemon run
- `gh pr list -R bborbe/dark-factory-sandbox` → PR #3 OPEN at SHA `215f5c50c6f20ad1fd0f4d8e6896b17a3599ca3e`
- Effective config (from log): `workflow=clone (project) pr=true (project)` — exact bug-trigger condition
- Cleanup: PR #3 closed, branch `dark-factory/002-verify-068` deleted on origin
**Verdict:** PASS

(Appended retroactively — spec was completed before the v0.149.3 spec-verifier introduced the auto-append step.)
