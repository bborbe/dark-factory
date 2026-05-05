---
status: approved
spec: [068-bug-clone-workflow-commits-ahead-fails-after-clone-removed]
created: "2026-05-04T21:30:00Z"
queued: "2026-05-05T11:25:00Z"
branch: dark-factory/bug-clone-workflow-commits-ahead-fails-after-clone-removed
---

<summary>
- The clone workflow now completes end-to-end: feature branch pushed to origin, PR opened (when `pr: true`), clone removed, prompt moved to completed
- No more `exit 128` crash in the daemon log at the post-commit step
- The clone executor pushes the feature branch from inside the clone before the clone is removed, so the branch survives past clone deletion
- The parent repo gains a fast helper that fetches the named branch as a local ref before counting commits, so the post-commit step sees a real reference
- Worktree, branch, and direct workflows are unchanged — the new fetch helper silently no-ops when origin does not yet have the branch
- The no-diff guard from spec 372 continues to work — empty diffs still skip push and PR cleanly
- A new scenario assertion proves push-before-remove ordering at the call-order level, not just by counter increments
</summary>

<objective>
Fix `cloneWorkflowExecutor.Complete` so that the feature branch is pushed to origin from INSIDE the clone (before chdir-back and clone-removal), and add a `Brancher.FetchBranch` call in `handleAfterIsolatedCommit` immediately before `CommitsAhead` so that the parent repo has a local ref for the branch and `git rev-list origin/&lt;default&gt;..&lt;branch&gt;` resolves correctly. The root cause is that `Complete` deleted the clone BEFORE the post-commit pipeline ran, so the parent repo had never seen the feature branch and `CommitsAhead` exited 128.
</objective>

<context>
Read `CLAUDE.md` for project conventions (error wrapping, Ginkgo/Gomega, Counterfeiter, no raw `go func()`).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read in full before editing:
- `pkg/git/brancher.go` — `Brancher` interface (lines 21-48), `Push` implementation (~line 95), `FetchAndVerifyBranch` implementation (~line 148) for style reference when adding `FetchBranch`
- `pkg/processor/workflow_executor_clone.go` — `Complete` method (lines 99-127): the mis-ordered commit/chdir/remove/handleAfterIsolatedCommit sequence
- `pkg/processor/workflow_executor_worktree.go` — `Complete` method for comparison: shows the shape that works because worktrees share `.git/`
- `pkg/processor/workflow_helpers.go` — `handleAfterIsolatedCommit` (lines 213-268): `CommitsAhead` at line 224, `Push` at line 232; this is where `FetchBranch` is added before `CommitsAhead`
- `pkg/processor/processor_internal_test.go` — `stubBrancher` struct (~line 390), `makeDeps` (~line 685), and existing clone tests `11e` (~line 832) and `11f` (~line 865) and `11ak` (~line 1657) which need updating; understand the test shape before writing new ones
- `pkg/git/brancher_test.go` — the `BeforeEach` real-git-repo pattern (temp dir, git init, git config, chdir) to follow for new `FetchBranch` tests

Key facts:
- `CommitsAhead` runs `git rev-list --count origin/<default>..<branch>` where `<branch>` is the bare branch name. For this to succeed, `<branch>` must exist as a local `refs/heads/<branch>` in the cwd's git repo.
- For worktree, the parent's `.git/refs/heads/<branch>` is populated by `git worktree add` — so bare `<branch>` always resolves there. For clone, the parent's `.git/` was never touched — so bare `<branch>` does NOT resolve, causing exit 128.
- `FetchBranch` uses `git fetch origin <branch>:<branch>` which creates (or updates) `refs/heads/<branch>` locally from the remote. If origin does not have the branch yet (worktree path, branch path before push), git exits non-zero with "couldn't find remote ref" on stderr — FetchBranch must swallow that specific error and return nil.
- After step A (push from inside clone), the branch IS on origin. FetchBranch creates a local copy in parent. CommitsAhead can then resolve `<branch>`.
- `handleAfterIsolatedCommit`'s `Push` at line 232 is idempotent for the clone path: `git push -u origin <branch>` on an already-pushed branch exits 0 ("Everything up-to-date"). No behavior change for worktree.
- The existing test stubs in `processor_internal_test.go` use a hand-written `stubBrancher` (not the Counterfeiter mock). Add `FetchBranch` to `stubBrancher` there. The Counterfeiter mock at `mocks/brancher.go` is regenerated via `go generate ./pkg/git/...`.
</context>

<requirements>

## 1. Add `FetchBranch` to the `Brancher` interface

In `pkg/git/brancher.go`, add the new method to the `Brancher` interface immediately after `CommitsAhead` (currently the last method):

