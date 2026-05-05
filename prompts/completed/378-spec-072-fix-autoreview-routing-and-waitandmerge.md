---
status: completed
spec: [072-bug-autoreview-path-unreachable-when-automerge-takes-precedence]
summary: Fixed autoReview routing order in handleAfterIsolatedCommit (AutoReview now checked before AutoMerge) and fixed WaitAndMerge to use correct mergeStateStatus values (CLEAN/DIRTY) via new decideMergeAction helper, with full unit test coverage for both fixes.
container: dark-factory-378-spec-072-fix-autoreview-routing-and-waitandmerge
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-05T21:40:00Z"
queued: "2026-05-05T21:46:39Z"
started: "2026-05-05T21:49:03Z"
completed: "2026-05-05T21:53:52Z"
branch: dark-factory/bug-autoreview-path-unreachable-when-automerge-takes-precedence
---

<summary>
- With `autoReview: true + autoMerge: true` (the only valid combo), the daemon now correctly transitions the prompt to `in_review` status instead of calling `WaitAndMerge` directly
- The daemon log emits `PR created, waiting for review` after PR creation; the review poller can now pick up the prompt
- `WaitAndMerge` is no longer called when `autoReview` is enabled — it only runs after a reviewer approves (via `handleApproved` per spec 071)
- `WaitAndMerge` now recognizes `mergeStateStatus: "CLEAN"` as the merge-ready signal (GitHub's actual value), replacing the incorrect `"MERGEABLE"` check
- `WaitAndMerge` now recognizes `mergeStateStatus: "DIRTY"` as a terminal conflict state and fails fast, replacing the incorrect `"CONFLICTING"` check
- `WaitAndMerge` continues polling on `"BLOCKED"`, `"BEHIND"`, `"UNKNOWN"`, `"UNSTABLE"`, `"HAS_HOOKS"` (existing polling behavior preserved)
- The autoMerge-only path (`autoReview: false`) is not regressed — it still calls `WaitAndMerge` after PR creation
- A new unit test asserts the routing fix: with both `autoMerge=true` and `autoReview=true`, `SetStatus(in_review)` is called and `WaitAndMerge` is not
- New unit tests cover all `mergeStateStatus` decision outcomes via an exported test helper
- CHANGELOG `## Unreleased` entry added
</summary>

<objective>
Fix two related bugs that together make the `autoReview: true + autoMerge: true` configuration completely non-functional: (1) `handleAfterIsolatedCommit` checks `deps.AutoMerge` before `deps.AutoReview`, short-circuiting to the auto-merge path before the autoReview branch can run; (2) `WaitAndMerge` switches on `"MERGEABLE"/"CONFLICTING"` (values of the `mergeable` field) instead of the correct `mergeStateStatus` values `"CLEAN"/"DIRTY"`, causing infinite polling. Both must be fixed together to enable the end-to-end autoReview → human approval → merge flow.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `docs/bug-workflow.md` for verification requirements.
Read `docs/workflows.md` for the autoReview behavioral contract.

Files to read before editing:
- `pkg/processor/workflow_helpers.go` — full file; focus on `handleAfterIsolatedCommit` (lines 229–290), specifically the routing block that checks `deps.AutoMerge` at line 264 before `deps.AutoReview` at line 277
- `pkg/git/pr_merger.go` — full file; focus on `WaitAndMerge` (lines 48–79), the `prStatus` struct (lines 43–46), and the switch at lines 68–76 that incorrectly matches `"MERGEABLE"/"CONFLICTING"` for a `mergeStateStatus` field
- `pkg/config/config.go` lines 461–476 — `validateAutoReview`, which requires `AutoMerge: true` when `AutoReview: true`. Do NOT change this.
- `pkg/processor/processor_automerge_test.go` — the existing "Auto-review" context (lines 608–829) and "Auto-merge" context (lines 83–606); understand the `newTestProcessor` call signature before adding new tests
- `pkg/git/pr_merger_test.go` — the existing test file; understand the package declaration (`package git_test`) before adding tests
- `pkg/processor/export_test.go` — the pattern for exposing internal functions to external test packages
</context>

<requirements>

## 1. Fix routing order in `handleAfterIsolatedCommit`

In `pkg/processor/workflow_helpers.go`, in the `handleAfterIsolatedCommit` function, swap the order of the `AutoReview` and `AutoMerge` checks so that `AutoReview` is evaluated first.

**Current code (lines 264–289):**
```go
if deps.AutoMerge {
    return handleAutoMergeForClone(
        gitCtx,
        ctx,
        deps,
        pf,
        branchName,
        promptPath,
        completedPath,
        prURL,
        title,
    )
}
if deps.AutoReview {
    savePRURLToFrontmatter(gitCtx, deps, promptPath, prURL)
    if err := deps.PromptManager.SetStatus(ctx, promptPath, string(prompt.InReviewPromptStatus)); err != nil {
        return errors.Wrap(ctx, err, "set in_review status")
    }
    slog.Info("PR created, waiting for review", "url", prURL)
    return nil
}
if err := moveToCompletedAndCommit(ctx, gitCtx, deps, pf, promptPath, completedPath); err != nil {
    return errors.Wrap(ctx, err, "move to completed and commit")
}
savePRURLToFrontmatter(gitCtx, deps, completedPath, prURL)
return nil
```

**Replace with (AutoReview checked first):**
```go
if deps.AutoReview {
    // AutoReview takes precedence over AutoMerge: open PR, wait for human approval,
    // then auto-merge after approval (handleApproved in pkg/review/poller.go handles the merge).
    savePRURLToFrontmatter(gitCtx, deps, promptPath, prURL)
    if err := deps.PromptManager.SetStatus(ctx, promptPath, string(prompt.InReviewPromptStatus)); err != nil {
        return errors.Wrap(ctx, err, "set in_review status")
    }
    slog.Info("PR created, waiting for review", "url", prURL)
    return nil
}
if deps.AutoMerge {
    return handleAutoMergeForClone(
        gitCtx,
        ctx,
        deps,
        pf,
        branchName,
        promptPath,
        completedPath,
        prURL,
        title,
    )
}
if err := moveToCompletedAndCommit(ctx, gitCtx, deps, pf, promptPath, completedPath); err != nil {
    return errors.Wrap(ctx, err, "move to completed and commit")
}
savePRURLToFrontmatter(gitCtx, deps, completedPath, prURL)
return nil
```

No other lines in `handleAfterIsolatedCommit` or `workflow_helpers.go` are changed.

## 2. Fix `WaitAndMerge` switch in `pkg/git/pr_merger.go`

### 2a. Extract `decideMergeAction` helper

Add a new package-private function immediately before `WaitAndMerge` in `pkg/git/pr_merger.go`:

```go
// decideMergeAction maps a mergeStateStatus value to an action:
//   (true, nil)  → merge the PR now
//   (false, err) → fatal conflict, abort polling
//   (false, nil) → keep polling (BLOCKED, BEHIND, UNKNOWN, UNSTABLE, HAS_HOOKS)
//
// GitHub mergeStateStatus reference:
//   CLEAN → all checks pass, no conflicts
//   DIRTY → merge conflicts
//   BLOCKED → branch protection blocking merge
//   BEHIND → branch is behind base
//   UNSTABLE → checks are failing
//   HAS_HOOKS → pre-receive hooks pending
//   UNKNOWN → state not yet computed
func decideMergeAction(mergeStateStatus string) (shouldMerge bool, err error) {
    switch mergeStateStatus {
    case "CLEAN":
        return true, nil
    case "DIRTY":
        return false, stderrors.New("PR has conflicts and cannot be merged")
    default:
        return false, nil
    }
}
```

You need to add a standard-library errors alias at the top of the file. Check the existing imports in `pr_merger.go`. Add to the import block:

```go
import (
    stderrors "errors"
    // ... existing imports ...
)
```

If `stderrors` is already aliased in this file, use the existing alias name. If `stderrors` is NOT used elsewhere in the file after this change, keep the import minimal — only add what is needed.

### 2b. Update `WaitAndMerge` to use `decideMergeAction`

Replace the switch block in `WaitAndMerge` (lines 68–76) with a call to `decideMergeAction`:

**Current switch (lines 68–76):**
```go
switch status.MergeStateStatus {
case "MERGEABLE":
    return p.mergePR(ctx, prURL)
case "CONFLICTING":
    return errors.Errorf(ctx, "PR has conflicts and cannot be merged")
default:
    // Continue polling for other states (BLOCKED, UNKNOWN, etc.)
    continue
}
```

**Replace with:**
```go
shouldMerge, mergeErr := decideMergeAction(status.MergeStateStatus)
if mergeErr != nil {
    return errors.Wrap(ctx, mergeErr, "PR not mergeable")
}
if shouldMerge {
    return p.mergePR(ctx, prURL)
}
// BLOCKED, BEHIND, UNKNOWN, UNSTABLE, HAS_HOOKS → continue polling
```

No other lines in `WaitAndMerge` or `pr_merger.go` are changed.

## 3. Create `pkg/git/export_test.go` for testing `decideMergeAction`

Create a new file `pkg/git/export_test.go` (package `git`, NOT `git_test`) that exports the internal `decideMergeAction` for external tests:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

// DecideMergeActionForTest exposes the internal decideMergeAction function for external tests.
func DecideMergeActionForTest(mergeStateStatus string) (shouldMerge bool, err error) {
    return decideMergeAction(mergeStateStatus)
}
```

## 4. Add `decideMergeAction` unit tests to `pkg/git/pr_merger_test.go`

In `pkg/git/pr_merger_test.go`, add a new `Describe("decideMergeAction", ...)` block immediately after the existing `Describe("PRMerger", ...)` block. Call `git.DecideMergeActionForTest` (the exported helper created in step 3):

```go
var _ = Describe("decideMergeAction", func() {
    DescribeTable("maps mergeStateStatus to action",
        func(status string, wantMerge bool, wantErr bool) {
            shouldMerge, err := git.DecideMergeActionForTest(status)
            if wantErr {
                Expect(err).To(HaveOccurred())
                Expect(shouldMerge).To(BeFalse())
            } else {
                Expect(err).NotTo(HaveOccurred())
                Expect(shouldMerge).To(Equal(wantMerge))
            }
        },
        Entry("CLEAN → merge", "CLEAN", true, false),
        Entry("DIRTY → conflict error", "DIRTY", false, true),
        Entry("BLOCKED → keep polling", "BLOCKED", false, false),
        Entry("BEHIND → keep polling", "BEHIND", false, false),
        Entry("UNKNOWN → keep polling", "UNKNOWN", false, false),
        Entry("UNSTABLE → keep polling", "UNSTABLE", false, false),
        Entry("HAS_HOOKS → keep polling", "HAS_HOOKS", false, false),
        Entry("empty string → keep polling", "", false, false),
        Entry("MERGEABLE (old wrong value) → keep polling", "MERGEABLE", false, false),
        Entry("CONFLICTING (old wrong value) → keep polling", "CONFLICTING", false, false),
    )

    It("DIRTY error message mentions conflicts", func() {
        _, err := git.DecideMergeActionForTest("DIRTY")
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("conflict"))
    })
})
```

The last two `Entry` lines (`"MERGEABLE"` and `"CONFLICTING"`) serve as regression guards: they verify the old wrong values no longer trigger a merge or an abort — they just fall into the keep-polling default.

## 5. Add routing regression test to `pkg/processor/processor_automerge_test.go`

Add a new test in the existing `Context("Auto-review", ...)` block in `pkg/processor/processor_automerge_test.go`. The test must verify that when BOTH `autoMerge=true` AND `autoReview=true` are set, the `autoReview` path takes precedence (prompt goes to `in_review`, `WaitAndMerge` is NOT called).

Add this `It` block immediately after the existing "should set status to in_review and NOT move to completed when autoReview=true (PR workflow)" test (around line 720):

```go
It(
    "should take autoReview path (not autoMerge) when both autoReview=true and autoMerge=true",
    func() {
        originalDir, err := os.Getwd()
        Expect(err).NotTo(HaveOccurred())
        DeferCleanup(func() {
            _ = os.Chdir(originalDir)
        })

        promptPath := filepath.Join(promptsDir, "001-autoreview-takes-precedence.md")
        queued := []prompt.Prompt{
            {Path: promptPath, Status: prompt.ApprovedPromptStatus},
        }

        cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
            return os.MkdirAll(destDir, 0750)
        }
        cloner.RemoveStub = func(_ context.Context, path string) error {
            return os.RemoveAll(path)
        }
        manager.ListQueuedReturnsOnCall(0, queued, nil)
        manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
        manager.AllPreviousCompletedReturns(true)
        executor.ExecuteReturns(nil)
        releaser.CommitOnlyReturns(nil)
        brancher.FetchReturns(nil)
        brancher.MergeOriginDefaultReturns(nil)
        brancher.PushReturns(nil)
        prCreator.CreateReturns("https://github.com/test/test/pull/99", nil)
        manager.SetStatusReturns(nil)
        manager.SetPRURLReturns(nil)

        // Create log file with success report
        logDir := filepath.Join(promptsDir, "log")
        _ = os.MkdirAll(logDir, 0750)
        logPath := filepath.Join(logDir, "001-autoreview-takes-precedence.log")
        _ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"autoReview precedence test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

        p := newTestProcessor(
            promptsDir,
            filepath.Join(promptsDir, "completed"),
            logDir,
            "test-project",
            executor,
            manager,
            releaser,
            versionGet,
            wakeup,
            true,
            config.WorkflowClone,
            brancher,
            prCreator,
            cloner,
            worktreer,
            prMerger,
            true, // autoMerge enabled
            false,
            true, // autoReview enabled — must take precedence
            autoCompleter,
            specLister,
            "",
            "",
            "",
            false,
            notifier.NewMultiNotifier(),
            nil,
            0,
            "",
            nil,
            nil,
            0,
            nil,
            nil,
            0,
            0,
            nil,
        )

        go func() {
            _ = p.Process(ctx)
        }()

        // PR must be created
        Eventually(func() int {
            return prCreator.CreateCallCount()
        }, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

        // SetStatus must be called with in_review (autoReview path taken)
        Eventually(func() string {
            if manager.SetStatusCallCount() == 0 {
                return ""
            }
            _, _, status := manager.SetStatusArgsForCall(
                manager.SetStatusCallCount() - 1,
            )
            return status
        }, 2*time.Second, 50*time.Millisecond).Should(Equal(string(prompt.InReviewPromptStatus)))

        // WaitAndMerge must NOT be called (autoMerge path bypassed)
        Consistently(func() int {
            return prMerger.WaitAndMergeCallCount()
        }, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

        // MoveToCompleted must NOT be called (prompt stays in in_review)
        Consistently(func() int {
            return manager.MoveToCompletedCallCount()
        }, 200*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

        cancel()
    },
)
```

Before writing this test, verify the exact `newTestProcessor` call signature by running:
```bash
grep -rn "func newTestProcessor" pkg/processor/
```
and count the number of arguments in the existing calls in `processor_automerge_test.go`. The parameter order and count must match exactly — do NOT guess.

## 6. Add CHANGELOG entry

In `CHANGELOG.md`, add two bullets under `## Unreleased` (create the section if it doesn't exist):

```markdown
## Unreleased

- fix: autoReview routing in `handleAfterIsolatedCommit` — check `AutoReview` before `AutoMerge` so `autoReview: true` prompts transition to `in_review` instead of being routed to `WaitAndMerge` directly
- fix: `WaitAndMerge` field mismatch — switch on correct `mergeStateStatus` values (`CLEAN` → merge, `DIRTY` → fail) instead of wrong `mergeable` enum values (`MERGEABLE`/`CONFLICTING`)
```

If `## Unreleased` already exists, append both bullets to it without replacing the existing entries.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT remove or change `validateAutoReview`'s `AutoMerge` requirement in `pkg/config/config.go` — the intended semantic is "autoReview means: open PR, wait for human review, then auto-merge after approval". Both flags work together by design.
- Do NOT regress the autoMerge-only path (`autoReview: false`): when only `autoMerge=true`, `handleAutoMergeForClone` must still be called.
- Do NOT change the `Releaser`/`Brancher` injection added by spec 071 — that wiring is correct.
- Wrap all non-nil errors with `errors.Wrapf` / `errors.Wrap` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`. The `stderrors.New(...)` in `decideMergeAction` is intentionally a stdlib error; it is immediately wrapped by the caller with `errors.Wrap(ctx, mergeErr, ...)`.
- The `pkg/git/export_test.go` file must use `package git` (NOT `package git_test`) so it can access unexported symbols.
- Do NOT touch `go.mod` / `go.sum` / `vendor/`.
- Existing tests must still pass.
- The `HAS_HOOKS` and `UNSTABLE` states should continue polling (return `false, nil` from `decideMergeAction`) — do not treat them as merge-ready or as failures.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. Verify routing fix order in workflow_helpers.go:
   ```bash
   grep -n "AutoReview\|AutoMerge" pkg/processor/workflow_helpers.go
   ```
   `AutoReview` check must appear at a lower line number than `AutoMerge` check inside `handleAfterIsolatedCommit`.

2. Verify WaitAndMerge no longer contains the old values:
   ```bash
   grep -n "MERGEABLE\|CONFLICTING" pkg/git/pr_merger.go
   ```
   Must return zero matches in production code. (The test file may reference them as regression guards — that's acceptable.)

3. Verify correct values are present:
   ```bash
   grep -n "CLEAN\|DIRTY" pkg/git/pr_merger.go
   ```
   Must show both values in `decideMergeAction`.

4. Verify export_test.go was created:
   ```bash
   ls pkg/git/export_test.go
   ```

5. Verify test coverage includes the routing regression test:
   ```bash
   grep -n "autoReview-takes-precedence\|autoreview-takes-precedence\|autoReview path (not autoMerge)" pkg/processor/processor_automerge_test.go
   ```
   Must return at least one match.

6. Verify decideMergeAction tests are present:
   ```bash
   grep -n "decideMergeAction\|DecideMergeActionForTest" pkg/git/pr_merger_test.go
   ```
   Must return at least one match.

7. Run the affected packages:
   ```bash
   go test -count=1 ./pkg/git/... ./pkg/processor/...
   ```
   Must exit 0.
</verification>
