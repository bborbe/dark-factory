---
status: draft
spec: [086-bug-prompt-move-not-pushed]
created: "2026-05-22T00:00:00Z"
branch: dark-factory/bug-prompt-move-not-pushed
---

<summary>
- All four workflow modes now move the prompt file BEFORE staging the work commit
- The single work commit contains both code changes AND the rename from `prompts/in-progress/` to `prompts/completed/`
- If the work commit fails, the prompt file is rolled back to `prompts/in-progress/` with `status: in_progress`
- No second unpushed "move" commit exists after successful prompt execution
- `RollbackMoveToCompleted` reverses a completed move when the work commit fails
</summary>

<objective>
Fix the prompt move ordering across all four workflow modes (`direct`, `branch`, `clone`, `worktree`) so that the prompt file is moved to `prompts/completed/` BEFORE the work commit, producing a single commit that contains both the code changes and the rename. On work-commit failure, roll the prompt file back to `prompts/in-progress/`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `docs/workflows.md` for workflow lifecycle expectations.
Read `docs/bug-workflow.md` for bug-reproduction methodology.

Key files to read before making changes:
- `pkg/prompt/prompt.go` — `MoveToCompleted` (~line 685), `moveToCompleted` (~line 1067), `Manager` struct
- `pkg/processor/workflow_executor_direct.go` — `completeCommit` method (~line 62), current two-phase pattern
- `pkg/processor/workflow_executor_clone.go` — `Complete` method (~line 99), calls `handleAfterIsolatedCommit`
- `pkg/processor/workflow_executor_worktree.go` — `Complete` method (~line 99), calls `handleAfterIsolatedCommit`
- `pkg/processor/workflow_executor_branch.go` — `Complete` method (~line 100), calls `moveToCompletedAndCommit`
- `pkg/processor/workflow_helpers.go` — `moveToCompletedAndCommit` (~line 55), `handleAfterIsolatedCommit` (~line 226)

The current flow in each executor:
- direct: commit work → move prompt → commit move (two commits)
- branch: work committed via `handleDirectWorkflow` → `moveToCompletedAndCommit` moves then commits separately
- clone/worktree: work committed in clone/worktree → `handleAfterIsolatedCommit` moves then commits separately

Read `go-concurrency-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` for atomic operation patterns.
</context>

<requirements>

### 1. Add `RollbackMoveToCompleted` to `pkg/prompt/prompt.go`

Add a `RollbackMoveToCompleted(ctx context.Context, completedPath string, originalParentDir string, mover FileMover) error` method to the `Manager` struct. It should:
- Compute the original path by joining `originalParentDir` (which is `inProgressDir`) with the filename from `completedPath`
- Load the prompt file at `completedPath`, change its status back to `CommittingPromptStatus` (not `in_progress` — the frontmatter field was `status: committing` when the move happened since `MoveToCompleted` calls `MarkCompleted()` which sets `status: completed`)
- Actually, read carefully: `moveToCompleted` calls `pf.MarkCompleted()` which sets `pf.Frontmatter.Status = string(CompletedPromptStatus)`. So on rollback, the status should be set back to `CommittingPromptStatus` (not `ApprovedPromptStatus` — `CommittingPromptStatus` is the correct state for "container succeeded, awaiting git commit")
- Actually, let me re-read: `pf.MarkCompleted()` sets `Status = "completed"` and `pf.MarkCommitting()` sets `Status = "committing"`. The file was `status: committing` BEFORE `MoveToCompleted` (set by the executor before calling `completeCommit`). `MoveToCompleted` changed it to `status: completed`. So on rollback, restore it to `CommittingPromptStatus`.
- Save the frontmatter back
- Move the file from `completedPath` to the computed original path
- Log: `slog.Info("rolled back move to completed", "file", filepath.Base(completedPath))`

The function signature:
```go
func (pm *Manager) RollbackMoveToCompleted(ctx context.Context, completedPath string, mover FileMover) error
```
Note: `originalParentDir` is just `pm.inProgressDir` internally — no need to pass it.

### 2. Refactor `pkg/processor/workflow_executor_direct.go`

In `completeCommit`, change the order so MoveToCompleted happens BEFORE the work commit:

Current order:
1. `handleDirectWorkflow` (commits work)
2. `MoveToCompleted` (moves prompt)
3. `CheckAndComplete` (spec auto-complete)
4. `CommitCompletedFile` (commits move)
5. `PushBranch` (if autoRelease)

