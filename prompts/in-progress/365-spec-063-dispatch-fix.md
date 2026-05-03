---
status: committing
spec: ["063"]
summary: Fixed branchWorkflowExecutor.Setup() to always generate a feature branch from baseName when prompt frontmatter has no branch field, matching clone/worktree executor behavior, and added a new Ginkgo test covering the no-branch-in-frontmatter path.
container: dark-factory-365-spec-063-dispatch-fix
dark-factory-version: v0.145.1-3-g93401a1
created: "2026-05-03T12:00:00Z"
queued: "2026-05-03T11:27:21Z"
started: "2026-05-03T11:37:58Z"
---

<summary>
- The branch workflow executor always creates a feature branch even when the prompt's frontmatter has no `branch:` field
- Branch name is derived from the prompt's baseName using the same `"dark-factory/" + baseName` convention already used by the clone and worktree executors
- The generated branch name is stored in the prompt frontmatter (via `SetBranchIfEmpty`) before execution begins, so it survives daemon restarts and is available for resume
- After the fix, `workflow: branch + pr: true + autoRelease: true` no longer commits directly to master — the separation step (branch creation) always runs before any delivery flag is evaluated
- With `workflow: branch + pr: true + autoMerge: false`, a feature branch is created, commits land on it, and the daemon stops at "await manual merge" — no tag is created on master
- The `workflow: direct + autoRelease: true` regression path is unaffected (directWorkflowExecutor has no branch logic)
- New Ginkgo unit test verifies that when a prompt has no `branch:` frontmatter field, the branch executor generates a branch name and calls `CreateAndSwitch`
- `CHANGELOG.md` gains an entry in the existing `## Unreleased` section
</summary>

<objective>
Fix `branchWorkflowExecutor.Setup()` in `pkg/processor/workflow_executor_branch.go` to always create a feature branch before execution, mirroring the behavior of `cloneWorkflowExecutor` and `worktreeWorkflowExecutor`. Currently, when the prompt has no `branch:` field in its frontmatter, `Setup()` returns nil without creating a branch, causing the subsequent `Complete()` call to treat the workflow as `direct` — which, when `autoRelease=true`, commits and tags directly on master instead of going through the branch+PR path.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Read these files in full before editing:
- `pkg/processor/workflow_executor_branch.go` — full file; the bug is in `Setup()` at ~line 30: when `pf.Branch()` returns empty string, it returns nil without creating a branch
- `pkg/processor/workflow_executor_clone.go` — full file; study how it handles `pf.Branch() == ""` at ~line 43–46 to understand the correct pattern (`e.branchName = "dark-factory/" + string(baseName)`)
- `pkg/processor/workflow_executor_worktree.go` — full file; same pattern at ~line 43–47
- `pkg/processor/workflow_helpers.go` — full file; study `handleDirectWorkflow` at ~line 113 to understand why an empty `featureBranch` arg causes the direct-release path
- `pkg/prompt/prompt.go` — grep for `SetBranchIfEmpty` (~line 521) to confirm the signature: `func (pf *PromptFile) SetBranchIfEmpty(branch string)` — this is the right function to call to persist the generated branch name without overwriting an existing one
- `pkg/processor/processor_branchswitch_test.go` — full file; study the existing test structure, mock setup, and `newProcWithWorkflow` helper to understand how to add a new test
- `pkg/processor/export_test.go` — read to see if `newTestProcessor` or `newProcessorTestPromptFile` are defined here; if so, note their signatures for the new test

The spec this implements: `specs/in-progress/063-bug-autorelease-overrides-pr-workflow.md`
Precondition: prompt `1-spec-063-config-validation.md` has been executed (CHANGELOG `## Unreleased` exists).
</context>

<requirements>

## 1. Fix `Setup()` in `pkg/processor/workflow_executor_branch.go`

The current `Setup()` function has `_ prompt.BaseName` (underscore — baseName is ignored). The fix is to USE `baseName` when `pf.Branch()` is empty, exactly like the clone/worktree executors.

