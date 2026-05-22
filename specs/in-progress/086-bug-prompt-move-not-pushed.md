---
status: prompted
tags:
    - dark-factory
    - spec
    - bug
approved: "2026-05-22T18:31:40Z"
generating: "2026-05-22T18:31:40Z"
prompted: "2026-05-22T18:34:30Z"
branch: dark-factory/bug-prompt-move-not-pushed
---

## Summary

- After a prompt completes, the daemon moves the file from `prompts/in-progress/<id>.md` to `prompts/completed/<id>.md` in a SECOND commit that is never pushed to GitHub.
- After the work PR merges, `origin/master` still shows the prompt under `in-progress/`, while the local daemon believes it lives at `completed/` — master and the daemon diverge.
- All four workflow modes (direct, branch, clone, worktree) exhibit the divergence — the move-then-commit ordering is the same defect in different places.
- Fix: move the file BEFORE the work commit so a single commit contains both the code change and the rename; the PR diff shows the prompt at its final `completed/` location.
- Semantically the PR carries a "completed" prompt while still open, but the divergence between local state and merged master disappears and tooling (git, PR review, BRO-20203-style repro) stays consistent.

## Reproduction

Setup: any dark-factory checkout running against a repo whose workflow is `branch`, `clone`, or `worktree` (the worktree default for `lib-crypto` reproduces this; the BRO-20203 task observed it).

Steps:

1. Queue a prompt that produces a non-trivial code change.
2. Let the daemon execute the prompt to completion. It opens a PR with the code change.
3. Merge the PR on GitHub.
4. Locally: `git fetch origin && git ls-tree origin/master prompts/in-progress/ | grep <prompt-id>`
5. Locally: `find prompts/completed -name '<prompt-id>.md'`

Observed evidence (verbatim from the repro):

```
$ git ls-tree origin/master prompts/in-progress/ | grep <prompt-id>
100644 blob <sha>    prompts/in-progress/<prompt-id>.md      # STILL on master after merge

$ find prompts/completed -name '<prompt-id>.md'
prompts/completed/<prompt-id>.md                              # local daemon moved it
```

Affected source locations (functions, not line ranges — line numbers will drift):

- `pkg/processor/workflow_executor_direct.go` — commits work, then moves file, then commits move (two-commit pattern)
- `pkg/processor/workflow_executor_clone.go` — commit in clone, push from clone, chdir back, `handleAfterIsolatedCommit` does the move-commit which is not pushed by the same code path
- `pkg/processor/workflow_executor_worktree.go` — same two-phase pattern as clone
- `pkg/processor/workflow_executor_branch.go` — calls `moveToCompletedAndCommit`
- `pkg/processor/workflow_helpers.go` — `moveToCompletedAndCommit`, `handleAfterIsolatedCommit`
- `pkg/prompt/prompt.go` — `MoveToCompleted` (sets frontmatter `status: completed`, renames the file on disk)

Version: latest master at time of report (commit `8feabd80c` or newer).

## Expected vs Actual

Expected: After a prompt completes and its PR merges, `origin/master` reflects the prompt at `prompts/completed/<id>.md` and NOT at `prompts/in-progress/<id>.md`. The PR diff is the source of truth: code change + prompt move land together. (Cited contract: "diff + prompt = technical how" — the merged diff must include the prompt at its final location for this contract to hold.)

Actual: The work commit (and its PR) only contains the code change. The prompt move is a separate commit on the local daemon clone/worktree that is never pushed. After PR merge, master shows the prompt still in `in-progress/`, while the local repo (daemon view) shows it under `completed/`. The two are permanently divergent until manual reconciliation.

## Why this is a bug

`docs/workflows.md` and the prompt lifecycle treat `prompts/completed/` as the canonical "this prompt is done" location. If master never sees the file move, then:

1. Anyone re-cloning the repo loses the daemon's completion record.
2. PR review cannot tell which prompt produced the change (the prompt that justified the diff is still listed as in-progress).
3. The "diff + prompt = technical how" contract is silently broken — readers of the merged commit cannot find the prompt at its documented final path.

