---
status: completed
spec: [067-bug-branch-workflow-checkout-fails-on-divergent-prompt-file]
summary: Added DiscardUncommittedInPaths to Brancher interface and implementation, called it in setupInPlaceBranch before branch switch, regenerated mocks, added unit tests for both the git implementation and the branch workflow executor, updated docs/workflows.md, and added a CHANGELOG entry.
container: dark-factory-373-spec-067-discard-prompt-dirt-before-branch-switch
dark-factory-version: dev
created: "2026-05-04T20:15:00Z"
queued: "2026-05-04T20:21:25Z"
started: "2026-05-04T20:21:27Z"
completed: "2026-05-04T20:31:24Z"
branch: dark-factory/bug-branch-workflow-checkout-fails-on-divergent-prompt-file
---

<summary>
- The `Brancher` interface gains a new method `DiscardUncommittedInPaths` that restores a given list of path prefixes to their HEAD state before a branch switch
- The `branch` workflow's `setupInPlaceBranch` calls this new method after the cleanliness gate (IsCleanIgnoring) passes and before `Brancher.Switch`, so git checkout no longer rejects the switch when the prompt file has divergent content on the target branch
- Dark-factory's own bookkeeping dirt (prompt frontmatter rewrites) is silently discarded from the master working tree before checkout; the runtime state (retry count, status) is preserved in memory and re-written to disk on the feature branch by the existing `pf.Save` call after Setup returns
- A non-existent or unchanged prefix is a no-op — the method returns nil without touching git
- An empty `IgnorePathPrefixes` slice produces a no-op and leaves the original `Brancher.Switch` failure path intact
- The negative-control from spec 066 is fully preserved: a dirty file outside the prompt directories still causes `IsCleanIgnoring` to abort Setup before `DiscardUncommittedInPaths` is ever called
- The Counterfeiter mock for `Brancher` is regenerated to include the new method
- `docs/workflows.md` `branch` section gains one sentence documenting the discard-before-switch behaviour
- Unit tests cover: `DiscardUncommittedInPaths` with a real git repo (restores dirty files, skips clean paths, no-ops on empty prefix list) and `setupInPlaceBranch` with the mocked `Brancher` (discard called before Switch on the existing-branch path, not called on the create-and-switch path)
</summary>

<objective>
Fix the `branch` workflow retry crash at `Brancher.Switch` that was left unresolved after spec 066. When a prompt is retried against a pre-existing feature branch, the prompt file's master-side frontmatter (rewritten by the daemon) diverges from the feature branch's snapshot, causing `git checkout <branch>` to refuse with "local changes would be overwritten". The fix discards the bookkeeping dirt from the master tree immediately before the switch, using the same `IgnorePathPrefixes` list that 066 introduced for `IsCleanIgnoring`, so the runtime state is not permanently lost — it survives in memory and is re-saved to the feature branch by the existing `pf.Save` call after Setup returns.
</objective>

<context>
Read `CLAUDE.md` for project conventions (error wrapping, Ginkgo/Gomega, Counterfeiter).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read in full before editing:
- `pkg/git/brancher.go` — `Brancher` interface (line 20), `IsCleanIgnoring` implementation (line 238) for style reference, `Switch` method (line 102) for the exact call site context
- `pkg/processor/workflow_executor_branch.go` — full file; focus on `setupInPlaceBranch` (line 52): `IsCleanIgnoring` call, `FetchAndVerifyBranch`/`Switch`/`CreateAndSwitch` branching at lines 72–80
- `pkg/processor/workflow_executor.go` — `WorkflowDeps` struct (line 70): the `IgnorePathPrefixes []string` field added by spec 066
- `mocks/brancher.go` — understand the counterfeiter-generated shape; will be regenerated after the interface change
- `pkg/git/brancher_test.go` — existing test setup pattern (real git repo, BeforeEach/AfterEach, chdir idiom) to follow when adding `DiscardUncommittedInPaths` tests
- `docs/workflows.md` — `branch` subsection (the line that begins "**Working-tree cleanliness check:**") for the exact sentence to extend
</context>

<requirements>

## 1. Add `DiscardUncommittedInPaths` to the `Brancher` interface

In `pkg/git/brancher.go`, add the new method to the `Brancher` interface immediately after `IsCleanIgnoring`:

```go
// DiscardUncommittedInPaths restores each path prefix to its HEAD state using
// git checkout HEAD -- <prefix>. Prefixes not covered by any uncommitted change
// are silently skipped. An empty prefixes slice is a no-op.
// This is called by setupInPlaceBranch immediately before Brancher.Switch so
// that dark-factory's own bookkeeping dirt does not prevent a branch switch
// when the target branch has divergent content for those paths.
DiscardUncommittedInPaths(ctx context.Context, prefixes []string) error
```