Replace:

```go
// Setup syncs with remote, then optionally switches to the feature branch from the prompt frontmatter.
func (e *branchWorkflowExecutor) Setup(
	ctx context.Context,
	_ prompt.BaseName,
	pf *prompt.PromptFile,
) error {
	if err := syncWithRemoteViaDeps(ctx, e.deps); err != nil {
		return err
	}
	branch := pf.Branch()
	if branch == "" {
		// No branch specified — run directly on current branch.
		return nil
	}
	return e.setupInPlaceBranch(ctx, branch)
}
```

With:

```go
// Setup syncs with remote, then switches to the feature branch.
// When the prompt frontmatter has no branch field, a branch name is derived
// from baseName using the same convention as the clone and worktree executors.
func (e *branchWorkflowExecutor) Setup(
	ctx context.Context,
	baseName prompt.BaseName,
	pf *prompt.PromptFile,
) error {
	if err := syncWithRemoteViaDeps(ctx, e.deps); err != nil {
		return err
	}
	branch := pf.Branch()
	if branch == "" {
		branch = "dark-factory/" + string(baseName)
		// Persist the generated branch into the in-memory PromptFile so the
		// caller's subsequent pf.Save() writes it to disk for resume support.
		pf.SetBranchIfEmpty(branch)
	}
	return e.setupInPlaceBranch(ctx, branch)
}
```

**Key invariant preserved:** `pf.SetBranchIfEmpty` does not overwrite an existing branch value (its implementation is guard-checked). If the prompt already has a branch in the frontmatter, this call is a no-op.

**Why the caller's pf.Save() is sufficient:** After `Setup()` returns, the processor calls `pf.PrepareForExecution(containerName, version)` and then `pf.Save(ctx)`. That single save persists both the container metadata and the now-set branch field together. No additional `pf.Save()` call is needed inside `Setup()`.

## 2. Update the `Setup()` doc comment on the interface in `pkg/processor/workflow_executor.go`

Find the `Setup` method doc comment in the `WorkflowExecutor` interface:

```go
// Setup prepares the execution environment before the YOLO container runs.
// For clone/worktree: creates the isolated directory and chdirs into it.
// For branch: creates or switches to the feature branch in-place.
// For direct: no-op.
// Returns a wrapped error on failure; partial setup is cleaned up before returning.
Setup(ctx context.Context, baseName prompt.BaseName, pf *prompt.PromptFile) error
```

The comment for `branch` already says "creates or switches to the feature branch in-place" — this is correct after the fix. No change needed.

## 3. Add test to `pkg/processor/processor_branchswitch_test.go`

Append the following `It` block inside the existing `Describe("In-place branch switching", func() {` block, after the last existing test case:

```go
It(
    "branch workflow, no branch in frontmatter: generates branch from baseName and calls CreateAndSwitch",
    func() {
        promptPath := filepath.Join(promptsDir, "042-no-branch-in-frontmatter.md")
        queued := []prompt.Prompt{
            {Path: promptPath, Status: prompt.ApprovedPromptStatus},
        }

        // Prompt has NO branch field — the branch executor must generate one.
        manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
            return newProcessorTestPromptFile(path, "# Test\n\nContent"), nil
        }
        manager.ListQueuedReturnsOnCall(0, queued, nil)
        manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
        manager.AllPreviousCompletedReturns(true)
        manager.MoveToCompletedReturns(nil)
        executor.ExecuteReturns(nil)
        releaser.CommitCompletedFileReturns(nil)
        releaser.HasChangelogReturns(false)
        releaser.CommitOnlyReturns(nil)
        autoCompleter.CheckAndCompleteReturns(nil)

        // Branch setup mocks: branch does not exist remotely → CreateAndSwitch is called.
        brancher.IsCleanReturns(true, nil)
        brancher.DefaultBranchReturns("main", nil)
        brancher.FetchAndVerifyBranchReturns(stderrors.New("branch not found on remote"))
        brancher.CreateAndSwitchReturns(nil)
        brancher.SwitchReturns(nil)

        // handleBranchCompletion mocks (pr=false path, last prompt on branch).
        brancher.MergeToDefaultReturns(nil)
        manager.HasQueuedPromptsOnBranchReturns(false, nil)

        p := newProcWithWorkflow(false, config.WorkflowBranch)
        go func() {
            _ = p.Process(ctx)
        }()

        Eventually(func() int {
            return executor.ExecuteCallCount()
        }, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

        // A branch must have been created even though the prompt had no branch field.
        Expect(brancher.CreateAndSwitchCallCount()).To(Equal(1))
        _, createdBranch := brancher.CreateAndSwitchArgsForCall(0)
        // Branch name is derived from the prompt filename (without .md): "042-no-branch-in-frontmatter"
        Expect(createdBranch).To(Equal("dark-factory/042-no-branch-in-frontmatter"))

        cancel()
    },
)
```

