---
status: committing
spec: [086-bug-prompt-move-not-pushed]
summary: Refactored prompt lifecycle across all four workflow modes so the move from in-progress/ to completed/ happens before the work commit, with rollback on failure
container: dark-factory-exec-405-spec-086-move-before-commit
dark-factory-version: v0.164.0
created: "2026-05-22T00:00:00Z"
queued: "2026-05-22T18:43:12Z"
started: "2026-05-22T18:43:14Z"
branch: dark-factory/bug-prompt-move-not-pushed
---

<summary>
- All four workflow modes (direct, branch, clone, worktree) move the prompt file from `prompts/in-progress/` to `prompts/completed/` BEFORE the work commit, so a single commit contains both code changes and the rename.
- A new `RollbackMoveToCompleted` method on the prompt `Manager` (and its interface) reverses the move, restoring the file at `prompts/in-progress/<id>.md` with the prior frontmatter status (`committing`).
- On work-commit failure in `direct` and `branch` modes, the executor rolls the prompt file back; in `branch` the rollback runs BEFORE the default-branch is restored (otherwise the rollback writes the file on the wrong branch).
- In `clone` and `worktree` modes the move happens inside the isolated working tree; on failure inside the clone/worktree the executor does NOT call rollback (the clone is discarded entirely on cleanup).
- The dead-code helper `moveToCompletedAndCommit` and the post-commit move path inside `handleAfterIsolatedCommit` are removed; the move only ever happens before the work commit, never after.
- `MoveToCompleted`'s externally observable behaviour is unchanged (frontmatter `status: completed`, file at `prompts/completed/`).
- `docs/workflows.md` is updated to describe the new lifecycle order (`move → stage → commit → push`).
- `CHANGELOG.md` gets a new entry under `## Unreleased` describing the fix.
</summary>

<objective>
After this prompt lands, every successful prompt execution produces exactly one work commit on the relevant branch, and that commit contains both the code changes and the rename `prompts/in-progress/<id>.md → prompts/completed/<id>.md`. No second "move" commit exists anywhere in the codebase. On work-commit failure, the prompt file is restored to `prompts/in-progress/<id>.md` with frontmatter `status: committing`.
</objective>

<context>
Read first (no edits yet):

- `CLAUDE.md` — project conventions, lifecycle, never-amend rule.
- `docs/workflows.md` — current documented lifecycle; this prompt updates it.
- `docs/bug-workflow.md` — bug-reproduction methodology.
- `specs/in-progress/086-bug-prompt-move-not-pushed.md` — the spec this prompt implements.

Code anchors (function names, not line numbers — line numbers drift):

- `pkg/prompt/prompt.go` — `Manager` struct, `MoveToCompleted`, unexported `moveToCompleted`, status constants (`CommittingPromptStatus`, `CompletedPromptStatus`). There is no `in_progress` PromptStatus — the prior state before `MoveToCompleted` is `committing`.
- `pkg/processor/prompt_manager.go` — `PromptManager` interface (the one this prompt extends).
- `pkg/processor/workflow_executor_direct.go` — `completeCommit` (current order: commit work → move → commit move).
- `pkg/processor/workflow_executor_branch.go` — `Complete` (currently calls `moveToCompletedAndCommit` then `handleDirectWorkflow`).
- `pkg/processor/workflow_executor_clone.go` — `Complete` (commit in clone, push, chdir back, then `handleAfterIsolatedCommit`).
- `pkg/processor/workflow_executor_worktree.go` — `Complete` (same shape as clone).
- `pkg/processor/workflow_helpers.go` — `moveToCompletedAndCommit` (to delete) and `handleAfterIsolatedCommit` (to simplify — five call sites currently flow into `moveToCompletedAndCommit`; after this change, ZERO).

Coding guides:

- `~/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — use `errors.Wrap` / `errors.Wrapf`, never `fmt.Errorf`.
- `~/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md` — exported method requires a godoc comment.
</context>

<requirements>

### 1. Add `RollbackMoveToCompleted` to `pkg/prompt/prompt.go`

Add this exported method on `*Manager`:

```go
// RollbackMoveToCompleted is the inverse of MoveToCompleted.
// It moves a prompt file from pm.completedDir back to pm.inProgressDir
// and restores its frontmatter status to CommittingPromptStatus
// (the state the prompt was in immediately before MoveToCompleted ran).
// Used by workflow executors when the work commit fails after the move.
func (pm *Manager) RollbackMoveToCompleted(ctx context.Context, completedPath string, mover FileMover) error
```

Behaviour:
1. Compute `originalPath := filepath.Join(pm.inProgressDir, filepath.Base(completedPath))`.
2. Load the prompt at `completedPath`, set its frontmatter status to `CommittingPromptStatus`, save.
3. Use `mover.Rename(ctx, completedPath, originalPath)` to move the file back.
4. Log `slog.InfoContext(ctx, "move-rolled-back-after-commit-failure", "file", filepath.Base(completedPath))`.
5. Wrap any returned error with `errors.Wrap(ctx, err, "rollback move to completed")`.

The signature above is the only signature. Do not introduce `originalParentDir` as a parameter — the `Manager` already owns `inProgressDir`.

### 2. Extend the `PromptManager` interface in `pkg/processor/prompt_manager.go`

Add one method to the interface:

```go
RollbackMoveToCompleted(ctx context.Context, completedPath string, mover FileMover) error
```

Regenerate the counterfeiter mock by running `go generate ./pkg/processor/...` (this is what the project's `make test` does automatically; do not hand-edit `mocks/`).

### 3. Refactor `pkg/processor/workflow_executor_direct.go`

In `completeCommit`, replace the current ordering:

Before:
```
handleDirectWorkflow → MoveToCompleted → CheckAndComplete → CommitCompletedFile → PushBranch
```

After:
```
MoveToCompleted → handleDirectWorkflow → CheckAndComplete → PushBranch
```

If `handleDirectWorkflow` returns an error, call `deps.PromptManager.RollbackMoveToCompleted(ctx, completedPath, mover)` BEFORE returning the wrapped error. Use `errors.Wrap(ctx, err, "handle direct workflow (rolled back move)")`. If rollback itself fails, log the rollback error and still return the original commit error wrapped.

Delete the now-unreachable call to `CommitCompletedFile`. Delete the helper function `CommitCompletedFile` if no other caller references it (grep first).

### 4. Refactor `pkg/processor/workflow_executor_branch.go`

In `Complete`, replace the current sequence (which calls `moveToCompletedAndCommit` then `handleDirectWorkflow`) with:

```
1. MoveToCompleted(ctx, promptPath)             // inside the feature branch's working tree
2. handleDirectWorkflow(...)                    // single combined commit on the feature branch
3. If handleDirectWorkflow fails:
     deps.PromptManager.RollbackMoveToCompleted(ctx, completedPath, mover)  // FIRST
     e.restoreDefaultBranch(ctx)                                            // THEN
     return errors.Wrap(ctx, err, "handle direct workflow on feature branch (rolled back move)")
4. Continue with auto-complete + PR creation as today
```

The order in step 3 is critical: rolling back AFTER `restoreDefaultBranch` would write the file on the default branch, not the feature branch.

Remove the call to `moveToCompletedAndCommit` from this file entirely.

### 5. Refactor `pkg/processor/workflow_executor_clone.go`

In `Complete`, change the in-clone sequence to:

```
1. chdir into clone
2. MoveToCompleted(ctx, promptPath)             // moves prompt within the clone
3. CommitOnly(title)                            // single combined commit
4. Push(branchName)                             // pushes the combined commit
5. chdir back to original
6. remove clone
7. handleAfterIsolatedCommit(...)               // PR creation / auto-merge only — see req 7
```

On any error between steps 2 and 4 (move, commit, or push), do NOT call `RollbackMoveToCompleted`. The entire clone directory is discarded by cleanup in step 6, and the original repo's `prompts/in-progress/<id>.md` was never touched. Return the wrapped error after running cleanup. Document this in a one-line comment in the source: `// no rollback needed: clone is discarded on cleanup; original prompt path untouched`.

### 6. Refactor `pkg/processor/workflow_executor_worktree.go`

Same shape as clone (req 5), substituting "worktree" for "clone" everywhere. The worktree is also discarded on cleanup, so the same no-rollback rule applies. Add the same one-line comment.

### 7. Simplify `handleAfterIsolatedCommit` and delete `moveToCompletedAndCommit` in `pkg/processor/workflow_helpers.go`

After reqs 3-6, every call path moves the prompt BEFORE the work commit. `moveToCompletedAndCommit` and the `ahead == 0` branch of `handleAfterIsolatedCommit` become unreachable.

Concrete changes:

1. Remove every call to `moveToCompletedAndCommit` from `handleAfterIsolatedCommit` (the function currently has five call sites to it across the `ahead == 0`, no-PR, PR-no-automerge, and `handleAutoMergeForClone` paths). Replace each with the post-push work that follows it (PR creation, auto-merge), or with `return nil` for the `ahead == 0` case (no commits = no PR work needed).
2. Delete the function `moveToCompletedAndCommit` entirely. Grep first to confirm no remaining callers.
3. If the `ahead == 0` case now has no meaningful work to do, either delete it or replace it with a defensive `slog.WarnContext(ctx, "after-isolated-commit-no-ahead-commits", ...)` log line and `return nil`. Pick the defensive log path — a zero-ahead state post-fix is unexpected and worth surfacing.

### 8. Add unit test for `RollbackMoveToCompleted` in `pkg/prompt/prompt_test.go`

A Ginkgo test (or table-driven test if that file uses plain testing) named "rolls back a completed move to in-progress with status committing":

Setup:
- Real `Manager` with temp `inProgressDir` and `completedDir`.
- Create a prompt file at `completedDir/123-x.md` with `status: completed`.

Execute:
- Call `mgr.RollbackMoveToCompleted(ctx, completedPath, osFileMover{})` where `osFileMover` is a thin wrapper around `os.Rename` (use the existing one in the prompt tests if present; create a minimal local fake otherwise).

Assert:
- File at `completedPath` does not exist.
- File at `inProgressDir/123-x.md` exists.
- Loaded prompt's frontmatter `status` is `committing`.

### 9. Update `docs/workflows.md`

Find the section that describes the per-mode lifecycle (search for "in-progress" / "completed" / "lifecycle"). Update it to read:

> All workflow modes use the lifecycle **move → stage → commit → push**. The prompt file is renamed from `prompts/in-progress/<id>.md` to `prompts/completed/<id>.md` before the work commit is staged, so a single commit (and a single push) carries both the code change and the rename. If the work commit fails, the rename is rolled back; the on-disk state always matches what HEAD reflects.

Keep any pre-existing mode-specific notes that don't conflict with this rule.

### 10. Add a `CHANGELOG.md` entry

Under the `## Unreleased` section, add a bullet:

```
- fix: prompt move from `in-progress/` to `completed/` is now part of the same commit as the code change, so master no longer diverges from the local daemon view after a PR merge (spec 086, addresses BRO-20203 lib-crypto repro)
```

If no `## Unreleased` section exists, add it above the most recent `## vX.Y.Z` section.

</requirements>

<constraints>
- All four workflow modes (`direct`, `branch`, `clone`, `worktree`) MUST converge on the same lifecycle order: move → stage → commit → push.
- `MoveToCompleted`'s external behaviour MUST remain identical (frontmatter `status: completed`, file at `prompts/completed/<id>.md`).
- `RollbackMoveToCompleted` restores to `inProgressDir` with `status: committing` — the prior PromptStatus before the move. There is no `in_progress` PromptStatus value; do not invent one.
- The `PromptManager` interface gains exactly one new method (`RollbackMoveToCompleted`); no other signature changes.
- `moveToCompletedAndCommit` and the post-commit move path inside `handleAfterIsolatedCommit` are deleted, not commented out or feature-flagged.
- Existing tests in `pkg/processor/` and `pkg/prompt/` MUST continue to pass.
- Errors wrap via `errors.Wrap` / `errors.Wrapf` (project convention). Never `fmt.Errorf`.
- Every new exported symbol (`RollbackMoveToCompleted` on `Manager` and on the interface) gets a godoc comment.
- Do NOT commit — dark-factory handles git.
- Failure of the work commit MUST NOT leave the prompt in an inconsistent on-disk state. Either the file is at `completed/` with a corresponding commit, or it is back at `in-progress/`. Never both, never neither.
</constraints>

<verification>
```bash
make precommit
go test ./pkg/prompt/... -count=1
ginkgo -v ./pkg/processor/
grep -rn "moveToCompletedAndCommit" pkg/    # must return zero matches in non-test code
grep -rn "CommitCompletedFile" pkg/         # must return zero matches if helper was deleted
grep -n "RollbackMoveToCompleted" pkg/prompt/prompt.go pkg/processor/prompt_manager.go  # must return ≥2 matches (impl + interface)
grep -n "move-rolled-back-after-commit-failure" pkg/prompt/prompt.go  # log line literal present
grep -n "move → stage → commit → push" docs/workflows.md  # docs updated
head -20 CHANGELOG.md | grep -i "spec 086\|prompt move\|in-progress.*completed"  # changelog updated
```

`make precommit` must exit 0. Sibling prompt `2-spec-086-tests.md` adds the integration tests that exercise the move-before-commit ordering against real git; this prompt only asserts the rollback unit-test, the dead-code deletions, and the doc/changelog updates.
</verification>