Place it directly after the `IsCleanIgnoring` method in the interface block (after line 37 in the current file).

## 2. Implement `DiscardUncommittedInPaths` on `*brancher`

Add the implementation to `pkg/git/brancher.go` directly after the `IsCleanIgnoring` implementation (after line 275):

```go
// DiscardUncommittedInPaths restores each path prefix to HEAD state.
// For each non-empty prefix, it runs git checkout HEAD -- <prefix>.
// If git reports that a prefix matched no tracked files ("did not match any
// file(s) known to git"), the error is logged at debug level and skipped —
// it is not a real failure for our purposes (the prefix was already clean).
// Any other git failure is returned wrapped.
func (b *brancher) DiscardUncommittedInPaths(ctx context.Context, prefixes []string) error {
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		var stderr strings.Builder
		// #nosec G204 -- prefix is a dark-factory config-controlled path prefix
		cmd := exec.CommandContext(ctx, "git", "checkout", "HEAD", "--", prefix)
		// Force English locale so the "did not match any file" probe below is
		// locale-stable. Without this, a French/German/etc. system git would
		// produce a different error string and we'd misclassify it as a real
		// failure. Append (not replace) so PATH and other env vars survive.
		cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			msg := stderr.String()
			if strings.Contains(msg, "did not match any file") {
				// The prefix has no tracked files to discard — treat as clean.
				slog.Debug("DiscardUncommittedInPaths: prefix matched no tracked files, skipping",
					"prefix", prefix)
				continue
			}
			return errors.Wrapf(ctx, err, "discard uncommitted changes in %q: %s", prefix, msg)
		}
		slog.Debug("DiscardUncommittedInPaths: discarded bookkeeping dirt", "prefix", prefix)
	}
	return nil
}
```

## 3. Call `DiscardUncommittedInPaths` in `setupInPlaceBranch`

In `pkg/processor/workflow_executor_branch.go`, inside `setupInPlaceBranch`, add the discard call **immediately after the `dirtyPaths` check block** (the closing `}` of the `if len(dirtyPaths) > 0 {` block) and **before** the `defaultBranch, err := e.deps.Brancher.DefaultBranch(ctx)` call.

Anchor by code, not by line number — the surrounding lines may shift slightly between binary builds.

The modified `setupInPlaceBranch` body becomes:

```go
func (e *branchWorkflowExecutor) setupInPlaceBranch(ctx context.Context, branch string) error {
	dirtyPaths, err := e.deps.Brancher.IsCleanIgnoring(ctx, e.deps.IgnorePathPrefixes)
	if err != nil {
		return errors.Wrap(ctx, err, "check working tree")
	}
	if len(dirtyPaths) > 0 {
		return errors.Errorf(
			ctx,
			"working tree is not clean; cannot switch to branch %q; uncommitted changes: %s",
			branch,
			strings.Join(dirtyPaths, ", "),
		)
	}

	// Discard any uncommitted changes in dark-factory's own bookkeeping
	// directories before switching branches. These paths are "clean enough"
	// per IsCleanIgnoring (spec 066), but git checkout still refuses to switch
	// if the target branch has divergent content for those exact files.
	// The in-memory PromptFile already holds the runtime state; pf.Save after
	// Setup writes it onto the feature branch where it belongs.
	if err := e.deps.Brancher.DiscardUncommittedInPaths(ctx, e.deps.IgnorePathPrefixes); err != nil {
		return errors.Wrap(ctx, err, "discard bookkeeping dirt before branch switch")
	}

	defaultBranch, err := e.deps.Brancher.DefaultBranch(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get default branch")
	}
	e.inPlaceDefaultBranch = defaultBranch
	e.inPlaceBranch = branch

	if err := e.deps.Brancher.FetchAndVerifyBranch(ctx, branch); err == nil {
		if err := e.deps.Brancher.Switch(ctx, branch); err != nil {
			return errors.Wrap(ctx, err, "switch to existing branch")
		}
	} else {
		if err := e.deps.Brancher.CreateAndSwitch(ctx, branch); err != nil {
			return errors.Wrap(ctx, err, "create and switch to branch")
		}
	}
	slog.Info("switched to branch for in-place execution", "branch", branch)
	return nil
}
```

**Important:** keep every other line in `setupInPlaceBranch` exactly as it is — only insert the new `DiscardUncommittedInPaths` call block.