```go
// FetchBranch fetches the named branch from origin as a local branch
// (git fetch origin <branch>:<branch>). This creates refs/heads/<branch>
// in the current repo so that CommitsAhead's bare-branch ref resolves
// correctly after a clone executor has pushed the branch from a separate
// clone directory.
// If origin does not yet have the branch (e.g., worktree path before push),
// the error is swallowed and nil is returned — the caller can proceed with
// whatever local ref already exists.
FetchBranch(ctx context.Context, branch string) error
```

Place it as the last method in the interface block (after `CommitsAhead`).

## 2. Implement `FetchBranch` on `*brancher`

Add the implementation to `pkg/git/brancher.go` at the end of the file (after `CommitsAhead`):

```go
// FetchBranch fetches the named branch from origin into a local branch of the same name.
// It runs: git fetch origin <branch>:<branch>
// If origin does not have the branch ("couldn't find remote ref"), the error is
// logged at debug level and nil is returned — a missing remote branch is not a
// failure; it simply means there is nothing to fetch yet (e.g. worktree path
// before push). Any other git failure is returned wrapped.
func (b *brancher) FetchBranch(ctx context.Context, branch string) error {
	if err := ValidateBranchName(ctx, branch); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	slog.Debug("fetching branch from origin as local ref", "branch", branch)
	var stderr strings.Builder
	// #nosec G204 -- branch name is derived from prompt filename and sanitized
	cmd := exec.CommandContext(ctx, "git", "fetch", "origin", branch+":"+branch)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if strings.Contains(msg, "couldn't find remote ref") {
			// Branch not on origin yet — nothing to fetch; caller uses existing local ref.
			slog.Debug("FetchBranch: branch not on origin yet, skipping", "branch", branch)
			return nil
		}
		return errors.Wrapf(ctx, err, "fetch branch %q from origin: %s", branch, msg)
	}
	slog.Debug("FetchBranch: fetched branch as local ref", "branch", branch)
	return nil
}
```

## 3. Push from inside the clone BEFORE chdir-back in `cloneWorkflowExecutor.Complete`

In `pkg/processor/workflow_executor_clone.go`, modify `Complete` to push from inside the clone immediately after `CommitOnly` and BEFORE `os.Chdir(e.originalDir)`.

The new `Complete` body must be:

```go
func (e *cloneWorkflowExecutor) Complete(
	gitCtx, ctx context.Context,
	pf *prompt.PromptFile,
	title, promptPath, completedPath string,
) error {
	if err := e.deps.Releaser.CommitOnly(gitCtx, title); err != nil {
		return errors.Wrap(ctx, err, "commit changes")
	}

	// Push from inside the clone while the feature branch is still locally
	// visible. The parent repo has never seen this branch; pushing here ensures
	// it exists on origin before the clone is removed and handleAfterIsolatedCommit
	// runs CommitsAhead against the parent repo.
	if err := e.deps.Brancher.Push(gitCtx, e.branchName); err != nil {
		return errors.Wrap(ctx, err, "push branch from clone")
	}

	if err := os.Chdir(e.originalDir); err != nil {
		return errors.Wrap(ctx, err, "chdir back to original directory")
	}

	if err := e.deps.Cloner.Remove(gitCtx, e.clonePath); err != nil {
		slog.Warn("failed to remove clone", "path", e.clonePath, "error", err)
	}
	e.cleanedUp = true

	return handleAfterIsolatedCommit(
		gitCtx,
		ctx,
		e.deps,
		pf,
		e.branchName,
		title,
		promptPath,
		completedPath,
	)
}
```

**Important:** the only change relative to the existing code is the new `Brancher.Push` call block inserted between `CommitOnly` and `os.Chdir`. Everything else stays exactly as-is.

## 4. Add `FetchBranch` call in `handleAfterIsolatedCommit` before `CommitsAhead`

In `pkg/processor/workflow_helpers.go`, inside `handleAfterIsolatedCommit`, add a `FetchBranch` call immediately BEFORE the `CommitsAhead` call at line 224.

The modified section becomes:

```go
// handleAfterIsolatedCommit handles push + optional PR + prompt lifecycle for clone/worktree.
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
	// Fetch the branch as a local ref so that CommitsAhead can resolve the bare
	// branch name via git rev-list. For the clone workflow the branch was just
	// pushed from inside the clone; for worktree the local ref already exists and
	// this is a fast no-op (or a silent skip if origin does not have it yet).
	if err := deps.Brancher.FetchBranch(gitCtx, branchName); err != nil {
		return errors.Wrap(ctx, err, "fetch branch before commit count")
	}
	ahead, err := deps.Brancher.CommitsAhead(gitCtx, branchName)
	if err != nil {
		return errors.Wrap(ctx, err, "count commits ahead")
	}
	// ... rest of the function unchanged ...
```

