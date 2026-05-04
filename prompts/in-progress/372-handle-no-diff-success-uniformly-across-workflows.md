---
status: committing
container: dark-factory-372-handle-no-diff-success-uniformly-across-workflows
dark-factory-version: dev
created: "2026-05-04T15:52:12Z"
queued: "2026-05-04T15:52:12Z"
started: "2026-05-04T16:50:15Z"
---

# Handle "agent reports success but produces no diff" uniformly across all workflows

<summary>
- All four workflow executors (`direct`, `branch`, `worktree`, `clone`) currently crash with `git commit: exit status 1` when the agent reports a successful run that produces no file changes.
- Root cause: `Releaser.CommitOnly` (`pkg/git/git.go:131-145`) and `Releaser.CommitAndRelease` (`pkg/git/git.go:~165-200`) call `git commit` unconditionally, which exits non-zero when there is nothing staged. None of the executors guard against this.
- Prior art exists: the package-level `CommitAll(ctx, message)` (`pkg/git/git.go:63-77`) already uses `git status --porcelain` to detect "nothing to commit" and returns nil gracefully. The fix is to apply the same pattern to `CommitOnly` and `CommitAndRelease`.
- Worktree's `handleAfterIsolatedCommit` already has an `if ahead == 0` guard at `pkg/processor/workflow_helpers.go:228` for the *post-commit* push/PR step. With the Releaser-layer fix in place, that guard is reached cleanly via the no-op commit and correctly skips push/PR.
- Discovered while verifying spec 065 AC 4: a re-run of an already-completed prompt (009) found tests already in place, agent correctly reported `success`, `git commit` then crashed.
</summary>