## 4. Regenerate the `Brancher` mock

After adding the new method to the interface, regenerate the counterfeiter mock:

```bash
cd /workspace && go generate ./pkg/git/...
```

This updates `mocks/brancher.go` to include `DiscardUncommittedInPathsStub`, `discardUncommittedInPathsMutex`, `discardUncommittedInPathsArgsForCall`, etc. Do NOT hand-edit `mocks/brancher.go`.

Run `make test` after regeneration to confirm existing tests still pass.

## 5. Unit tests for `DiscardUncommittedInPaths` in `pkg/git/brancher_test.go`

Add a `Describe("DiscardUncommittedInPaths", ...)` block to `pkg/git/brancher_test.go` following the existing real-git-repo pattern (use the `tempDir` and `b` variables from the outer `BeforeEach`).

**5a. Restores a dirty tracked file to HEAD state**

```
Setup:
  - Write "modified content" to an existing tracked file (e.g. create "subdir/dirty.txt"
    tracked in HEAD, then modify it after setup)

  Concretely: in BeforeEach (or inside the test), create and commit
  "prompts/in-progress/001-test.md" with content "status: queued", then
  modify it to "status: failed" (without committing).

  Call: b.DiscardUncommittedInPaths(ctx, []string{"prompts/"})

  Assert:
  - err is nil
  - file content is back to "status: queued" (HEAD state)
  - git status --porcelain shows no changes for that file
```

**5b. Empty prefixes slice is a no-op — returns nil, leaves dirty file untouched**

```
Setup: same dirty file as 5a.
Call: b.DiscardUncommittedInPaths(ctx, []string{})
Assert:
  - err is nil
  - file is still dirty (git status --porcelain shows the file as modified)
```

**5c. Prefix with no tracked files is silently skipped — returns nil**

The goal of this test is to actually exercise the `strings.Contains(msg, "did not match any file")` branch in the implementation, not just any prefix that happens to be clean.

To force git to emit that exact error, the prefix must (a) not exist on disk, (b) not match any tracked file in HEAD, and (c) not be `.` or any prefix that resolves to existing repo content. A literally non-existent directory name like `"completely-missing-dir-xyz/"` (with a name that cannot have ever been tracked) is what triggers `error: pathspec 'completely-missing-dir-xyz/' did not match any file(s) known to git`.

```
Setup: clean BeforeEach repo (no file at the prefix path)
Call: b.DiscardUncommittedInPaths(ctx, []string{"completely-missing-dir-xyz/"})
Assert:
  - err is nil  (the "did not match" stderr branch is reached AND swallowed)
```

If this assertion fails because the implementation returned an error, that's evidence the locale-stability fallback (LC_ALL=C, LANG=C) didn't take effect or the substring match is wrong — both are bugs to fix in the implementation, not in the test.

**5d. Prefix outside the dirty path is skipped without affecting the dirty file**

```
Setup: dirty file under "prompts/in-progress/001-test.md"
Call: b.DiscardUncommittedInPaths(ctx, []string{"specs/"})
Assert:
  - err is nil
  - "prompts/in-progress/001-test.md" is STILL dirty (specs/ prefix didn't touch it)
```

For tests 5a and 5b, you need a file that is tracked in HEAD and then modified. Create and commit it inside the test (or in a nested BeforeEach) before modifying:

```go
// Create and commit the file first
err := os.MkdirAll(filepath.Join(tempDir, "prompts", "in-progress"), 0750)
Expect(err).NotTo(HaveOccurred())
err = os.WriteFile(filepath.Join(tempDir, "prompts", "in-progress", "001-test.md"),
    []byte("status: queued"), 0600)
Expect(err).NotTo(HaveOccurred())
cmd := exec.Command("git", "add", ".")
cmd.Dir = tempDir
Expect(cmd.Run()).To(Succeed())
cmd = exec.Command("git", "commit", "-m", "add prompt")
cmd.Dir = tempDir
Expect(cmd.Run()).To(Succeed())
// Now dirty it
err = os.WriteFile(filepath.Join(tempDir, "prompts", "in-progress", "001-test.md"),
    []byte("status: failed"), 0600)
Expect(err).NotTo(HaveOccurred())
```

## 6. Unit test for `setupInPlaceBranch` with mock Brancher

Create a new file `pkg/processor/workflow_executor_branch_test.go` in package `processor` (internal test package — same as `workflow_executor_direct_test.go`).

Add a `Describe("branchWorkflowExecutor setupInPlaceBranch", ...)` block.

**6a. Existing branch: `DiscardUncommittedInPaths` is called before `Switch`**