The defect is in the order of operations: move-then-commit collapses to a single observable artifact (one commit, one PR), commit-then-move splits the lifecycle across one pushed commit and one unpushed commit.

## Goal

After every successful prompt execution — across all four workflow modes — the work commit pushed to the remote contains BOTH the code change AND the rename from `prompts/in-progress/<id>.md` to `prompts/completed/<id>.md`. No second "move" commit exists. Local daemon view and `origin/master` agree on the prompt's location after PR merge.

## Constraints

- Existing single-prompt success behavior unchanged from a user/operator perspective: the prompt still ends up at `prompts/completed/<id>.md` with frontmatter `status: completed`.
- All four workflow modes (`direct`, `branch`, `clone`, `worktree`) MUST converge on the same lifecycle order: move → stage → commit → push. No mode-specific divergence.
- `MoveToCompleted` may change signature if needed, but its externally-observable effect (frontmatter status set to `completed`, file relocated to `prompts/completed/`) MUST remain identical.
- Existing tests in `pkg/processor/` and `pkg/prompt/` MUST continue to pass.
- Failure of the work commit MUST NOT leave the prompt in an inconsistent on-disk state (no orphan `completed/` file with no corresponding commit on master).
- Branch protection rules MUST continue to be respected — no new bypass paths.

## Desired Behavior

1. Before staging files for the work commit, the daemon moves the prompt file from `prompts/in-progress/<id>.md` to `prompts/completed/<id>.md` and updates its frontmatter to `status: completed`.
2. The single work commit produced by each workflow mode contains both the code changes AND the file rename.
3. The push that publishes the work commit also publishes the rename — no second unpushed commit exists for the move.
4. If the work commit fails (e.g., precommit hook, branch protection, push rejection), the daemon ROLLS THE PROMPT FILE BACK to `prompts/in-progress/` with its original frontmatter (`status: committing` (the prior PromptStatus value before MoveToCompleted ran; no `in_progress` PromptStatus exists)). No orphan `completed/` file is left on disk. This applies uniformly across all four workflow modes.
5. After a prompt's PR merges, `git ls-tree origin/master prompts/in-progress/` does NOT contain the prompt id, and `git ls-tree origin/master prompts/completed/` does contain it.
6. Daemon restart in the middle of execution (after move, before commit) recovers cleanly: either the move is reverted, or the next loop iteration completes the commit-and-push.

## Failure Modes

| Trigger | Expected behavior | Detection | Reversibility | Recovery |
|---------|-------------------|-----------|---------------|----------|
| Work commit fails after move (precommit hook rejects) | File rolled back to `in-progress/` with `status: committing` (the prior PromptStatus value before MoveToCompleted ran; no `in_progress` PromptStatus exists) restored | non-zero exit from `git commit`; log line `move-rolled-back-after-commit-failure` with the failing command output | reversible | Operator fixes the precommit cause; daemon re-runs the prompt, producing a single combined commit |
| Push fails after successful local commit (network, auth, branch protection) | Local single commit (code + move) retained; daemon emits push failure with remote refusal verbatim | non-zero exit from `git push`; log line `push-failed-after-move-commit` with the remote error verbatim | irreversible without manual git intervention | Operator manually pushes or `git reset --soft HEAD~1` + retry; no on-disk state change needed |
| Prompt has no frontmatter `status:` field | Treated as fresh in-progress prompt on the next daemon loop | parse on next daemon loop; log line `prompt-missing-status-treated-as-in-progress` | reversible | n/a — daemon proceeds normally |
| Branch protection blocks direct-to-master in `direct` workflow | Workflow fails fast BEFORE any file movement on disk; no orphan move | push exit code non-zero, no `completed/` file exists, log line `direct-push-blocked-no-move` | reversible (no state change) | Operator switches repo workflow config to a PR-based mode (`branch`, `clone`, `worktree`) |
| Daemon crash between move and commit | On next startup, daemon detects the staged-but-uncommitted move and rolls back to `in-progress/` | git status shows staged rename of `prompts/in-progress/<id>.md` → `prompts/completed/<id>.md` with no HEAD commit referencing it | reversible | Daemon recovery routine runs on startup; operator re-queues if recovery cannot proceed |
| Two daemon instances racing on the same prompt | File system rename is atomic; second instance sees the file already at `completed/` and skips | duplicate-execution guard log line `prompt-already-moved-skipping` | n/a (one wins) | n/a — race resolved by atomic rename |