**Do not change any other line in `handleAfterIsolatedCommit`.** The rest of the function (the `ahead == 0` guard, the `Push`, the PR creation, autoMerge, autoReview paths) must stay exactly as-is.

## 5. Add `FetchBranch` to `stubBrancher` in `processor_internal_test.go`

In `pkg/processor/processor_internal_test.go`, add the new method to `stubBrancher` (currently the last method added is `DiscardUncommittedInPaths` at ~line 466):

```go
func (s *stubBrancher) FetchBranch(_ context.Context, _ string) error { return nil }
```

Place it after `DiscardUncommittedInPaths` in the stub.

## 6. Update existing clone tests for the new push ordering

The existing tests `11e` and `11f` in `processor_internal_test.go` expect `stubBr.pushCount` to be `1`. After the fix, `cloneWorkflowExecutor.Complete` calls `Push` once (from inside the clone executor — step 3 above), and `handleAfterIsolatedCommit` calls `Push` again (the existing call at line 232, now a no-op in practice but still recorded by the stub). Update both test assertions:

- Test `11e` (`"11e: workflow clone, pr false"`): change `Expect(stubBr.pushCount).To(Equal(1))` → `Expect(stubBr.pushCount).To(Equal(2))`
- Test `11f` (`"11f: workflow clone, pr true"`): change `Expect(stubBr.pushCount).To(Equal(1))` → `Expect(stubBr.pushCount).To(Equal(2))`

Also verify test `11ak` (`"11ak: cloneWorkflowExecutor no-diff success"`): CommitOnly no-ops, then `Push` is called in the executor regardless. Check `stubBr.commitsAhead = 0` still skips the second push in `handleAfterIsolatedCommit`. So `pushCount` should become `1` (executor push) instead of `0`. Update `11ak` from `Expect(stubBr.pushCount).To(Equal(0))` → `Expect(stubBr.pushCount).To(Equal(1))`.

Run `grep -n "pushCount" pkg/processor/processor_internal_test.go` to find ALL clone-related push assertions and update them. Do NOT update worktree-related push assertions.

## 7. Regenerate the `Brancher` mock

After updating the `Brancher` interface, regenerate the counterfeiter mock:

```bash
cd /workspace && go generate ./pkg/git/...
```

This updates `mocks/brancher.go` to include `FetchBranchStub`, `fetchBranchMutex`, `fetchBranchArgsForCall`, etc. Do NOT hand-edit `mocks/brancher.go`.

Run `make test` after regeneration to confirm existing tests still pass with the stub update in step 5.

## 8. Unit tests for `FetchBranch` in `pkg/git/brancher_test.go`

Add a `Describe("FetchBranch", ...)` block to `pkg/git/brancher_test.go` following the existing real-git-repo pattern (use the `tempDir`, `b`, and `ctx` variables from the outer `BeforeEach`).

**8a. Returns nil when origin does not have the branch (swallows "couldn't find remote ref")**

This tests the worktree-path case where origin has no branch yet. The real git binary will emit "couldn't find remote ref" on stderr.

```
Setup: use the single-repo tempDir from BeforeEach (no remote configured,
       or configure a bare remote but don't push the branch).
       The simplest setup: initialize a bare "origin" repo as a second tempDir,
       add it as remote, but do NOT push any feature branch to it.

Call: b.FetchBranch(ctx, "dark-factory/not-yet-pushed")

Assert:
  - err is nil  (the "couldn't find remote ref" branch is reached and swallowed)
```

Concrete setup inside the test:
```go
bareDir := GinkgoT().TempDir()
cmd := exec.Command("git", "init", "--bare", bareDir)
Expect(cmd.Run()).To(Succeed())
cmd = exec.Command("git", "remote", "add", "origin", bareDir)
cmd.Dir = tempDir
Expect(cmd.Run()).To(Succeed())

err := b.FetchBranch(ctx, "dark-factory/not-yet-pushed")
Expect(err).NotTo(HaveOccurred())
```

**8b. Creates a local branch when origin has the branch**

This tests the clone-path case: origin has the branch (pushed from inside the clone), FetchBranch creates it locally in the parent repo.