```go
Describe("when FetchAndVerifyBranch succeeds (branch exists)", func() {
    It("calls DiscardUncommittedInPaths before Switch", func() {
        fakeBrancher := &mocks.Brancher{}
        fakeBrancher.IsCleanIgnoringReturns([]string{}, nil) // clean
        fakeBrancher.DefaultBranchReturns("master", nil)
        fakeBrancher.FetchAndVerifyBranchReturns(nil)        // branch exists
        fakeBrancher.SwitchReturns(nil)
        fakeBrancher.DiscardUncommittedInPathsReturns(nil)

        prefixes := []string{"prompts/in-progress/", "prompts/completed/"}
        deps := WorkflowDeps{
            Brancher:           fakeBrancher,
            IgnorePathPrefixes: prefixes,
        }
        executor := &branchWorkflowExecutor{deps: deps}
        err := executor.setupInPlaceBranch(ctx, "dark-factory/test-prompt")
        Expect(err).NotTo(HaveOccurred())

        // DiscardUncommittedInPaths must be called with the prefix list
        Expect(fakeBrancher.DiscardUncommittedInPathsCallCount()).To(Equal(1))
        _, gotPrefixes := fakeBrancher.DiscardUncommittedInPathsArgsForCall(0)
        Expect(gotPrefixes).To(Equal(prefixes))

        // Switch must be called (branch existed)
        Expect(fakeBrancher.SwitchCallCount()).To(Equal(1))
        Expect(fakeBrancher.CreateAndSwitchCallCount()).To(Equal(0))
    })
})
```

**6b. New branch: `DiscardUncommittedInPaths` is still called (discard happens before the CreateAndSwitch path too)**

```go
Describe("when FetchAndVerifyBranch fails (branch does not exist)", func() {
    It("calls DiscardUncommittedInPaths and CreateAndSwitch", func() {
        fakeBrancher := &mocks.Brancher{}
        fakeBrancher.IsCleanIgnoringReturns([]string{}, nil)
        fakeBrancher.DefaultBranchReturns("master", nil)
        fakeBrancher.FetchAndVerifyBranchReturns(stderrors.New("not found"))
        fakeBrancher.CreateAndSwitchReturns(nil)
        fakeBrancher.DiscardUncommittedInPathsReturns(nil)

        deps := WorkflowDeps{
            Brancher:           fakeBrancher,
            IgnorePathPrefixes: []string{"prompts/in-progress/"},
        }
        executor := &branchWorkflowExecutor{deps: deps}
        err := executor.setupInPlaceBranch(ctx, "dark-factory/new-prompt")
        Expect(err).NotTo(HaveOccurred())

        Expect(fakeBrancher.DiscardUncommittedInPathsCallCount()).To(Equal(1))
        Expect(fakeBrancher.SwitchCallCount()).To(Equal(0))
        Expect(fakeBrancher.CreateAndSwitchCallCount()).To(Equal(1))
    })
})
```

**6c. `DiscardUncommittedInPaths` failure aborts Setup**

```go
It("returns error when DiscardUncommittedInPaths fails", func() {
    fakeBrancher := &mocks.Brancher{}
    fakeBrancher.IsCleanIgnoringReturns([]string{}, nil)
    fakeBrancher.DefaultBranchReturns("master", nil)
    fakeBrancher.DiscardUncommittedInPathsReturns(stderrors.New("git error"))

    deps := WorkflowDeps{
        Brancher:           fakeBrancher,
        IgnorePathPrefixes: []string{"prompts/"},
    }
    executor := &branchWorkflowExecutor{deps: deps}
    err := executor.setupInPlaceBranch(ctx, "dark-factory/broken-prompt")
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("discard bookkeeping dirt before branch switch"))

    // Neither Switch nor CreateAndSwitch must be called
    Expect(fakeBrancher.SwitchCallCount()).To(Equal(0))
    Expect(fakeBrancher.CreateAndSwitchCallCount()).To(Equal(0))
})
```

**6d. `IsCleanIgnoring` finds user-source dirt: Setup aborts before `DiscardUncommittedInPaths` is called (066 negative-control)**

```go
It("aborts before discard when IsCleanIgnoring finds user-source dirt", func() {
    fakeBrancher := &mocks.Brancher{}
    fakeBrancher.IsCleanIgnoringReturns([]string{"pkg/handler/list-sprints.go"}, nil)

    deps := WorkflowDeps{
        Brancher:           fakeBrancher,
        IgnorePathPrefixes: []string{"prompts/"},
    }
    executor := &branchWorkflowExecutor{deps: deps}
    err := executor.setupInPlaceBranch(ctx, "dark-factory/some-prompt")
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("working tree is not clean"))

    // Discard must NOT be called — 066's gate runs first
    Expect(fakeBrancher.DiscardUncommittedInPathsCallCount()).To(Equal(0))
})
```

