---
status: completed
spec: ["027"]
summary: 'Verified PR workflow fix: prompt lifecycle operations (move to completed, status updates, PR URL) happen in the original repo, not in the clone; stale clone recovery, better error messages, and correct test coverage all confirmed in place.'
container: dark-factory-152-pr-workflow-prompt-lifecycle
dark-factory-version: dev
created: "2026-03-09T19:30:00Z"
queued: "2026-03-09T20:10:32Z"
started: "2026-03-09T20:10:36Z"
completed: "2026-03-09T20:15:05Z"
---

<summary>
- Prompts no longer get stuck at "executing" after PR creation
- Dark-factory manages prompt status in the project repo, not in temporary clones
- PRs contain only the code changes the agent made — no internal prompt bookkeeping
- Crashed runs no longer block subsequent runs with stale temp directories
- Errors during clone and branch operations show the actual git error message
</summary>

<objective>
Verify and finalize the PR workflow fix so that prompt lifecycle (move to completed, status updates, PR URL saving) happens in the original repo, not in the clone. The implementation is partially done in the working directory — verify it matches the requirements below, fix any gaps or inconsistencies, and ensure all tests pass.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — focus on `handlePostExecution` and `handleCloneWorkflow` methods.
Read `pkg/git/cloner.go` — the `Clone` method.
Read `pkg/git/cloner_test.go` — existing tests for Clone and Remove.
Read `/home/node/.claude/docs/go-testing.md`.

Background: Spec 027 unified the PR workflow. The spec says "worktree" but the implementation uses `git clone` to a temp directory instead. Reason: worktrees share `.git` with the parent repo, which is not accessible inside the YOLO Docker container. A full clone gives a self-contained repo that works when mounted into Docker.

NOTE: These changes are partially implemented in the working directory. For each requirement: read the current code, check if it already satisfies the requirement, and only make changes where gaps or inconsistencies exist. Do not duplicate or conflict with existing implementations.
</context>

<requirements>
1. **Stale clone cleanup** (`pkg/git/cloner.go`, method `Clone`): Before running `git clone`, the method must check if `destDir` already exists with `os.Stat`. If it exists, log with `slog.Warn` and remove with `os.RemoveAll`. Verify this is implemented and handles errors from `os.RemoveAll`.

2. **Better error messages** (`pkg/git/cloner.go`, method `Clone`): The `git clone` and `git checkout -b` commands must capture stderr via `strings.Builder` and include it in error messages using `errors.Wrapf` with `srcDir`, `destDir`, and `branch` context. Verify all git commands in `Clone` have stderr capture.

3. **Visible clone logging** (`pkg/git/cloner.go`, method `Clone`): The initial clone log line must use `slog.Info` (not `slog.Debug`). Verify.

4. **Test stale clone recovery** (`pkg/git/cloner_test.go`): A test named `"removes stale destDir before cloning"` must exist. It should create a non-empty `destDir`, call `Clone`, and assert the error does NOT contain "already exists". Verify the test exists and the assertion is correct.

5. **PR workflow: no prompt ops in clone** (`pkg/processor/processor.go`, method `handlePostExecution`): For the PR workflow path (`p.workflow == config.WorkflowPR`), `handleCloneWorkflow` must be called WITHOUT calling `moveToCompletedAndCommit` first. There must be no `isAutoReviewPR` variable. The direct workflow path must still call `moveToCompletedAndCommit` before `handleDirectWorkflow`. Verify this separation is correct.

6. **PR workflow: prompt lifecycle in original repo** (`pkg/processor/processor.go`, method `handleCloneWorkflow`): After clone removal and `os.Chdir` back to original dir, the method must handle prompt lifecycle in the original repo:
   - `autoMerge` path: wait for merge, then `moveToCompletedAndCommit`, then `postMergeActions`
   - `autoReview` path: save PR URL to frontmatter at `promptPath` (not completedPath), set status to `in_review`
   - Default path: `moveToCompletedAndCommit`, then save PR URL to `completedPath`
   Verify all three paths exist and operate on the correct paths (original repo, not clone).

7. **No regressions**: Run the full test suite. All existing tests must pass. If any test fails due to the changes, fix the test or the implementation to be consistent.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- The `handleDirectWorkflow` path must remain unchanged
- All prompt lifecycle operations (move, status update, PR URL save) must happen in the original repo, never in the clone
- The PR should contain only code changes — no prompt file operations
- Use `errors.Wrapf` (not `errors.Wrap`) when format parameters are needed
- Follow existing code style — Ginkgo/Gomega for tests, counterfeiter for mocks
- Do not rewrite code that already satisfies a requirement — only fix gaps
</constraints>

<verification>
Run `make precommit` -- must pass with exit code 0.
</verification>
