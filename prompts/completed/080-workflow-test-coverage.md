---
spec: ["010"]
status: completed
summary: Added WorkflowWorktree auto-merge, WaitAndMerge failure, and autoSetQueuedStatus tests pushing processor coverage from 78.1% to 81.4%
container: dark-factory-080-workflow-test-coverage
dark-factory-version: v0.17.12
created: "2026-03-06T08:30:31Z"
queued: "2026-03-06T08:30:31Z"
started: "2026-03-06T08:36:29Z"
completed: "2026-03-06T08:53:21Z"
---

Add missing processor test cases for `WorkflowPR` and `WorkflowWorktree`. All existing tests use `WorkflowDirect` only.

## Context

Read `pkg/processor/processor_test.go` to understand the existing test structure before adding anything.
Read `pkg/processor/processor.go` to understand what each workflow does.

The mocks `mockPRCreator` (*mocks.PRCreator) and `mockPRMerger` (*mocks.PRMerger) are already wired into `NewProcessor` in every test — they just aren't exercised yet.

## What each workflow does

**WorkflowDirect**: executor runs → `CommitAndRelease` (commit + tag + push on master)

**WorkflowPR**:
1. `brancher.CreateAndSwitch(branchName)`
2. executor runs
3. `releaser.CommitOnly(title)`
4. `brancher.Push(branchName)`
5. `prCreator.Create(title, "Automated by dark-factory")` → prURL
6. If `autoMerge`: `prMerger.WaitAndMerge(prURL)` → switch to default branch → pull
7. If not `autoMerge`: `brancher.Switch(originalBranch)`

**WorkflowWorktree**:
1. `worktree.Add(worktreePath, branchName)` + `os.Chdir(worktreePath)`
2. executor runs
3. `releaser.CommitOnly(title)`
4. `brancher.Push(branchName)`
5. `prCreator.Create(title, "Automated by dark-factory")` → prURL
6. `os.Chdir(originalDir)` + `worktree.Remove(worktreePath)`
7. If `autoMerge`: `prMerger.WaitAndMerge(prURL)` → switch to default branch → pull

## Test cases to add

### WorkflowDirect (verify existing behavior is explicitly tested)

- `[existing]` — keep all existing tests as-is, no changes needed

### WorkflowPR

1. **processes prompt and creates PR** (`autoMerge: false`)
   - Setup: `WorkflowPR`, `autoMerge: false`
   - Mock: `mockBrancher.CreateAndSwitchStub` succeeds, `mockBrancher.SwitchStub` succeeds, `mockExecutor` succeeds, `mockReleaser.CommitOnlyStub` succeeds, `mockBrancher.PushStub` succeeds, `mockPRCreator.CreateStub` returns `("https://github.com/test/repo/pull/1", nil)`
   - Assert: `mockPRCreator.CreateCallCount() == 1`, title arg matches prompt title, `mockPRMerger.WaitAndMergeCallCount() == 0`

2. **processes prompt, creates PR and auto-merges** (`autoMerge: true`)
   - Setup: `WorkflowPR`, `autoMerge: true`
   - Mock: above + `mockPRMerger.WaitAndMergeStub` succeeds, `mockBrancher.DefaultBranchStub` returns `("master", nil)`, `mockBrancher.SwitchStub` succeeds, `mockBrancher.PullStub` succeeds
   - Assert: `mockPRCreator.CreateCallCount() == 1`, `mockPRMerger.WaitAndMergeCallCount() == 1`, WaitAndMerge arg is the prURL returned by Create

3. **PR creation failure marks prompt as failed**
   - Setup: `WorkflowPR`, `mockPRCreator.CreateStub` returns error
   - Assert: prompt status ends as `failed` (or `error`), `mockPRMerger.WaitAndMergeCallCount() == 0`

### WorkflowWorktree

4. **processes prompt and creates PR via worktree** (`autoMerge: false`)
   - Setup: `WorkflowWorktree`, `autoMerge: false`
   - Mock: `mockWorktree.AddStub` succeeds, `mockWorktree.RemoveStub` succeeds, executor succeeds, `mockReleaser.CommitOnlyStub` succeeds, `mockBrancher.PushStub` succeeds, `mockPRCreator.CreateStub` returns `("https://github.com/test/repo/pull/2", nil)`
   - Assert: `mockWorktree.AddCallCount() == 1`, `mockPRCreator.CreateCallCount() == 1`, `mockWorktree.RemoveCallCount() == 1`, `mockPRMerger.WaitAndMergeCallCount() == 0`

5. **processes prompt, creates PR and auto-merges via worktree** (`autoMerge: true`)
   - Setup: `WorkflowWorktree`, `autoMerge: true`
   - Mock: above + `mockPRMerger.WaitAndMergeStub` succeeds, `mockBrancher.DefaultBranchStub` returns `("master", nil)`, `mockBrancher.SwitchStub` succeeds, `mockBrancher.PullStub` succeeds
   - Assert: `mockWorktree.RemoveCallCount() == 1`, `mockPRCreator.CreateCallCount() == 1`, `mockPRMerger.WaitAndMergeCallCount() == 1`

## Constraints

- Use Ginkgo v2 (`It(...)` blocks), same style as existing tests
- Use the existing mocks — do NOT introduce new dependencies
- WorkflowWorktree tests will call `os.Chdir` — use `DeferCleanup` to restore the original dir
- Keep existing tests unchanged

## Verification

Run `make precommit` — must pass with all new tests green.
