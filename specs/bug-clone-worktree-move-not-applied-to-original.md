---
tags:
  - dark-factory
  - spec
  - bug
status: idea
kind: bug
---

## Summary

- Spec 086 moved the prompt rename BEFORE the work commit so all four workflows produce one combined commit (code + rename).
- For direct and branch workflows this is correct because the rename happens in the same repo the daemon sees.
- For clone and worktree workflows the rename happens INSIDE the isolated working tree, is pushed to the remote in the combined commit, then the isolated tree is destroyed — but the ORIGINAL repo's prompt file is never moved.
- After the run, the ORIGINAL repo still has the prompt at `prompts/in-progress/<id>.md` while the remote shows it at `prompts/completed/<id>.md`. The daemon's local view diverges from remote.
- Downstream effect: `savePRURLToFrontmatter` opens `prompts/completed/<id>.md` in the original repo, fails with `no such file or directory`, and the workflow logs a hard error on every clone/worktree run.

## Problem

Spec 086 (immutable, completed) ordered the prompt-move-to-completed BEFORE the work commit so workflows produce a single combined commit instead of two. The change is correct for direct and branch workflows because they operate in the same repository the daemon observes. For clone and worktree workflows the move occurs inside an isolated working tree that is removed after push; the original repository's working copy is never updated. The next step (`savePRURLToFrontmatter` in `handleAfterIsolatedCommit`) then tries to open the prompt at its new path in the ORIGINAL repo, crashes, and the workflow finishes with a divergent local state and a hard error in the daemon log.

## Reproduction

dark-factory version: tip of `master` after spec 086 merged (2026-05-23).

Configuration: scenario 002 (prompt with `pr: true`), workflow mode `clone`.

Command:

```
$ /tmp/new-dark-factory run
```

Observed log:

```
[10:44:10] dark-factory: moved to completed file=006-toggle-comment.md  (inside clone)
[10:44:14] dark-factory: open PR already exists ... url=https://github.com/bborbe/dark-factory-sandbox/pull/10
[10:44:14] dark-factory: WARN failed to save PR URL to frontmatter
   error: "open prompts/completed/006-toggle-comment.md: no such file or directory"
   stack: pkg/prompt/prompt.go:316 load → :985 setPRURL → :661 SetPRURL
          → pkg/processor/workflow_helpers.go:96 savePRURLToFrontmatter
          → pkg/processor/workflow_helpers.go:249 handleAfterIsolatedCommit
          → pkg/processor/workflow_executor_clone.go:135 (*cloneWorkflowExecutor).Complete
```

Post-run filesystem state of the ORIGINAL repo:

```
$ ls prompts/completed/toggle-comment.md
ls: cannot access 'prompts/completed/toggle-comment.md': No such file or directory

$ ls prompts/in-progress/
006-toggle-comment.md
```

Remote state:

```
$ gh pr list --repo bborbe/dark-factory-sandbox --state open
10  006-toggle-comment   dark-factory/006-toggle-comment   OPEN
# Remote master contains the combined commit with the prompt at prompts/completed/.
```

The same reproduction applies to workflow mode `worktree` (`workflow_executor_worktree.go` follows the same pattern as `workflow_executor_clone.go`).

## Expected vs Actual

Expected (per spec 086 + workflow invariants): after a successful clone or worktree run, the ORIGINAL repo's prompt file is at `prompts/completed/<id>.md` with `status: completed` in frontmatter. The daemon's local view agrees with what was pushed to the remote. No `savePRURLToFrontmatter` error.

Actual: ORIGINAL repo's prompt file is still at `prompts/in-progress/<id>.md`. Remote master shows the prompt at `prompts/completed/`. `savePRURLToFrontmatter` errors with `no such file or directory` on every run.

## Why this is a bug

The lifecycle invariant is: after a successful workflow execution, the daemon's local repository view must agree with what was pushed to the remote. Spec 086 preserved this for direct and branch workflows. The same invariant must hold for clone and worktree workflows — the user's local repo cannot be left in a state where the remote is ahead and the prompt's local path contradicts its remote path. The current behavior also produces a hard error (`failed to save PR URL to frontmatter`) on every clone/worktree run, making the failure mode permanent and visible.

## Goal

Clone and worktree workflows end with the ORIGINAL repo's prompt at `prompts/completed/<id>.md`, with `status: completed` in frontmatter, and with no additional git commit in the original. The daemon's local view matches the pushed remote state. `savePRURLToFrontmatter` finds the file at its new path and updates frontmatter without error.

## Non-goals

- Reverting spec 086. The combined-commit fix is immutable; this spec is additive.
- Changing the direct or branch workflow code paths.
- Implementing daemon-startup recovery for the "crash between push and original-move" failure mode. It is documented for completeness; implementation can be deferred to a follow-up.
- Changing how PR URL is persisted in frontmatter.
- Introducing any branch protection bypass.

## Desired Behavior

1. After a clone-workflow run pushes the combined commit, the ORIGINAL repo's prompt is renamed from `prompts/in-progress/<id>.md` to `prompts/completed/<id>.md`.
2. After a worktree-workflow run pushes the combined commit, the same rename occurs in the ORIGINAL repo.
3. The rename in the ORIGINAL repo is filesystem-only — it produces NO additional git commit, because the rename is already present in the pushed combined commit.
4. `savePRURLToFrontmatter` opens the prompt at the new path successfully; no `no such file or directory` log line appears.
5. If the rename in the ORIGINAL repo fails after the remote push succeeded, the daemon emits a `clone-sync-mismatch` WARN that names both paths and instructs the operator to `git pull`. The workflow does not crash.
6. Re-running the same prompt after a partial failure does not error on the rename step (idempotent — already-at-completed is treated as success).
7. Direct and branch workflows behave exactly as they did after spec 086 (no regression).