The test file needs these imports:

```go
package processor

import (
    "context"
    stderrors "errors"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/bborbe/dark-factory/mocks"
)
```

Use `stdstderrors.New("not found")` (or whatever sentinel value) for the `FetchAndVerifyBranchReturns` and `DiscardUncommittedInPathsReturns` arguments in test 6b/6c — these are test-only sentinel errors, not production error wrapping, so the standard library is the right choice. Do NOT import `github.com/bborbe/dark-factory/pkg/prompt` — `setupInPlaceBranch` is being tested directly, no `prompt.PromptFile` value is needed.

## 7. Update `docs/workflows.md`

In the `### branch` subsection, append one sentence to the **Working-tree cleanliness check** bullet. The current sentence ends with "Any uncommitted change outside those directories still aborts Setup with an error naming the specific dirty file." Append:

```
 After the check passes, dark-factory discards the bookkeeping dirt (via `git checkout HEAD -- <prefix>` for each configured prefix) so that `git checkout <featureBranch>` does not refuse when the feature branch has divergent content for those same files.
```

The full bullet should read:
```
- **Working-tree cleanliness check:** before switching branches, dark-factory verifies the tree is clean — but ignores uncommitted changes inside the four prompt directories (`inboxDir`, `inProgressDir`, `completedDir`, `logDir`) because those are dark-factory's own bookkeeping writes, not user work. Any uncommitted change outside those directories still aborts Setup with an error naming the specific dirty file. After the check passes, dark-factory discards the bookkeeping dirt (via `git checkout HEAD -- <prefix>` for each configured prefix) so that `git checkout <featureBranch>` does not refuse when the feature branch has divergent content for those same files.
```

## 8. Add CHANGELOG entry

Add a bullet under `## Unreleased` in `CHANGELOG.md`:

```markdown
- fix: `branch` workflow retry no longer crashes at `git checkout` when the feature branch has divergent content for prompt-file paths (discards dark-factory's own bookkeeping dirt before the branch switch)
```

## 9. Run `make test` iteratively

Run `make test` after each meaningful change. Run `make precommit` once at the end.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT regress spec 066's `IsCleanIgnoring` filter — it remains in `setupInPlaceBranch` and must run BEFORE `DiscardUncommittedInPaths`. The negative-control (user-source dirt aborts Setup) must remain intact.
- Do NOT touch `worktree`, `clone`, or `direct` workflow executors — the new discard call is only in `branchWorkflowExecutor.setupInPlaceBranch`.
- Do NOT introduce `git checkout -f` (force discard) — that would clobber legitimate user-source dirt in non-prompt directories. Only `git checkout HEAD -- <prefix>` scoped to the configured prefixes.
- Do NOT reuse `IgnorePathPrefixes` in any way that changes the IsCleanIgnoring semantics — the same list is passed to both methods, but the methods are independent.
- Do NOT hand-edit `mocks/brancher.go` — regenerate it via `go generate ./pkg/git/...`.
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- The `DiscardUncommittedInPaths` implementation must swallow "did not match any file(s) known to git" errors from git (log at debug, continue) and return errors for all other failures.
- `go.mod` / `go.sum` / `vendor/` must not be modified.
- Existing tests must still pass.
- `make precommit` must exit 0.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "DiscardUncommittedInPaths" pkg/git/brancher.go` — two occurrences: one in the interface, one as the method implementation.
2. `grep -n "DiscardUncommittedInPaths" pkg/processor/workflow_executor_branch.go` — one occurrence, between the `dirtyPaths` check and the `FetchAndVerifyBranch` call.
3. `grep -n "DiscardUncommittedInPaths" mocks/brancher.go` — at least three occurrences (stub field, mutex, argsForCall slice) confirming the mock was regenerated.
4. `grep -n "DiscardUncommittedInPaths" pkg/git/brancher_test.go` — at least four occurrences (one per sub-test case).
5. `grep -n "DiscardUncommittedInPaths" pkg/processor/workflow_executor_branch_test.go` — at least four occurrences (one per test case).
6. `grep -A2 "Working-tree cleanliness check" docs/workflows.md` — shows the extended sentence mentioning `git checkout HEAD -- <prefix>`.
7. `grep "branch.*divergent\|discard.*bookkeeping" CHANGELOG.md` — shows the new changelog entry.
</verification>
