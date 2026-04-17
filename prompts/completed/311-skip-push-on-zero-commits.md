---
status: completed
summary: Added CommitsAhead to Brancher interface and impl, updated handleAfterIsolatedCommit to skip push/PR when zero commits, updated all affected tests
container: dark-factory-311-skip-push-on-zero-commits
dark-factory-version: v0.111.2
created: "2026-04-17T07:45:18Z"
queued: "2026-04-17T07:45:18Z"
started: "2026-04-17T07:45:20Z"
completed: "2026-04-17T07:58:10Z"
---

<summary>
- Clone and worktree workflows skip push and PR when the agent made no code changes
- A new method on the brancher counts how many commits the feature branch is ahead of the default branch
- Post-commit logic checks whether new commits exist before attempting push — zero commits goes straight to move-to-completed
- Prompts that succeed but produce no diff are marked completed instead of failing on push
</summary>

<objective>
Prevent clone/worktree workflows from failing when a prompt succeeds but produces no code changes. Currently `handleAfterIsolatedCommit` unconditionally pushes the branch, which fails when there are zero new commits. Add a commit-count check that skips push/PR and goes straight to move-to-completed.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-error-wrapping.md` in the coding plugin docs directory.
Read `go-testing.md` in the coding plugin docs directory.

Key files:
- `pkg/processor/workflow_helpers.go` — `handleAfterIsolatedCommit` (~line 213)
- `pkg/git/brancher.go` — `Brancher` interface and `brancher` implementation
- `pkg/processor/processor_internal_test.go` — existing test stubs `stubBrancher`, test `11g`
- `mocks/brancher.go` — counterfeiter-generated fake
</context>

<requirements>

## 1. Add `CommitsAhead` to the `Brancher` interface

In `pkg/git/brancher.go`, add a new method to the `Brancher` interface:

```go
// Before (interface block):
MergeToDefault(ctx context.Context, branch string) error

