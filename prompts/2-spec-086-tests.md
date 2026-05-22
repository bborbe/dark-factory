---
status: draft
spec: [086-bug-prompt-move-not-pushed]
created: "2026-05-22T00:00:00Z"
branch: dark-factory/bug-prompt-move-not-pushed
---

<summary>
- Ginkgo tests verify all four workflow modes produce a single commit containing both code changes AND the prompt rename
- Each test asserts `git log --name-status` output contains both a non-prompt file modification AND `R prompts/in-progress/ prompts/completed/` rename in the SAME commit
- Tests cover the rollback scenario: when work commit fails after move, prompt is restored to `in-progress/` with `status: committing`
- BRO-20203 regression test confirms divergence no longer occurs after a prompt PR merges
</summary>

<objective>
Add Ginkgo tests that verify the move-before-commit fix is correct across all four workflow modes and the failure-mode rollback behavior works. Tests use real git commands (via the existing test infrastructure) to assert that `git log --name-status` on the produced commit shows both code changes and the prompt rename in a single commit.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` for Ginkgo v2 test patterns and coverage rules.
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` — determine appropriate test types (unit vs integration) for these tests.

Key existing test files to reference:
- `pkg/processor/workflow_executor_direct_test.go` — existing test patterns using `osFileMover`, temp directories, and real PromptManager
- `pkg/processor/processor_internal_test.go` — stub patterns (`stubWorkflowReleaser`, `stubWorkflowBrancher`), matrix test patterns

Key source files that were modified in the previous prompt:
- `pkg/processor/workflow_executor_direct.go` — `completeCommit` now does move before commit
- `pkg/processor/workflow_executor_clone.go` — `Complete` does move before commit
- `pkg/processor/workflow_executor_worktree.go` — `Complete` does move before commit
- `pkg/processor/workflow_executor_branch.go` — `Complete` does move before commit
- `pkg/processor/workflow_helpers.go` — `handleAfterIsolatedCommit` takes `moveAlreadyCommitted bool`
- `pkg/prompt/prompt.go` — `Manager.RollbackMoveToCompleted` added

The existing `osFileMover` in `workflow_executor_direct_test.go` uses `os.Rename`. For tests that span git operations, the tests set up a real temporary git repo using the same pattern as `pkg/git/brancher_test.go`.

</context>

<requirements>

### 1. Read existing workflow tests for patterns

Before writing new tests, read:
- `pkg/processor/workflow_executor_direct_test.go` — full file
- `pkg/git/brancher_test.go` — look at how it sets up a temporary git repo with `git init`, remotes, commits (around line 242 `Describe("DefaultBranch", ...)`)

### 2. Add `moveBeforeCommit directWorkflowExecutor` test

Add a new `Describe` block in `pkg/processor/workflow_executor_direct_test.go`:

Test: "single commit contains both code changes and prompt rename when MoveToCompleted is called before CommitOnly"

Setup:
- Create a temp git repo (real `git init`, not a stub)
- Create `prompts/in-progress/` and `prompts/completed/` directories inside the repo
- Write a real prompt file `prompts/in-progress/001-test.md` with `status: committing`
- Create a second file `code.go` in the repo root (the "work" that the agent would produce)
- Build a `directWorkflowExecutor` with real `PromptManager`, stub `Releaser` (that delegates to real git for `CommitOnly`), real `AutoCompleter`
- The stub `Releaser.CommitOnly` should run actual git commands: `git add -A && git commit -m title`

Execute:
- Call `executor.Complete(ctx, ctx, pf, "test commit", promptPath, completedPath)`

Assert:
- `git log --name-status HEAD` output contains:
  - A line with `M` (modified) for `code.go` (or whatever the work file is)
  - A line with `R100 prompts/in-progress/001-test.md prompts/completed/001-test.md` (rename)
- Both lines are in the SAME commit (single `git log` entry)
- `git ls-tree HEAD prompts/in-progress/` does NOT contain `001-test.md`
- `git ls-tree HEAD prompts/completed/` DOES contain `001-test.md`

