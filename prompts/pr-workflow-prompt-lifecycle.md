---
status: created
created: "2026-03-09T19:30:00Z"
---

<summary>
- PR workflow keeps prompt lifecycle in the original repo where dark-factory runs
- PRs contain only code changes, no prompt file moves
- Prompt is moved to completed/ in the original repo after PR creation succeeds
- Stale clone directories from crashed runs are automatically cleaned up before cloning
- Clone and branch operations include stderr in error messages for easier debugging
</summary>

<objective>
Fix the PR workflow so that prompt lifecycle (move to completed, status updates, PR URL saving) happens in the original repo, not in the clone. Currently, prompts get stuck at "executing" status because the move-to-completed happens inside the clone which is deleted after PR creation. The dark-factory process runs in the original repo and needs to manage prompt files there.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — the `handlePostExecution` method (around line 424) and `handleCloneWorkflow` method (around line 638).
Read `pkg/git/cloner.go` — the `Clone` method.
Read `pkg/git/cloner_test.go` — existing tests for Clone and Remove.
Read `/home/node/.claude/docs/go-testing.md`.
</context>

<requirements>
1. In `pkg/git/cloner.go`, method `Clone`: before running `git clone`, check if `destDir` already exists using `os.Stat`. If it exists, remove it with `os.RemoveAll` and log a warning with `slog.Warn`. This handles stale clones from previous crashed runs.

2. In `pkg/git/cloner.go`, method `Clone`: capture stderr from all `exec.Command` calls (`git clone`, `git checkout -b`) into a `strings.Builder` via `cmd.Stderr = &stderr`. Include the stderr content in error messages using `errors.Wrapf` with format parameters showing `srcDir`, `destDir`, and `branch` for debugging.

3. In `pkg/git/cloner.go`, method `Clone`: change the first `slog.Debug` call to `slog.Info` so clone operations are visible in normal log output.

4. In `pkg/git/cloner_test.go`: add a test case `"removes stale destDir before cloning"` that creates a non-empty `destDir` (with a file inside), calls `Clone`, and asserts the error does NOT contain "already exists". The clone will still fail (bare repo has no remote), but the error should be about the missing remote, not about the existing directory.

5. In `pkg/processor/processor.go`, method `handlePostExecution`: for the PR workflow path (`p.workflow == config.WorkflowPR`), call `handleCloneWorkflow` BEFORE `moveToCompletedAndCommit`. Pass `pf`, `promptPath`, and `completedPath` as additional parameters to `handleCloneWorkflow`. Remove the `isAutoReviewPR` logic that currently skips the move — that will be handled inside `handleCloneWorkflow`.

6. In `pkg/processor/processor.go`, update `handleCloneWorkflow` signature to accept additional parameters: `pf *prompt.PromptFile`, `promptPath string`, `completedPath string`. The method should:
   a. Commit only code changes in the clone (existing `CommitOnly` call — unchanged)
   b. Push branch (existing `Push` call — unchanged)
   c. Create PR (existing `Create` call — unchanged)
   d. Switch back to original directory (existing `os.Chdir` — unchanged)
   e. Remove clone (existing `cloner.Remove` — unchanged)
   f. After returning to original dir: if `autoMerge`, wait for merge then call `moveToCompletedAndCommit` in original repo
   g. If `autoReview`, save PR URL to prompt frontmatter at `promptPath` (not completedPath) and set status to `in_review`
   h. Default case: call `moveToCompletedAndCommit` in original repo, then save PR URL to `completedPath`

7. Remove the call to `moveToCompletedAndCommit` that currently happens before `handleCloneWorkflow` in `handlePostExecution` for the PR path. For the direct workflow path, keep `moveToCompletedAndCommit` before `handleDirectWorkflow` as-is.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- The `handleDirectWorkflow` path must remain unchanged
- All prompt lifecycle operations (move, status update, PR URL save) must happen in the original repo, never in the clone
- The PR should contain only code changes — no prompt file operations
- Use `errors.Wrapf` (not `errors.Wrap`) when format parameters are needed
- Follow existing code style — Ginkgo/Gomega for tests, counterfeiter for mocks
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