```go
// Create a bare "origin" with a feature branch
bareDir := GinkgoT().TempDir()
cmd := exec.Command("git", "init", "--bare", bareDir)
Expect(cmd.Run()).To(Succeed())
// Add origin remote
cmd = exec.Command("git", "remote", "add", "origin", bareDir)
cmd.Dir = tempDir
Expect(cmd.Run()).To(Succeed())
// Push the initial commit to origin (sets up master)
cmd = exec.Command("git", "push", "origin", "HEAD:master")
cmd.Dir = tempDir
Expect(cmd.Run()).To(Succeed())
// Create and push a feature branch to origin
cmd = exec.Command("git", "checkout", "-b", "dark-factory/feature-foo")
cmd.Dir = tempDir
Expect(cmd.Run()).To(Succeed())
cmd = exec.Command("git", "push", "origin", "dark-factory/feature-foo")
cmd.Dir = tempDir
Expect(cmd.Run()).To(Succeed())
// Switch back to master so local dark-factory/feature-foo is accessible as
// a non-checked-out branch (simulates parent repo state after clone executor chdir back)
cmd = exec.Command("git", "checkout", "master")
cmd.Dir = tempDir
Expect(cmd.Run()).To(Succeed())
// Delete the local branch to simulate parent-repo state (parent never had it locally)
cmd = exec.Command("git", "branch", "-D", "dark-factory/feature-foo")
cmd.Dir = tempDir
Expect(cmd.Run()).To(Succeed())

// Now FetchBranch should recreate it
err := b.FetchBranch(ctx, "dark-factory/feature-foo")
Expect(err).NotTo(HaveOccurred())

// Verify the local branch now exists
cmd = exec.Command("git", "rev-parse", "--verify", "dark-factory/feature-foo")
cmd.Dir = tempDir
Expect(cmd.Run()).To(Succeed(), "local branch should have been created by FetchBranch")
```

**8c. Invalid branch name returns error**

```go
err := b.FetchBranch(ctx, "--injected-flag")
Expect(err).To(HaveOccurred())
Expect(err.Error()).To(ContainSubstring("validate branch name"))
```

## 9. New unit test for push-before-remove ordering in clone executor

The original bug was an ordering bug: `Cloner.Remove` ran BEFORE the feature branch was pushed to origin, so the parent repo could never resolve the branch in `CommitsAhead`. A counter-only assertion (`pushCount == 2 && removeCount == 1`) is structurally insufficient — both counters reach those values whether Push runs before or after Remove. The test MUST assert call order, not just call count.

Add a `callOrder []string` field to BOTH `stubBrancher` (record `"Push"` on each `Push` call) and `stubCloner` (record `"Remove"` on each `Remove` call). Use a single shared slice via a closure or a tiny `callRecorder` struct passed into both stubs. Then add the new test:

```go
// 11al: cloneWorkflowExecutor.Complete — Push precedes clone removal
Describe("11al: cloneWorkflowExecutor.Complete — push precedes clone removal", func() {
    It("calls Push before Remove (ordering assertion, not just counts)", func() {
        recorder := &callRecorder{}
        stubBr.recorder = recorder
        stubCl.recorder = recorder

        stubBr.commitsAhead = 1
        deps := makeDeps(false) // pr=false; focus is on push ordering
        rawExec, ok := NewCloneWorkflowExecutor(deps).(*cloneWorkflowExecutor)
        Expect(ok).To(BeTrue())
        pf := newPromptFile("feature/clone-push-order")

        cloneDir := GinkgoT().TempDir()
        originalDir, err := os.Getwd()
        Expect(err).NotTo(HaveOccurred())
        DeferCleanup(func() { _ = os.Chdir(originalDir) })

        rawExec.branchName = "feature/clone-push-order"
        rawExec.clonePath = cloneDir
        rawExec.originalDir = originalDir

        Expect(os.Chdir(cloneDir)).To(Succeed())

        err = rawExec.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
        Expect(err).NotTo(HaveOccurred())

        // Counter sanity
        Expect(stubBr.pushCount).To(Equal(2))  // once in Complete, once in handleAfterIsolatedCommit
        Expect(stubCl.removeCount).To(Equal(1))

        // Load-bearing assertion: the FIRST Push must come before the only Remove.
        // Without this, a regression that swaps order would still pass the counter checks.
        firstPushIdx := -1
        removeIdx := -1
        for i, op := range recorder.ops {
            if op == "Push" && firstPushIdx == -1 {
                firstPushIdx = i
            }
            if op == "Remove" {
                removeIdx = i
            }
        }
        Expect(firstPushIdx).To(BeNumerically(">=", 0), "Push should have been called")
        Expect(removeIdx).To(BeNumerically(">=", 0), "Remove should have been called")
        Expect(firstPushIdx).To(BeNumerically("<", removeIdx),
            "Push must precede Remove — the original bug was Remove-before-Push")
    })
})
```

