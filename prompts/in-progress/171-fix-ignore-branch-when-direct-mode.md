---
status: approved
created: "2026-03-11T09:45:00Z"
queued: "2026-03-11T15:34:56Z"
---
<summary>
- When both `pr: false` and `worktree: false`, the `branch` frontmatter field is silently ignored and execution continues on the current branch
- Processor no longer fails with "working tree is not clean" when processing prompts in direct mode that happen to have a `branch` field
- In-place branch switching only activates when at least one of `pr: true` or `worktree: true` is set
- Existing behaviour for `pr: true`, `worktree: true`, and `worktree: true, pr: false` is unchanged
- New test covers the regression: direct-mode prompt with a `branch` field runs without calling `IsClean`, `Switch`, or `CreateAndSwitch`
</summary>

<objective>
Fix `setupWorkflow` so that the `branch` frontmatter field is only used when `pr: true` OR `worktree: true`. When both flags are false, the processor works on the current branch and ignores the `branch` field entirely. Without this fix, the processor calls `setupInPlaceBranchState`, which immediately fails because status-file writes by the processor itself have dirtied the working tree.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making any changes:
- `pkg/processor/processor.go` — focus on `setupWorkflow` method and `processor` struct fields `pr bool` and `worktree bool`
- `pkg/processor/processor_test.go` — existing test patterns and `NewProcessor` call sites
- `pkg/processor/processor_internal_test.go` — internal test helpers
</context>

<requirements>
**Step 1: Fix `setupWorkflow` in `pkg/processor/processor.go`**

Locate the `setupWorkflow` method. Its current body is:

```go
func (p *processor) setupWorkflow(
    ctx context.Context,
    baseName string,
    pf *prompt.PromptFile,
) (*workflowState, error) {
    state := &workflowState{}
    if p.worktree {
        return p.setupCloneWorkflowState(ctx, baseName, pf, state)
    }
    // In-place branch switching (when worktree is false and branch is set)
    if branch := pf.Branch(); branch != "" {
        return p.setupInPlaceBranchState(ctx, branch, state)
    }
    return state, nil
}
```

Change it to guard the in-place branch switch behind a check that at least one of `p.pr` or `p.worktree` is true:

```go
func (p *processor) setupWorkflow(
    ctx context.Context,
    baseName string,
    pf *prompt.PromptFile,
) (*workflowState, error) {
    state := &workflowState{}
    if p.worktree {
        return p.setupCloneWorkflowState(ctx, baseName, pf, state)
    }
    // In-place branch switching only applies when pr or worktree is enabled.
    // When both are false we work on the current branch and ignore the branch field.
    if (p.pr || p.worktree) {
        if branch := pf.Branch(); branch != "" {
            return p.setupInPlaceBranchState(ctx, branch, state)
        }
    }
    return state, nil
}
```

Simplify the condition — since `p.worktree` is already handled above (early return), the guard simplifies to just `p.pr`:

```go
func (p *processor) setupWorkflow(
    ctx context.Context,
    baseName string,
    pf *prompt.PromptFile,
) (*workflowState, error) {
    state := &workflowState{}
    if p.worktree {
        return p.setupCloneWorkflowState(ctx, baseName, pf, state)
    }
    // In-place branch switching only applies when pr is enabled.
    // When pr is false (and worktree is false) we work on the current branch and ignore the branch field.
    if p.pr {
        if branch := pf.Branch(); branch != "" {
            return p.setupInPlaceBranchState(ctx, branch, state)
        }
    }
    return state, nil
}
```

Update the inline comment on the outer `if p.worktree` block if it references branch behaviour to keep docs accurate.

**Step 2: Add a regression test in `pkg/processor/processor_test.go`**

Find the existing describe block for `setupWorkflow` or `ProcessQueue` (or create a new context block). Add a test case:

```
Context("when pr=false and worktree=false and prompt has a branch field", func() {
    It("ignores the branch field and does not attempt to switch branches", func() {
        // arrange: create processor with pr=false, worktree=false
        // arrange: create a prompt file that has a non-empty Branch() value
        // act: call setupWorkflow (or processPrompt at the level where setupWorkflow is invoked)
        // assert: mockBrancher.IsCleanCallCount() == 0
        // assert: mockBrancher.SwitchCallCount() == 0
        // assert: mockBrancher.CreateAndSwitchCallCount() == 0
    })
})
```

Because `setupWorkflow` is not exported, test it through `ProcessQueue` with a minimal happy-path stub:
- `mockManager.ListQueuedReturnsOnCall(0, ...)` with one prompt that has a non-empty `branch` field
- `mockManager.ListQueuedReturnsOnCall(1, nil, nil)` (empty second call ends the loop)
- Set `mockBrancher.FetchReturns(nil)`, `mockBrancher.MergeOriginDefaultReturns(nil)` for the sync step at top of `processPrompt`
- `mockManager.LoadReturns(promptFile, nil)` where `promptFile.Branch()` returns `"some-feature-branch"`
- `mockExecutor.ExecuteReturns(nil)`
- `mockReleaser.HasChangelogReturns(false)`
- `mockReleaser.CommitOnlyReturns(nil)` or `CommitCompletedFileReturns(nil)`
- `mockManager.MoveToCompletedReturns(nil)`
- `mockAutoCompleter.CheckAndCompleteReturns(nil)`
- After calling `proc.ProcessQueue(ctx)`:
  - `Expect(mockBrancher.IsCleanCallCount()).To(Equal(0))`
  - `Expect(mockBrancher.SwitchCallCount()).To(Equal(0))`
  - `Expect(mockBrancher.CreateAndSwitchCallCount()).To(Equal(0))`

Look at the existing `ProcessQueue` tests in `pkg/processor/processor_test.go` for the exact `NewProcessor` call signature and mock wiring pattern to follow — replicate that setup exactly.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- `pr=false, worktree=false, branch=""`: identical to current direct workflow — zero behaviour change
- `pr=false, worktree=true, branch="x"`: clone workflow path is unchanged (worktree early-return fires before the pr guard)
- `pr=true, worktree=false, branch="x"`: in-place branch switching still activates — unchanged
- `pr=true, worktree=true, branch="x"`: clone path runs — unchanged
- Do not change `setupInPlaceBranchState`, `restoreDefaultBranch`, or any other method — the fix is one guard condition in `setupWorkflow` only
- Follow existing error wrapping: `errors.Wrap(ctx, err, "message")`
- All existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
```bash
# Confirm the guard uses p.pr
grep -n "p\.pr" pkg/processor/processor.go

# Confirm setupWorkflow no longer calls setupInPlaceBranchState unconditionally
grep -A 15 "func (p \*processor) setupWorkflow" pkg/processor/processor.go

make precommit
```
Must pass with no errors.
</verification>
