---
spec: ["001"]
status: completed
summary: Added 17 new test cases to pkg/processor/processor_test.go covering savePRURLToFrontmatter, shouldSkipPrompt, handlePRWorkflow, postMergeActions, handleAutoMerge, and handleDirectWorkflow error paths, raising processor coverage from 81.4% to 90.7%
container: dark-factory-083-processor-test-coverage
dark-factory-version: v0.17.12
created: "2026-03-06T09:19:57Z"
queued: "2026-03-06T09:19:57Z"
started: "2026-03-06T09:19:58Z"
completed: "2026-03-06T09:39:18Z"
---

Increase processor test coverage from 81.4% to 90%+. Focus on untested and low-coverage functions.

## Context

Read `pkg/processor/processor_test.go` and `pkg/processor/processor.go` before making changes.

Current coverage per function (target in parentheses):

| Function | Current | Gap |
|----------|---------|-----|
| savePRURLToFrontmatter | 33.3% | saves PR URL and branch to frontmatter on success; skips on error |
| shouldSkipPrompt | 53.8% | previously-failed prompt skipped silently; validation failure logged; modified prompt retried |
| handleEmptyPrompt | 66.7% | empty body returns early without executing |
| postMergeActions | 66.7% | switch to default branch, pull, auto-release when enabled |
| Process | 68.8% | context cancellation stops loop cleanly |
| cleanupWorktreeOnError | 71.4% | chdir back + remove worktree on error |
| setupWorktreeForExecution | 75.0% | chdir failure after worktree add |
| handleDirectWorkflow | 76.9% | CommitAndRelease error; MinorBump for "add" titles |
| handlePRWorkflow | 78.6% | commit error; push error; PR creation error |
| handleAutoMerge | 80.0% | switch-back failure after merge error (logged but non-fatal) |

## Test cases to add

### WorkflowPR — basic flow (no autoMerge)

**PR: processes prompt and creates PR**
- Config: `WorkflowPR`, `autoMerge: false`
- Mock: brancher.CreateAndSwitch succeeds, executor succeeds, releaser.CommitOnly succeeds, brancher.Push succeeds, prCreator.Create returns `("https://github.com/test/pull/1", nil)`, brancher.Switch succeeds
- Assert: prCreator.CreateCallCount() == 1, first arg is title, prMerger.WaitAndMergeCallCount() == 0, brancher.Switch called with original branch

**PR: commit error stops before push**
- Mock: releaser.CommitOnly returns error
- Assert: brancher.PushCallCount() == 0, prCreator.CreateCallCount() == 0

**PR: push error stops before PR creation**
- Mock: brancher.Push returns error
- Assert: prCreator.CreateCallCount() == 0

**PR: PR creation error marks prompt failed**
- Mock: prCreator.Create returns error
- Assert: prompt status is failed, prMerger.WaitAndMergeCallCount() == 0

### WorkflowPR — with autoMerge

**PR: auto-merge after PR creation**
- Config: `WorkflowPR`, `autoMerge: true`
- Mock: prCreator.Create returns URL, prMerger.WaitAndMerge succeeds, brancher.DefaultBranch returns "master", brancher.Switch succeeds, brancher.Pull succeeds
- Assert: prMerger.WaitAndMergeCallCount() == 1 with correct URL, brancher.Switch called with "master"

**PR: auto-merge failure switches back to original branch**
- Mock: prMerger.WaitAndMerge returns error
- Assert: brancher.Switch called to switch back to original branch, prompt fails

### WorkflowWorktree — basic flow (no autoMerge)

**Worktree: processes prompt and creates PR**
- Config: `WorkflowWorktree`, `autoMerge: false`
- Mock: worktree.Add succeeds, executor succeeds, releaser.CommitOnly succeeds, brancher.Push succeeds, prCreator.Create returns URL, worktree.Remove succeeds
- Assert: worktree.AddCallCount() == 1, prCreator.CreateCallCount() == 1, worktree.RemoveCallCount() == 1, prMerger.WaitAndMergeCallCount() == 0

### savePRURLToFrontmatter

**saves PR URL to frontmatter**
- Write a prompt file, call savePRURLToFrontmatter (or trigger via PR workflow)
- Assert: frontmatter contains `pr-url` and `branch` fields after completion

### shouldSkipPrompt

**skips previously-failed prompt that is unchanged**
- Setup: prompt with status failed, same modtime as last seen
- Assert: returns true (skip)

**retries previously-failed prompt that was modified**
- Setup: prompt with status failed, newer modtime than last seen
- Assert: returns false (process it)

### postMergeActions with autoRelease

**auto-release after merge when changelog exists**
- Config: `autoMerge: true`, `autoRelease: true`
- Mock: prMerger.WaitAndMerge succeeds, releaser.HasChangelog returns true, releaser.CommitAndRelease succeeds
- Assert: releaser.CommitAndReleaseCallCount() == 1

**no release when no changelog**
- Mock: releaser.HasChangelog returns false
- Assert: releaser.CommitAndReleaseCallCount() == 0

## Constraints

- Use Ginkgo v2, match existing test style exactly
- Use existing mocks only — no new dependencies
- Do NOT modify existing tests
- WorkflowWorktree tests need `DeferCleanup` for os.Chdir restoration

## Verification

Run `make precommit` — must pass.
Target: processor coverage >= 90%.