Hint: the `osFileMover` uses `os.Rename`. For git to see a rename, the file must be in the index before the commit. The `CommitOnly` stub should run real git commands. Since the prompt was already moved to `completed/` before `CommitOnly` is called, git sees the file as a new file in `completed/` and an old file deleted from `in-progress/` — git's rename detection handles this automatically.

### 3. Add `moveBeforeCommit branchWorkflowExecutor` test

Add a new `Describe` block in `pkg/processor/workflow_executor_branch_test.go` (create the file if it doesn't exist):

Test: "single commit contains both code changes and prompt rename"

This test creates a git repo with a default branch, creates a feature branch, writes a prompt file and a code file, calls `Complete`, and verifies the same `git log --name-status` pattern.

### 4. Add `moveBeforeCommit cloneWorkflowExecutor` test

Add to `pkg/processor/workflow_executor_clone_test.go` (create if needed):

Test: "clone workflow single commit contains both code changes and prompt rename"

Setup:
- Create a bare git repo (the "remote")
- Clone it to a temp directory (the "original")
- Set up a `cloneWorkflowExecutor` pointing at this clone
- Write prompt file in the original's `prompts/in-progress/`
- Write a code file in the original

Execute:
- Call `executor.Complete(...)`

Assert: same `git log --name-status` pattern on the pushed commit.

### 5. Add `moveBeforeCommit worktreeWorkflowExecutor` test

Add to `pkg/processor/workflow_executor_worktree_test.go` (create if needed):

Test: "worktree workflow single commit contains both code changes and prompt rename"

Similar to clone test but using worktree operations.

### 6. Add rollback failure-mode test

Test: "when CommitOnly fails after MoveToCompleted, prompt is rolled back to in-progress/ with status committing"

Add to `pkg/processor/workflow_executor_direct_test.go`:

- Set up the same temp git repo with prompt and code file
- Stub `Releaser.CommitOnly` to return an error
- Call `executor.Complete(...)`
- Assert: the error from `Complete` is returned (or nil if the executor retries)
- Assert: the file at `completedPath` does NOT exist
- Assert: the file at `promptPath` DOES exist
- Assert: `promptPath` contains `status: committing` in frontmatter

### 7. Add BRO-20203 regression test

Add a test named `lib-crypto-divergence-bro-20203` (name must contain `bro-20203` or `lib-crypto-divergence` per the acceptance criteria):

Test: "after prompt PR merge, origin/master shows prompt only at completed/ not in-progress/"

This test simulates the original repro:
- Set up a bare remote + local repo with `branch` workflow
- Queue a prompt, execute it, produce a PR
- Merge the PR locally (simulate by checking the commit content)
- Assert: `git ls-tree origin/master prompts/in-progress/ | grep <id>` returns nothing (file not in in-progress on origin)
- Assert: `git ls-tree origin/master prompts/completed/ | grep <id>` returns one line (file in completed on origin)

### 8. Update ginkgo test runner

Run `ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'move.*before.*commit'` to verify at least 4 matching lines (one per workflow mode).

</requirements>

<constraints>
- Tests must use real git operations where possible (not just stubs) to verify the `git log --name-status` output
- Test naming must contain `moveBeforeCommit` or `move.*before.*commit` for the grep-based acceptance criteria: `ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'move.*before.*commit'` must return ≥4 lines
- Rollback test must assert BOTH file path AND frontmatter status after rollback
- Coverage: new code must achieve ≥80% statement coverage per `definition-of-done.md`
- Do NOT commit — dark-factory handles git
- All existing tests must continue to pass
- Do NOT modify `mocks/mocks.go` — use real in-memory fakes or the existing `osFileMover` pattern
</constraints>

<verification>
```bash
make precommit
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'move.*before.*commit'
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'commit fail.*roll(back|ed)'
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'push fail.*after.*move'
go test ./pkg/prompt/... -count=1
```
Each grep must return ≥1 line.
</verification>