New order:
1. `MoveToCompleted` (moves prompt to completed/)
2. `handleDirectWorkflow` (commits work + completed prompt file together)
3. If `handleDirectWorkflow` fails: call `RollbackMoveToCompleted` on `completedPath`, then return error
4. `CheckAndComplete` (spec auto-complete — runs after successful commit)
5. `PushBranch` (if autoRelease)

The key insight: `handleDirectWorkflow` stages all modified files and commits. Since the prompt file has already been moved to `completed/`, git sees it as a rename (or new file in completed/) and includes it in the commit automatically.

If `handleDirectWorkflow` fails, call `RollbackMoveToCompleted` before returning.

### 3. Refactor `pkg/processor/workflow_executor_clone.go`

In `Complete`, the current flow is:
1. `CommitOnly` (commits work in clone)
2. `Push` (pushes branch from clone)
3. chdir back to original
4. Remove clone
5. `handleAfterIsolatedCommit` (moves prompt, commits move)

New flow:
1. `MoveToCompleted` (moves prompt to completed/) — BEFORE leaving the clone directory
2. `CommitOnly` (commits work + renamed prompt — git sees the rename from clone's perspective)
3. `Push` (pushes the single combined commit)
4. chdir back to original
5. Remove clone
6. `handleAfterIsolatedCommit` — but now it should NOT call `moveToCompletedAndCommit`; instead it should just do the push-related work (PR creation, auto-merge) since the move already happened

Actually, let me re-read the clone flow more carefully. The clone workflow does work INSIDE the clone directory. So `MoveToCompleted` would move the file within the clone, then `CommitOnly` would commit the work + the moved prompt. Then `Push` pushes that single commit. Then we chdir back and remove the clone.

The key change to `handleAfterIsolatedCommit` for clone: it should skip the `moveToCompletedAndCommit` call when called from clone, since the move already happened. But it still needs to handle the PR creation and auto-merge logic.

Wait, let me think again. `handleAfterIsolatedCommit` is called AFTER the clone is removed. It handles the post-commit workflow for clone and worktree. Since the move happened inside the clone before the commit, the single commit pushed to origin contains both work and the rename. `handleAfterIsolatedCommit` should NOT call `moveToCompletedAndCommit` for the clone case.

So for clone `Complete`:
```
1. MoveToCompleted(ctx, promptPath)  // moves file, sets status=completed, physically moves within clone
2. CommitOnly(title)                 // commits work + renamed prompt in clone (single commit)
3. Push(branchName)                  // pushes the single combined commit
4. chdir back to original
5. remove clone
6. handleAfterIsolatedCommit — but modified to NOT call moveToCompletedAndCommit
```

For `handleAfterIsolatedCommit` — when called from clone, the move+commit already happened. The function needs to know whether it's being called after a combined commit or after a separate move. One approach: add a boolean parameter `moveAlreadyCommitted` to `handleAfterIsolatedCommit`.

Actually a cleaner approach: since `handleAfterIsolatedCommit` checks `CommitsAhead == 0` and calls `moveToCompletedAndCommit` only in that case (when there are no new commits). For clone: there ARE new commits (the combined commit). For worktree: same. The function's logic would naturally skip `moveToCompletedAndCommit` if the branch has ahead commits. Let me re-read:

```go
ahead, err := deps.Brancher.CommitsAhead(gitCtx, branchName)
if err != nil { ... }
if ahead == 0 {
    // no new commits — this is the no-PR case, need to move
    return moveToCompletedAndCommit(...)
}
// ahead > 0 — there are commits pushed, move already happened in the clone/worktree
```

So the clone's `Complete` should:
1. MoveToCompleted (within clone, before commit)
2. CommitOnly (single combined commit in clone)
3. Push (push the combined commit)
4. chdir back + remove clone
5. handleAfterIsolatedCommit — the `ahead > 0` path will skip moveToCompletedAndCommit and go straight to push/PR logic

For the worktree `Complete`:
1. MoveToCompleted (within worktree, before commit)
2. CommitOnly (single combined commit in worktree)
3. chdir back + remove worktree
4. handleAfterIsolatedCommit — same as clone, `ahead > 0` path skips moveToCompletedAndCommit

But wait — `CommitOnly` commits files in the CURRENT directory. For clone, the current directory IS the clone. So MoveToCompleted operates on the clone's filesystem. For worktree, same.

One subtlety: `CommitOnly` uses the title and stages all files in the current working tree. After `MoveToCompleted`, the prompt file is at `completed/` within the clone/worktree. So the single commit will contain both the code changes AND the rename.

### 4. Refactor `pkg/processor/workflow_executor_worktree.go`

Identical pattern to clone `Complete`:
```
1. MoveToCompleted(ctx, promptPath)  // within worktree
2. CommitOnly(title)                // single combined commit
3. chdir back to original
4. remove worktree
5. handleAfterIsolatedCommit — ahead > 0 path, skip moveToCompletedAndCommit
```

### 5. Refactor `pkg/processor/workflow_executor_branch.go`

Current flow in `Complete`:
1. `moveToCompletedAndCommit` (moves, commits move)
2. `handleDirectWorkflow` (commits work on feature branch) — but wait, this is AFTER the move+commit?

Actually looking at `Complete`:
```go
if err := moveToCompletedAndCommit(ctx, gitCtx, e.deps, pf, promptPath, completedPath); err != nil {
    e.restoreDefaultBranch(ctx)
    return errors.Wrap(ctx, err, "move to completed and commit")
}

if err := handleDirectWorkflow(gitCtx, ctx, e.deps, title, featureBranch); err != nil {
    e.restoreDefaultBranch(ctx)
    return errors.Wrap(ctx, err, "handle direct workflow")
}
```

So currently: move+commit move separately, then `handleDirectWorkflow` commits work. This is wrong in both order and structure.

New flow:
1. `MoveToCompleted(ctx, promptPath)` — move BEFORE any commit
2. `handleDirectWorkflow(gitCtx, ctx, e.deps, title, featureBranch)` — this stages all files and commits. Since the prompt is already at `completed/`, git sees it as a rename and includes it in the commit.
3. If `handleDirectWorkflow` fails: call `RollbackMoveToCompleted(ctx, completedPath)` then restore default branch
4. Auto-complete specs
5. Rest of the PR/merge logic

Remove the call to `moveToCompletedAndCommit`. Replace with `MoveToCompleted` + error rollback.

### 6. Update `handleAfterIsolatedCommit` in `pkg/processor/workflow_helpers.go`

The function needs a new parameter `moveAlreadyCommitted bool` to skip `moveToCompletedAndCommit` when the clone/worktree already performed the combined commit.

```go
func handleAfterIsolatedCommit(
    gitCtx context.Context,
    ctx context.Context,
    deps WorkflowDeps,
    pf *prompt.PromptFile,
    branchName string,
    title string,
    promptPath string,
    completedPath string,
    moveAlreadyCommitted bool, // NEW: true for clone/worktree where move happened before commit
) error
```

When `moveAlreadyCommitted == false` AND `ahead == 0`, call `moveToCompletedAndCommit`.
When `moveAlreadyCommitted == true` OR `ahead > 0`, skip the move and go straight to push/PR.

Update all callers:
- `workflow_executor_clone.go`: pass `true` for `moveAlreadyCommitted`
- `workflow_executor_worktree.go`: pass `true` for `moveAlreadyCommitted`

### 7. Add `RollbackMoveToCompleted` to the `Manager` interface

The `Manager` interface (used by `WorkflowDeps.PromptManager`) needs a `RollbackMoveToCompleted` method. Add it:

```go
RollbackMoveToCompleted(ctx context.Context, completedPath string) error
```

The implementation in `prompt.go` uses `pm.inProgressDir` as the rollback destination.

### 8. Error rollback test

After implementing the rollback logic, test it by simulating a `handleDirectWorkflow` failure after `MoveToCompleted` has already happened. The prompt file must end up back at `in-progress/<id>.md` with `status: committing`.

</requirements>

<constraints>
- All four workflow modes (`direct`, `branch`, `clone`, `worktree`) MUST converge on the same lifecycle order: move → stage → commit → push
- `MoveToCompleted` external contract preserved: frontmatter `status: completed`, file at `prompts/completed/`
- `RollbackMoveToCompleted` restores to `in-progress/` with `status: committing`
- Existing tests in `pkg/processor/` and `pkg/prompt/` MUST continue to pass
- Do NOT change the `PromptManager` interface signature beyond adding `RollbackMoveToCompleted`
- Failure of the work commit MUST NOT leave the prompt in an inconsistent on-disk state (no orphan `completed/` file)
- Do NOT commit — dark-factory handles git
- Read `go-context-cancellation-in-loops.md` in `~/.claude/plugins/marketplaces/coding/docs/` — apply context-check patterns if any new loops are introduced
- Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — all errors must use `errors.Wrap` / `errors.Wrapf`, never `fmt.Errorf`
</constraints>

<verification>
```bash
make precommit
```

Additional verification: after the change, manually inspect `workflow_executor_direct.go`, `workflow_executor_clone.go`, `workflow_executor_worktree.go`, and `workflow_executor_branch.go` to confirm the move-before-commit ordering in each.
</verification>
