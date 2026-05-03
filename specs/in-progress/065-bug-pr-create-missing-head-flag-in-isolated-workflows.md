---
status: prompted
approved: "2026-05-03T19:33:27Z"
generating: "2026-05-03T19:35:11Z"
prompted: "2026-05-03T19:39:12Z"
branch: dark-factory/bug-pr-create-missing-head-flag-in-isolated-workflows
---

# `gh pr create` invoked without `--head` in worktree/clone workflows → "head==base" error

## Summary

For `workflow: worktree` (and very likely `workflow: clone`), PR creation fails immediately after a successful commit + push. `prCreator.Create` calls `gh pr create` without an explicit `--head <branch>` flag. Because the worktree workflow `os.Chdir`s back to the original directory (the master worktree) before invoking `Create`, `gh` infers `--head` from the current branch — which is `master` — and rejects the request with `head branch "master" is the same as base branch "master"`. The feature branch is committed and pushed correctly; only the PR opening step is broken.

## Reproduction

dark-factory version: `v0.147.2-1-g30ba42f`

1. Project: `~/Documents/workspaces/jira-task-creator`
2. `.dark-factory.yaml`:
   ```yaml
   workflow: worktree
   pr: true
   autoMerge: true
   autoRelease: true
   autoReview: true
   allowedReviewers: ["bborbe", "pr-review-of-ben"]
   ```
3. Drop a draft prompt in `prompts/`, e.g. `009-test-loglevel-handler.md` (anything that produces ≥1 file change).
4. `dark-factory prompt approve 009`
5. `dark-factory daemon`
6. Observe in `prompts/log/009-test-loglevel-handler.log` and frontmatter:

   - Test was written, `make test` and `make precommit` passed inside the YOLO container.
   - Branch `dark-factory/009-test-loglevel-handler` was created with the commit `Add unit tests for CreateSetLoglevelHandler` and pushed to `origin`.
   - Prompt frontmatter then flips to `status: failed` with:

     ```
     lastFailReason: |
         find or create PR: create pull request: create pull request: exit status 1: Warning: 2 uncommitted changes
         head branch "master" is the same as base branch "master", cannot create a pull request
     ```

7. `git branch -a` confirms the branch exists locally and on origin:

   ```
   dark-factory/009-test-loglevel-handler
   remotes/origin/dark-factory/009-test-loglevel-handler
   ```
8. `git rev-list --count master..dark-factory/009-test-loglevel-handler` returns `1` — the commit is on the right branch.
9. Manually running `gh pr create --head dark-factory/009-test-loglevel-handler --title "..." --body ""` from the master worktree succeeds. Confirms the missing flag is the fault.

## Expected vs Actual

**Expected** (per `docs/workflows.md` `worktree` row, "Delivery" column = "branch + PR"):
> After commit on the isolated branch in the temp worktree, dark-factory pushes the branch and opens a PR against the default branch.

**Actual:**
- Branch is committed and pushed correctly.
- `gh pr create` is invoked with no `--head` flag.
- Current cwd at PR-create time is the original (master) worktree because `worktreeWorkflowExecutor.Complete` does `os.Chdir(e.originalDir)` (`pkg/processor/workflow_executor_worktree.go:108`) before calling `handleAfterIsolatedCommit`.
- `gh` infers `--head` from the current branch (`master`) and errors out: `head branch "master" is the same as base branch "master"`.

## Why this is a bug

The contract from `docs/workflows.md` is "isolated commit on a feature branch, then push + PR." The push step works (it explicitly takes `branchName`). The PR step silently drops `branchName` on the floor.

Asymmetry inside the same package proves the omission is unintentional:

- `pkg/git/pr_creator.go:56-67` — `FindOpenPR` uses `--head branch` correctly.
- `pkg/git/pr_creator.go:77-93` — `Create` does not pass `--head` at all.

The interface `Create(ctx, title, body) (string, error)` doesn't accept a branch, so callers can't supply it even if they wanted to.