## Constraints

- Direct workflow MUST continue to produce a single combined commit (no regression in spec 086).
- Branch workflow MUST continue to produce a single combined commit (no regression in spec 086).
- Clone and worktree workflows MUST NOT add a second git commit in the ORIGINAL repo for the post-push rename — the rename is already in the pushed combined commit, so this is a filesystem-only operation.
- No new bypass paths for branch protection.
- The post-push original-repo rename MUST be idempotent: if the prompt is already at `prompts/completed/<id>.md`, the operation succeeds without error.
- See `docs/bug-workflow.md` for the bug spec lifecycle.

## Failure Modes

| Trigger | Expected behavior | Detection | Reversibility | Recovery |
|---|---|---|---|---|
| Original-repo rename fails after clone/worktree push succeeded | WARN `clone-sync-mismatch` naming both paths; workflow does not crash; remote is ahead of local | non-zero return from the post-push rename | reversible | Operator runs `git pull` on the original repo to catch up |
| File already at `prompts/completed/<id>.md` in original (idempotent re-run) | No-op success; debug log noting already-at-destination | rename detects same source/dest or destination already exists | reversible | n/a |
| Clone/worktree removed but chdir back to original failed | Workflow fails fast; original prompt untouched at `in-progress/`; no partial rename | chdir error returned | reversible | Operator restarts daemon |
| Daemon crash between remote push and original-repo rename | On next startup, daemon observes: remote `master` has prompt at `completed/`, original has prompt at `in-progress/`. Daemon emits `clone-sync-mismatch` WARN with recovery instructions (auto-sync via `git pull` is a follow-up). | mismatch between `git log origin/master` for the prompt and local filesystem state | reversible | Operator `git pull` on original repo OR daemon-startup recovery (follow-up spec) |
| External `gh`/git unavailable during push | Existing push-failure handling applies — combined commit not pushed, no original-repo rename attempted, prompt stays at `in-progress/` in both original and clone | push exit code non-zero | reversible | Operator retries when network/`gh` restored |
| Concurrent daemon instances on same repo | Second instance observes prompt already at `completed/` (idempotent path), no-op; or observes mid-rename and fails fast on stale state | lock file or git index state | partial | Daemon-instance lock (existing); second instance defers |

## Acceptance Criteria

- [ ] Clone workflow ends with the prompt at `prompts/completed/<id>.md` in the ORIGINAL repo — evidence: after a fresh clone-workflow scenario 002 run, `ls prompts/completed/<id>.md` in the original exits 0 AND `ls prompts/in-progress/<id>.md` exits 2 (file absent).
- [ ] Worktree workflow ends with the same state — evidence: same `ls` exit-code test after a worktree-mode scenario run.
- [ ] `savePRURLToFrontmatter` no longer errors with `no such file or directory` after a clone or worktree run — evidence: `grep -c "failed to save PR URL to frontmatter" <daemon.log>` returns 0 after a scenario 002 run.
- [ ] The post-push rename in the original repo produces no additional git commit — evidence: after the scenario run, `git -C <original-repo> log origin/master..HEAD --oneline` returns empty output (no local-only commits).
- [ ] Direct workflow continues to produce a single combined commit — evidence: scenario 001 (direct mode) run shows exactly one commit added to master containing both the code change and the prompt rename (`git log -1 --stat` lists both).
- [ ] Branch workflow continues to produce a single combined commit — evidence: existing pkg/processor ginkgo tests for branch mode pass; new ginkgo assertion that the branch's tip commit contains both the code change and the rename.
- [ ] Idempotent rerun — evidence: a ginkgo unit test that invokes the original-repo move twice on the same prompt succeeds on the second call (no error) and emits a debug log indicating already-at-destination.
- [ ] Failure of the original-repo rename does not crash the workflow — evidence: a ginkgo test that injects a rename failure after push asserts the workflow returns success-with-warning AND the daemon log contains a line matching `clone-sync-mismatch`.
- [ ] New integration test covers clone + worktree original-repo move — evidence: a ginkgo test drives `cloneWorkflowExecutor.Complete` and `worktreeWorkflowExecutor.Complete` end-to-end against a bare remote and asserts the ORIGINAL repo's filesystem has `prompts/completed/<id>.md` present and `prompts/in-progress/<id>.md` absent.
- [ ] `make precommit` exits 0 in the changed module.
- [ ] Live verification: scenario 002 re-run against `/tmp/new-dark-factory` in clone mode shows the original repo's prompt at `prompts/completed/<id>.md` after the run, the daemon log contains zero `failed to save PR URL to frontmatter` matches, and the PR remains open on the remote with the correct content.

## Verification

```
# Run scenario 002 in clone mode against the sandbox repo
/tmp/new-dark-factory run

# Then in the original repo:
ls prompts/completed/<id>.md          # exits 0
ls prompts/in-progress/<id>.md        # exits 2
git log origin/master..HEAD --oneline # empty
grep -c "failed to save PR URL to frontmatter" <daemon.log>  # 0

# Re-run scenario 002 in worktree mode — same shell assertions

# Re-run scenario 001 (direct) — confirm single combined commit (no regression)

# Run module tests
make precommit
```

Verification follows `docs/spec-verification.md` and the bug-specific rule from `docs/bug-workflow.md`: the original reproduction must be replayed against the fixed build and produce the expected behavior. Inspection-only evidence is not acceptable.

## Do-Nothing Option

Clone and worktree workflows remain broken — every prompt run leaves the original repo's daemon view at `in-progress/` while the remote shows the prompt at `completed/`. The `savePRURLToFrontmatter` error fires on every run. Operators must manually `git pull` after each clone/worktree run to recover, and the daemon log accumulates hard errors that mask real failures. Not acceptable.