Where `callRecorder` is a tiny shared struct:

```go
type callRecorder struct {
    mu  sync.Mutex
    ops []string
}

func (r *callRecorder) record(op string) {
    if r == nil {
        return
    }
    r.mu.Lock()
    defer r.mu.Unlock()
    r.ops = append(r.ops, op)
}
```

Add `recorder *callRecorder` field to both `stubBrancher` and `stubCloner`. Call `s.recorder.record("Push")` at the top of `stubBrancher.Push` and `s.recorder.record("Remove")` at the top of `stubCloner.Remove`. The recorder is `nil`-safe so existing tests that don't set it continue to work unchanged.

## 10. Update `docs/workflows.md` clone section

Find the `clone` (or `pr`) subsection in `docs/workflows.md`. Add a sentence to the Complete/post-commit description documenting the push-before-remove invariant:

```
**Push-before-remove:** the clone executor pushes the feature branch to origin from inside
the clone (before `os.Chdir` back to the original repo and before `Cloner.Remove`). This
ensures the branch is reachable on origin when `handleAfterIsolatedCommit` runs in the
parent repo after the clone is gone.
```

Locate the relevant paragraph by searching for "clone" near "Complete" or "push" or "remove" in the file. Add the sentence in the most appropriate place (the description of the clone Complete path or the behavioral notes table).

## 11. Update `CHANGELOG.md`

Add a bullet under `## Unreleased` in `CHANGELOG.md`:

```markdown
- fix: `clone` workflow (`workflow: clone`, legacy `workflow: pr`) now completes end-to-end — feature branch pushed from inside the clone before removal, parent repo fetches the branch ref before `CommitsAhead`, eliminating `exit 128` crash at post-commit step
```

## 12. Run `make test` iteratively

Run `make test` after each meaningful change (after step 5 stub addition, after step 7 mock regen, after each test file edit). Run `make precommit` once at the very end.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT regress `worktree` or `branch` workflow behavior. The new `FetchBranch` call in `handleAfterIsolatedCommit` must silently no-op when origin does not have the branch yet.
- Do NOT modify `handleAfterIsolatedCommit` beyond adding the single `FetchBranch` call before `CommitsAhead`. No conditional "if clone path" logic.
- Do NOT delete or weaken the `ahead == 0` guard at `workflow_helpers.go:228` — it's load-bearing for the no-diff case (spec 372).
- Do NOT use `git fetch origin` (full fetch) in `FetchBranch` — it must be scoped to the named branch (`git fetch origin <branch>:<branch>`).
- Do NOT hand-edit `mocks/brancher.go` — regenerate via `go generate ./pkg/git/...`.
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Do NOT touch `go.mod` / `go.sum` / `vendor/`.
- Existing tests must still pass; the `stubBrancher` update (step 5) and push-count updates (step 6) must be made before running `make test`.
- `make precommit` must exit 0.
- The `FetchBranch` implementation must swallow "couldn't find remote ref" errors from git and return nil. All other git errors must be returned wrapped.
- `make precommit` is necessary but NOT sufficient. Spec 068's load-bearing acceptance criterion is replaying scenario `002-workflow-pr.md` against the dark-factory sandbox at runtime — the original bug passed `make precommit` but failed in the daemon. Do not declare the spec complete on test-only evidence.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "FetchBranch" pkg/git/brancher.go` — two occurrences: one in the interface, one as the method implementation.
2. `grep -n "FetchBranch" pkg/processor/workflow_helpers.go` — one occurrence, immediately before the `CommitsAhead` call.
3. `grep -n "Brancher.Push\|deps.Brancher.Push\|e.deps.Brancher.Push" pkg/processor/workflow_executor_clone.go` — one occurrence, after `CommitOnly` and before `os.Chdir`.
4. `grep -n "FetchBranch" mocks/brancher.go` — at least three occurrences (stub field, mutex, argsForCall slice) confirming the mock was regenerated.
5. `grep -n "FetchBranch" pkg/processor/processor_internal_test.go` — at least one occurrence in `stubBrancher`.
6. `grep -n "FetchBranch" pkg/git/brancher_test.go` — at least three occurrences (one per test case).
7. `grep -n "push-before-remove\|Push-before-remove\|pushed from inside\|push.*before.*remov\|remov.*after.*push" docs/workflows.md` — at least one match confirming the doc update.
8. `grep "clone.*exit 128\|clone.*commits-ahead\|clone.*push.*removal" CHANGELOG.md` — shows the new changelog entry.
9. `cd /workspace && go test ./pkg/git/... ./pkg/processor/...` — all pass.
</verification>