## Acceptance Criteria

- [ ] All four workflow modes (`direct`, `branch`, `clone`, `worktree`) move the prompt file BEFORE the work commit — evidence: `git log -p --name-status <work-commit-sha>` on a PR produced by each workflow mode shows BOTH the code change AND a `R prompts/in-progress/<id>.md  prompts/completed/<id>.md` rename entry in the SAME commit
- [ ] After a PR merge produced by each workflow mode, `git ls-tree origin/master prompts/in-progress/ | grep <id>` returns empty (exit code 1 from grep) AND `git ls-tree origin/master prompts/completed/ | grep <id>` returns exactly one line — evidence: shell exit codes and grep output
- [ ] No second unpushed move-commit exists after a successful prompt run — evidence: `git log origin/master..HEAD --oneline` is empty after the workflow completes
- [ ] Ginkgo tests cover all four workflow modes for the move-before-commit ordering — evidence: `ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'move.*before.*commit'` returns ≥1 matching line per workflow mode (≥4 total); each matching test must invoke `git log --name-status` (or equivalent porcelain) on the produced commit and assert that the output contains both a non-prompt file modification AND the `R prompts/in-progress/<id>.md  prompts/completed/<id>.md` rename — naming alone is not sufficient
- [ ] Failure-mode behavior for "work commit fails after move" is exercised by a test that asserts the file is restored to `prompts/in-progress/<id>.md` with `status: committing` (the prior PromptStatus value before MoveToCompleted ran; no `in_progress` PromptStatus exists) — evidence: `ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'commit fail.*roll(back|ed)'` returns ≥1 line; test body must assert post-condition file path AND frontmatter contents
- [ ] Failure-mode behavior for "push fails after commit" is exercised by a test — evidence: `ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'push fail.*after.*move'` returns ≥1 line
- [ ] `MoveToCompleted` external contract preserved — evidence: existing unit tests in `pkg/prompt/` pass unchanged: `go test ./pkg/prompt/... -count=1` exits 0
- [ ] `make precommit` exits 0 from the dark-factory repo root after the fix lands — evidence: shell exit code
- [ ] BRO-20203 repro (originating Jira ticket from the Brogrammers tracker; observed on `lib-crypto`) no longer shows divergence after a prompt PR merges — evidence: a new integration or scenario test replays the original repro and asserts that `origin/master` after merge contains the prompt only under `completed/`. Test name must contain `bro-20203` or `lib-crypto-divergence` so it can be located via `grep`.

## Verification

Manual reproduction replay (mandatory per `docs/bug-workflow.md`):

```
# 1. Queue a prompt that produces a code change on a repo using `worktree` workflow
# 2. Let the daemon execute and open a PR
# 3. Merge the PR on GitHub
# 4. Verify master shows the prompt only at completed/
git fetch origin
git ls-tree origin/master prompts/in-progress/ | grep <prompt-id>   # exit 1 expected
git ls-tree origin/master prompts/completed/   | grep <prompt-id>   # exit 0, one line
# 5. Verify no local divergence
git log origin/master..HEAD --oneline                                # empty expected
# 6. Repeat for each of the four workflow modes (direct, branch, clone, worktree)
```

Automated:

```
make precommit
ginkgo -v ./pkg/processor/
ginkgo -v ./pkg/prompt/
go test ./... -count=1
```

## Non-goals

- Fixing the related "Unreleased → vX.Y.Z rename after PR merge" divergence (tracked separately).
- Retroactively migrating already-merged PRs whose prompts are stuck in `in-progress/` on master.
- Changing the PR review process or the PR template.
- Adding branch-protection bypass paths.
- Changing the externally-visible API of `MoveToCompleted` beyond what is required to support move-before-commit.

## Do-Nothing Option

Divergence continues on every prompt run; the alternative is an out-of-band reconciliation job, which is strictly more complex than fixing the ordering and can race with new executions.