**Notes on the test:**
- `newProcessorTestPromptFile` creates a `PromptFile` with `status: approved` and an empty `branch` field — confirm this by reading its definition in `export_test.go` or `processor_test.go`.
- The `brancher.CommitsAheadReturns(1, nil)` is already set in `BeforeEach` (from the outer block) and is NOT overridden here. This allows `handleBranchCompletion` to proceed without crashing.
- `brancher.MergeToDefaultReturns(nil)` and `manager.HasQueuedPromptsOnBranchReturns(false, nil)` are set so that `handleBranchCompletion` (the `pr=false` code path in `Complete()`) runs cleanly.
- The prompt baseName is derived from the filename: `"042-no-branch-in-frontmatter.md"` → `"042-no-branch-in-frontmatter"`.

## 4. Append CHANGELOG entry

In `CHANGELOG.md`, append to the existing `## Unreleased` section (created by prompt 1):

```markdown
- fix: branch workflow always creates feature branch from baseName even when prompt frontmatter has no branch field
```

Do NOT create a duplicate `## Unreleased` section.

## 5. Run `make test`

```bash
cd /workspace && make test
```

All tests must pass before proceeding.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- The `baseName` parameter in `Setup()` must change from `_ prompt.BaseName` to `baseName prompt.BaseName` — the underscore suppresses compiler warnings for unused params; removing it is correct since it is now used
- Use `pf.SetBranchIfEmpty(branch)` (not `pf.SetBranch(branch)`) so that resuming a prompt with an already-set branch is not affected
- Do NOT call `pf.Save(ctx)` inside `Setup()` — the processor's existing `pf.Save()` call (immediately after `Setup()` returns) is sufficient to persist the branch field
- The `workflow: direct` path uses `directWorkflowExecutor`, which has no branch logic — do NOT touch `workflow_executor_direct.go`
- The `workflow_executor_clone.go` and `workflow_executor_worktree.go` files already follow the correct pattern — do NOT change them
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- Existing tests must still pass — do not delete or modify any existing test case
- The test `"worktree=false, branch='': no branch switch called"` uses `config.WorkflowDirect` — it is not affected by this change and must continue to pass
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "baseName prompt.BaseName" pkg/processor/workflow_executor_branch.go` — baseName is no longer `_`; must show the named parameter in `Setup` (and optionally `ReconstructState` if unchanged there)
2. `grep -n "SetBranchIfEmpty\|dark-factory.*baseName" pkg/processor/workflow_executor_branch.go` — `SetBranchIfEmpty` call present in `Setup`
3. `grep -n "42-no-branch-in-frontmatter\|generates branch from baseName" pkg/processor/processor_branchswitch_test.go` — new test present
4. `grep -A2 "## Unreleased" CHANGELOG.md` — shows two entries: config validation fix AND dispatch fix
5. `go test ./pkg/processor/... -v -run "no branch in frontmatter"` — new test passes
</verification>