This combination (missing flag + cwd reset to master before Create) means the documented `worktree` + `pr` happy path is unreachable. By inspection, `clone` workflow has the same shape (commits in a separate clone dir, pushes, returns to original cwd) and likely reproduces the same bug — needs verification.

## Workaround

After `status: failed` with the head-equals-base error, recover manually from the master worktree:

```bash
gh pr create \
  --head dark-factory/<prompt-name> \
  --title "<commit title>" \
  --body ""
```

Then `dark-factory prompt complete <name>` (or merge the PR and let the next daemon cycle pick up).

## Code pointers

- `pkg/git/pr_creator.go:77-93` — extend the `PRCreator` interface to take `branch string`, pass `--head <branch>` in the `gh pr create` argv.
- `pkg/processor/workflow_helpers.go:findOrCreatePR` and `findOrCreatePR` callers — thread the existing `branchName` parameter through to `Create`.
- `pkg/git/bitbucket_pr_creator.go` — apply the same fix for the Bitbucket provider; verify whether it has the analogous flaw.

## Failure modes the fix MUST cover

1. `workflow: worktree + pr: true` → PR opens against default branch with the feature branch as head.
2. `workflow: clone + pr: true` → same.
3. `workflow: branch + pr: true` (existing-working path) → continues to work; the explicit `--head` should match the branch the workflow checked out, not regress.
4. The "2 uncommitted changes" `gh` warning observed alongside the head==base error — figure out where those come from (likely the `.dark-factory.yaml` and prompt-status writes happening in the master worktree before PR-create); decide whether they should be staged/cleaned before invoking `gh` or whether they're benign and just noisy.
5. Bitbucket provider parity (or explicit non-applicability note).

## Acceptance Criteria

- [ ] `gh pr create` is invoked with an explicit `--head <featureBranch>` for `worktree`, `clone`, and `branch` workflows.
- [ ] `PRCreator.Create` interface signature includes the branch (or an equivalent typed value) so callers cannot omit it.
- [ ] Replaying the reproduction in `~/Documents/workspaces/jira-task-creator` with `workflow: worktree` produces an open PR (not a `head==base` error). Verified at runtime, not via unit tests alone.
- [ ] Existing `branch` workflow continues to work end-to-end (regression scenario).
- [ ] `clone` workflow either opens a PR end-to-end (verified at runtime), or the spec records evidence that the bug does not apply to `clone`.
- [ ] Unit test exists for `prCreator.Create` asserting the argv contains `--head <branch>`.
- [ ] Bitbucket PR creator (`pkg/git/bitbucket_pr_creator.go`) is reviewed for the same flaw and fixed if applicable.

## Open Questions

1. Should the fix also force-`chdir` into the feature branch before invoking `gh`, as a belt-and-suspenders measure? (Probably not — explicit `--head` is the contract; cwd-dependence is the latent fragility we're removing.)
2. Are there other `gh` subcommands in `pkg/git/` with the same pattern (cwd-inferred branch where an explicit flag would be safer)? `pr_merger.go` uses the PR URL directly so it's safe; check `prCreator.FindOpenPR` callers, status checks, etc.
3. The "2 uncommitted changes" — investigate whether `.dark-factory.yaml` or in-progress prompt files are being touched mid-flow in the master worktree. If yes, that's a separate cleanliness issue worth tracking; if no, the warning is just `gh` describing pre-existing dirt.

## See also

- `pkg/git/pr_creator.go` — the offending implementation.
- `pkg/processor/workflow_executor_worktree.go:99-127` — the cwd reset before `handleAfterIsolatedCommit`.
- `pkg/processor/workflow_helpers.go:213-268` — `handleAfterIsolatedCommit` push + PR sequence.
- Spec 063 (`bug-autorelease-overrides-pr-workflow`) — sibling bug in the same dispatch path; this one is in the PR-create implementation rather than the dispatch decision.