<objective>
Make `Releaser.CommitOnly` and `Releaser.CommitAndRelease` no-op gracefully when nothing is staged, mirroring the existing `CommitAll` pattern. Add one regression test per workflow asserting the no-op success path. Do not change executor-level code (the existing `ahead == 0` guard handles downstream skipping correctly once the commit step itself stops crashing).
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/dod.md` for the Definition of Done.
Read `docs/workflows.md` for the four workflow executors and their Complete-path responsibilities.
Read `~/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`.
Read `~/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`.

Files to read in full before editing:
- `pkg/git/git.go` — `Releaser` interface, `CommitAll` at line 63-77 (the prior-art no-op pattern to mirror), `CommitOnly` at line 131-145, `CommitAndRelease` (read the file to find the exact lines).
- `pkg/processor/workflow_helpers.go` — `handleDirectWorkflow` at line 113-152, `handleAfterIsolatedCommit` at line 213-268 (existing `ahead == 0` guard at line 228).
- `pkg/processor/workflow_executor_direct.go`, `_branch.go`, `_worktree.go`, `_clone.go` — Complete methods. Read to confirm none of them re-implement the no-op detection (they shouldn't; the fix is at the Releaser layer).

Key facts:
- The agent's `DARK-FACTORY-REPORT.status = "success"` does NOT imply files were modified. A prompt that asks "ensure X is the case" is legitimately successful when X is already the case, with zero diff.
- `git commit` with nothing staged exits 1 with `nothing to commit, working tree clean` on stderr. This is what's currently being wrapped as `"git commit: create commit: exit status 1"`.
- `git commit --allow-empty` is NOT the right answer — empty commits pollute history.
- `CommitAll` already uses `git status --porcelain` after `gitAddAll` to detect emptiness. Use the same approach (not `git diff --cached --quiet`) so all three Releaser methods share one detection technique.
- `github.com/bborbe/errors` re-exports `errors.As` if you need it for boundary-error tests; confirmed via `go doc github.com/bborbe/errors`.

Out of scope:
- Changing the agent's reporting protocol. The agent is allowed to report success with no diff.
- Adding `--allow-empty` anywhere. Empty commits are forbidden.
- Auditing other `gitAddAll` callers (`MoveFile`, etc.) — those have different semantics and are not in this fix's scope.
- Rewriting `handleDirectWorkflow`'s log messages. The "committed changes" log line is slightly inaccurate when the commit was a no-op, but that's a cosmetic cleanup, not a bug — handle separately if at all.
- Changing the public `Releaser` interface signature.
</context>

<requirements>

## 1. Extract a shared "stage and check if anything was staged" helper

The detection pattern in `CommitAll` (`pkg/git/git.go:63-77`) is good but inlined. Extract a package-private helper so all three commit-shaped methods share it:

```go
// stageAllAndCheck stages all changes and reports whether anything was staged.
// Returns (false, nil) when the working tree was already clean — caller should
// treat as a graceful no-op (do NOT call git commit, which would exit 1).
// Returns (true, nil) when at least one path is staged for commit.
// A non-nil error means git itself failed.
func stageAllAndCheck(ctx context.Context) (bool, error) {
    if err := gitAddAll(ctx); err != nil {
        return false, errors.Wrap(ctx, err, "git add")
    }
    // #nosec G204 -- fixed command with no user input
    cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
    output, err := cmd.Output()
    if err != nil {
        return false, errors.Wrap(ctx, err, "git status")
    }
    return len(strings.TrimSpace(string(output))) > 0, nil
}
```

## 2. Apply the helper to `CommitOnly` and `CommitAndRelease` only

### 2a. `CommitOnly` (`pkg/git/git.go:131-145`)

```go
// CommitOnly performs a commit if and only if there are staged changes.
// When the working tree has no staged changes, this returns nil without
// invoking git commit. The downstream CommitsAhead guards in
// handleAfterIsolatedCommit handle the no-commit case correctly.
func (r *releaser) CommitOnly(ctx context.Context, message string) error {
    has, err := stageAllAndCheck(ctx)
    if err != nil {
        return err // already wrapped by stageAllAndCheck
    }
    if !has {
        slog.Info("no staged changes — skipping commit")
        return nil
    }
    if err := gitCommit(ctx, message); err != nil {
        return errors.Wrap(ctx, err, "git commit")
    }
    return nil
}
```

### 2b. `CommitAndRelease` — same pattern

Read `CommitAndRelease` in `pkg/git/git.go` (search for `func (r *releaser) CommitAndRelease`). It currently calls `gitAddAll` then commits + tags + pushes unconditionally. Apply `stageAllAndCheck` at the top:

- If `has == false`: log `"no staged changes — skipping release"` and return nil. Do NOT bump the version, do NOT create a tag (a tag against an unchanged HEAD would be wrong).
- If `has == true`: proceed with the existing commit + tag + push sequence, but replace the leading `gitAddAll` call with the result of `stageAllAndCheck` (don't double-stage).

### 2c. Optionally refactor `CommitAll` to use the helper

`CommitAll` at line 63-77 has the inlined version of this logic. Replacing it with `stageAllAndCheck` is a clean DRY improvement:

```go
func CommitAll(ctx context.Context, message string) error {
    has, err := stageAllAndCheck(ctx)
    if err != nil {
        return err
    }
    if !has {
        return nil
    }
    return gitCommit(ctx, message)
}
```

If this introduces a measurable diff in test output (it shouldn't — behavior is identical), revert it. Refactor only if it cleanly compiles and passes existing tests.

## 3. Tests — Releaser layer

Add tests in `pkg/git/git_test.go`. Use the existing `BeforeEach` temp-repo pattern in that file (real `git` binary, real temp directory). If the file currently has no temp-repo `BeforeEach`, ask: this prompt should NOT introduce a new test pattern; instead, add the new tests to whichever existing test file in `pkg/git/` already uses the real-git temp-repo pattern. Identify it via `grep -l "MkdirTemp\|os.Chdir" pkg/git/*_test.go` before writing.

### 3a. `CommitOnly` no-ops when nothing is staged

```go
It("returns nil and creates no commit when working tree is clean", func() {
    // Setup: clean temp git repo (BeforeEach already established).
    headBefore, _ := exec.Command("git", "-C", tempDir, "rev-parse", "HEAD").CombinedOutput()
    err := r.CommitOnly(ctx, "should be a no-op")
    Expect(err).NotTo(HaveOccurred())
    headAfter, _ := exec.Command("git", "-C", tempDir, "rev-parse", "HEAD").CombinedOutput()
    Expect(string(headAfter)).To(Equal(string(headBefore)))
})
```

### 3b. `CommitOnly` commits when something is staged

Mirror the existing happy-path test (if there is one in the file) or write the symmetric case: write a new file, call `CommitOnly`, assert HEAD advanced.

### 3c. `CommitAndRelease` no-ops when nothing is staged

Same shape as 3a but for `CommitAndRelease`. Assert: HEAD unchanged, no new tag created (`git tag -l` returns the same set as before).

### 3d. Boundary: git command failure propagates as error, not as false

The detection helper returns `(false, err)` when `git status --porcelain` itself fails (e.g. cwd is not a git repo). Verify this with a unit test that runs `stageAllAndCheck` from a non-git directory and asserts the error path:

```go
It("returns an error when git status fails (not a git repo)", func() {
    nonGitDir, err := os.MkdirTemp("", "non-git-*")
    Expect(err).NotTo(HaveOccurred())
    defer os.RemoveAll(nonGitDir)
    Expect(os.Chdir(nonGitDir)).To(Succeed())

    has, err := stageAllAndCheck(ctx)
    Expect(err).To(HaveOccurred())
    Expect(has).To(BeFalse())
})
```

This pins the contract: any non-zero git failure becomes a wrapped error, not a silent `(false, nil)` return.

## 4. Tests — workflow regression tests

For each workflow executor, add or extend a test that asserts the Complete path returns nil and moves the prompt to `completed/` when `Releaser.CommitOnly` is configured to no-op. Use the existing Counterfeiter mock for `Releaser`.

These tests pin the existing behavior of `handleAfterIsolatedCommit:228` (the `ahead == 0` guard) AND the new no-op behavior of `Releaser.CommitOnly` working together. They are regression tests, not tests for new code.

### 4a. `pkg/processor/workflow_executor_direct_test.go`

Mock `Releaser.CommitOnlyReturns(nil)` (the no-op success path). Call `directWorkflowExecutor.Complete`. Assert: returns nil, prompt moved to `completed/`, no tag created (mock `Releaser.CommitAndReleaseCallCount()` is 0 — direct workflow without changelog skips release).

### 4b. `pkg/processor/workflow_executor_branch_test.go`

Same as 4a for `branchWorkflowExecutor.Complete`. Additionally, assert `Brancher.PushCallCount()` is appropriate to the configured `pr` flag (push happens on a feature branch even with no commits — git push is idempotent — but the existing `ahead == 0` skip in `handleAfterIsolatedCommit` may apply here; read the file to confirm).

### 4c. `pkg/processor/workflow_executor_worktree_test.go`

Mock `Releaser.CommitOnlyReturns(nil)` and `Brancher.CommitsAheadReturns(0, nil)`. Call `worktreeWorkflowExecutor.Complete`. Assert: returns nil, worktree removed, prompt moved to `completed/`, no PR created (`PRCreator.CreateCallCount()` is 0).

### 4d. `pkg/processor/workflow_executor_clone_test.go`

Same as 4c for clone. If clone has a different post-commit shape that doesn't go through `handleAfterIsolatedCommit`, document the divergence in the test comment.

If the matching `_test.go` file does not exist for one of the executors, add the regression test inline in `processor_internal_test.go` or wherever the executor is currently tested. Run `grep -rln "directWorkflowExecutor\|branchWorkflowExecutor\|worktreeWorkflowExecutor\|cloneWorkflowExecutor" pkg/processor/*_test.go` before deciding placement.

## 5. Update CHANGELOG

Add a `## Unreleased` entry:

```
- fix: all workflows (direct, branch, worktree, clone) handle "agent reports success but produces no diff" gracefully — no more `git commit: exit status 1` crash; prompt moves to completed/ as expected
```

## 6. Verification

```bash
cd /workspace
make test
make precommit
```

Expected: all existing tests still pass; the new tests in §3 and §4 pass.

</requirements>

<constraints>
- Use `github.com/bborbe/errors` for all error wrapping. Never `fmt.Errorf`, never bare `return err`.
- Do NOT use `git commit --allow-empty`. Empty commits pollute history.
- Do NOT report `failed` from the agent's report — the agent's `success` report is correct in this scenario; the bug is dark-factory's handling.
- Do NOT change the public `Releaser` interface signature. Only the implementation behavior.
- Do NOT change the agent's prompt-assembly suffix (the DARK-FACTORY-REPORT contract). The fix is purely host-side.
- Do NOT delete or weaken the existing `ahead == 0` guard in `handleAfterIsolatedCommit:228`. The new fix is upstream of it; both should coexist.
- Do NOT touch `MoveFile` or `gitAddAll` callers other than `CommitOnly`, `CommitAndRelease`, and (optionally) `CommitAll`. Their semantics differ — out of scope.
- Do NOT introduce a new test pattern (e.g. `git diff --cached --quiet` mocking). Reuse the `git status --porcelain` detection technique that already exists in `CommitAll`.
- Do NOT commit. dark-factory handles git.
- Do NOT touch `go.mod` / `go.sum` / `vendor/`.
- Use `slog` for log lines (matches existing patterns in `pkg/processor/`); use the field-name pattern (`slog.Info("...", "key", value)`).
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "stageAllAndCheck" pkg/git/git.go` — at least 3 occurrences (definition + CommitOnly + CommitAndRelease; possibly 4 if CommitAll was refactored to use it).
2. `grep -n "no staged changes" pkg/git/git.go` — at least one log line per Releaser method that no-ops.
3. `grep -c "no-op\|no staged changes\|no diff success\|no commit\|HEAD unchanged" pkg/git/git_test.go pkg/processor/workflow_executor_*_test.go pkg/processor/processor_internal_test.go` — ≥4 occurrences across the test files (one regression test per workflow, plus boundary).
4. `cd /workspace && go test ./pkg/git/... ./pkg/processor/...` — all pass.

The runtime "agent reports success with no diff" scenario will be exercised post-build by replaying prompt 010 in `~/Documents/workspaces/jira-task-creator` against the rebuilt binary; that runtime gate is operator-driven and not part of this prompt's verification.
</verification>