// After:
MergeToDefault(ctx context.Context, branch string) error
CommitsAhead(ctx context.Context, branch string) (int, error)
```

## 2. Implement `CommitsAhead` on `*brancher`

Add the concrete method in `pkg/git/brancher.go`:

```go
// CommitsAhead returns the number of commits on branch ahead of the default branch.
func (b *brancher) CommitsAhead(ctx context.Context, branch string) (int, error) {
	if err := ValidateBranchName(ctx, branch); err != nil {
		return 0, errors.Wrap(ctx, err, "validate branch name")
	}
	defaultBranch, err := b.DefaultBranch(ctx)
	if err != nil {
		return 0, errors.Wrap(ctx, err, "get default branch for commit count")
	}
	// #nosec G204 -- branch name is derived from prompt filename and sanitized
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", "origin/"+defaultBranch+".."+branch)
	output, err := cmd.Output()
	if err != nil {
		return 0, errors.Wrap(ctx, err, "count commits ahead")
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, errors.Wrap(ctx, err, "parse commit count")
	}
	return count, nil
}
```

Add `"strconv"` to the import block in `brancher.go`.

## 3. Regenerate the `Brancher` mock

```bash
go generate ./pkg/git/...
```

Verify the new method appears:
```bash
grep -c "func (f \*Brancher) CommitsAhead" mocks/brancher.go
```

## 4. Update `handleAfterIsolatedCommit` in `pkg/processor/workflow_helpers.go`

Change the function to check commit count before pushing. Find the current code at ~line 224:

```go
// Before:
func handleAfterIsolatedCommit(
	gitCtx context.Context,
	ctx context.Context,
	deps WorkflowDeps,
	pf *prompt.PromptFile,
	branchName string,
	title string,
	promptPath string,
	completedPath string,
) error {
	if err := deps.Brancher.Push(gitCtx, branchName); err != nil {
		return errors.Wrap(ctx, err, "push branch")
	}

// After:
func handleAfterIsolatedCommit(
	gitCtx context.Context,
	ctx context.Context,
	deps WorkflowDeps,
	pf *prompt.PromptFile,
	branchName string,
	title string,
	promptPath string,
	completedPath string,
) error {
	ahead, err := deps.Brancher.CommitsAhead(gitCtx, branchName)
	if err != nil {
		return errors.Wrap(ctx, err, "count commits ahead")
	}
	if ahead == 0 {
		slog.Info("no new commits on branch — skipping push/PR", "branch", branchName)
		return moveToCompletedAndCommit(ctx, gitCtx, deps, pf, promptPath, completedPath)
	}
	if err := deps.Brancher.Push(gitCtx, branchName); err != nil {
		return errors.Wrap(ctx, err, "push branch")
	}
```

The rest of the function (PR creation, auto-merge, auto-review, etc.) remains unchanged.

## 5. Update `stubBrancher` in `pkg/processor/processor_internal_test.go`

Add the `CommitsAhead` method to `stubBrancher` (alongside the existing stub methods around ~line 1510):

```go
// Add field to stubBrancher struct:
commitsAhead    int
commitsAheadErr error

// Add method:
func (s *stubBrancher) CommitsAhead(_ context.Context, _ string) (int, error) {
	return s.commitsAhead, s.commitsAheadErr
}
```

## 6. Update existing test `11g` to set `commitsAhead`

The existing test at `Describe("11g: handleAfterIsolatedCommit — pr false skips PR creation"` expects push to be called. It must now set `commitsAhead: 1` on the stub so the zero-commit guard doesn't short-circuit:

Find the existing test (~line 1967):
```go
// Before:
It("pushes branch and moves to completed without creating a PR", func() {
	deps := makeDeps(false)

// After:
It("pushes branch and moves to completed without creating a PR", func() {
	stubBr.commitsAhead = 1
	deps := makeDeps(false)
```

## 7. Add new test for zero-commits skip

Add a new `Describe` block after test `11g` (before `11h`):

```go
// 11g2: handleAfterIsolatedCommit — zero commits skips push
Describe("11g2: handleAfterIsolatedCommit — zero commits skips push", func() {
	It("skips push and PR, moves directly to completed", func() {
		stubBr.commitsAhead = 0
		deps := makeDeps(true) // pr=true to verify it's also skipped
		pf := newPromptFile("")

		err := handleAfterIsolatedCommit(
			ctx, ctx, deps, pf,
			"feature/no-changes",
			"test title",
			promptPath, completedPath,
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(stubBr.pushCount).To(Equal(0))
		Expect(stubPR.createCount).To(Equal(0))
		Expect(stubMgr.moveToCompletedCount).To(Equal(1))
		Expect(stubRel.commitFileCount).To(Equal(1))
	})
})
```

## 8. Fix tests that call `handleAfterIsolatedCommit` indirectly via `Complete()`

Tests 11e (~line 1904) and 11f (~line 1936) call `cloneWorkflowExecutor.Complete()` which internally calls `handleAfterIsolatedCommit`. Both assert `pushCount == 1`. Without setting `commitsAhead`, the zero-commit guard short-circuits and `pushCount` stays 0, breaking both tests.

Add `stubBr.commitsAhead = 1` before `makeDeps()` in each:

**Test 11e** (~line 1906, `"calls cloner.Remove, brancher.Push, but NOT prCreator.Create"`):
```go
// Before:
It("calls cloner.Remove, brancher.Push, but NOT prCreator.Create", func() {
	deps := makeDeps(false)

// After:
It("calls cloner.Remove, brancher.Push, but NOT prCreator.Create", func() {
	stubBr.commitsAhead = 1
	deps := makeDeps(false)
```

**Test 11f** (~line 1938, `"calls cloner.Remove, brancher.Push, and prCreator.Create"`):
```go
// Before:
It("calls cloner.Remove, brancher.Push, and prCreator.Create", func() {
	deps := makeDeps(true)

// After:
It("calls cloner.Remove, brancher.Push, and prCreator.Create", func() {
	stubBr.commitsAhead = 1
	deps := makeDeps(true)
```

Also grep for any other indirect callers:
```bash
grep -n "\.Complete(" pkg/processor/processor_internal_test.go
```
For each test that asserts `pushCount > 0`, add `stubBr.commitsAhead = 1`.

## 9. Run `make test`

```bash
cd /workspace && make test
```

All tests must pass.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Use `errors.Wrap(ctx, err, ...)` for error wrapping (not `fmt.Errorf`)
- Keep the `CommitsAhead` implementation simple — `git rev-list --count` is sufficient
- Do not change the `Complete()` methods in `workflow_executor_clone.go` or `workflow_executor_worktree.go` — the fix lives entirely in `handleAfterIsolatedCommit`
</constraints>

<verification>
`make test` in `/workspace` must pass.

Additional spot checks:
1. `grep -n "CommitsAhead" pkg/git/brancher.go` — interface + implementation
2. `grep -n "CommitsAhead" mocks/brancher.go` — regenerated fake has the method
3. `grep -n "ahead == 0" pkg/processor/workflow_helpers.go` — zero-commit guard present
4. `grep -n "commitsAhead" pkg/processor/processor_internal_test.go` — stub field + test usage
</verification>
