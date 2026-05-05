---
status: committing
spec: [071-bug-autoreview-skips-postmerge-actions-no-tag-no-release]
summary: Exported postMergeActions as PostMergeActions from pkg/processor, injected Brancher/Releaser/AutoRelease into reviewPoller, called PostMergeActions from handleApproved after MoveToCompleted, updated factory.go CreateReviewPoller call site, added new test cases for autoRelease=true/false and WaitAndMerge failure, and added CHANGELOG entry.
container: dark-factory-377-fix-071-autoreview-postmerge-actions
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-05T20:30:00Z"
queued: "2026-05-05T20:34:03Z"
started: "2026-05-05T20:34:04Z"
branch: dark-factory/bug-autoreview-skips-postmerge-actions-no-tag-no-release
---

<summary>
- After an allowed reviewer approves a PR, dark-factory now pulls the default branch and optionally cuts a release tag — the same end state as the autoMerge-only path
- `postMergeActions` is exported as `PostMergeActions` so both the processor and review-poller call sites share one implementation with no logic duplication
- The review poller receives three new injected deps (`Brancher`, `Releaser`, `AutoRelease`) wired from the factory — no parallel git abstraction added to `pkg/review/`
- `handleApproved` calls `PostMergeActions` after `MoveToCompleted` so master is fast-forwarded to origin, `## Unreleased` is renamed to `## vX.Y.Z`, and the tag is pushed
- With `autoRelease: false` or no `CHANGELOG.md`, the merge still fast-forwards master but creates no tag — matches the existing autoMerge-only path under those conditions
- The autoMerge-only path (`handleAutoMergeForClone`) is unchanged in behavior; it now calls the shared `PostMergeActions` instead of the previously-lowercase private function
- New unit tests assert that `handleApproved` calls `PostMergeActions` (via mock Brancher/Releaser), covers autoRelease=true with changelog, autoRelease=false, and no-changelog variants
- Existing test that asserts `WaitAndMerge + MoveToCompleted` is updated to also assert the post-merge brancher calls
- `CreateReviewPoller` in `pkg/factory/factory.go` is updated to pass `deps.brancher`, `releaser`, and `cfg.AutoRelease` — call site in `CreateRunner` passes the already-constructed `releaser`
- CHANGELOG.md `## Unreleased` entry added
</summary>

<objective>
Fix the bug where `reviewPoller.handleApproved` merges the PR and moves the prompt to completed but does NOT run post-merge actions (switch to default branch, pull, create release tag). The fix exports `postMergeActions` from `pkg/processor` as `PostMergeActions`, injects `Brancher`, `Releaser`, and `AutoRelease` into the review poller, and calls `PostMergeActions` from `handleApproved` after `MoveToCompleted`. After this fix, a project configured with `pr+autoMerge+autoRelease+autoReview` produces the same end state as `pr+autoMerge+autoRelease` (no autoReview) once the reviewer approves.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read in full before editing:

- `pkg/processor/workflow_helpers.go` — `postMergeActions` (lines 167–191) is the function to export; `handleAutoMergeForClone` (lines 193–224) is the existing caller; `handleDirectWorkflow` (lines 126–165) is called inside `postMergeActions`
- `pkg/review/poller.go` — `handleApproved` (lines 203–218) is the missing call site; `NewReviewPoller` constructor (lines 34–60) needs three new params; `reviewPoller` struct (lines 63–77) needs three new fields; `processPrompt` (lines 142–200) needs to pass `pf` to `handleApproved`
- `pkg/review/poller_test.go` — existing test "calls WaitAndMerge and MoveToCompleted when review is approved" (line ~101) needs updating; constructor call (line ~80) needs new params; new test cases needed
- `pkg/factory/factory.go` — `CreateReviewPoller` (lines 961–985) needs `releaser git.Releaser` and `autoRelease bool` new params; the call site at line 355 must pass `releaser` and `cfg.AutoRelease`
- `mocks/brancher.go` — Brancher mock; `DefaultBranchReturns`, `SwitchReturns`, `PullReturns` are available
- `mocks/releaser.go` — Releaser mock; `HasChangelogReturns`, `CommitAndReleaseReturns` are available

Key inline snippet — the existing `postMergeActions` function body (to be exported verbatim, no logic change):

```go
func postMergeActions(
    gitCtx context.Context,
    ctx context.Context,
    deps WorkflowDeps,
    title string,
) error {
    defaultBranch, err := deps.Brancher.DefaultBranch(gitCtx)
    if err != nil {
        return errors.Wrap(ctx, err, "get default branch")
    }
    if err := deps.Brancher.Switch(gitCtx, defaultBranch); err != nil {
        return errors.Wrap(ctx, err, "switch to default branch")
    }
    if err := deps.Brancher.Pull(gitCtx); err != nil {
        return errors.Wrap(ctx, err, "pull default branch")
    }
    slog.Info("merged PR and updated default branch", "branch", defaultBranch)
    if deps.AutoRelease && deps.Releaser.HasChangelog(gitCtx) {
        if err := handleDirectWorkflow(gitCtx, ctx, deps, title, ""); err != nil {
            return errors.Wrap(ctx, err, "auto-release after merge")
        }
    }
    return nil
}
```

Key inline snippet — the existing `handleApproved` body (to be modified):

```go
// handleApproved merges the PR and marks the prompt as completed.
func (p *reviewPoller) handleApproved(ctx context.Context, path string, prURL string) {
    if err := p.prMerger.WaitAndMerge(ctx, prURL); err != nil {
        slog.Warn("failed to merge PR", "file", filepath.Base(path), "error", err)
        return
    }
    if err := p.promptManager.MoveToCompleted(ctx, path); err != nil {
        slog.Warn(
            "failed to move approved prompt to completed",
            "file",
            filepath.Base(path),
            "error",
            err,
        )
    }
}
```

The call site in `processPrompt`:

```go
case git.ReviewVerdictApproved:
    p.handleApproved(ctx, path, prURL)
```

`pf` is available in `processPrompt` at that point (loaded at the top of the function).
</context>

<requirements>

## 1. Export `postMergeActions` in `pkg/processor/workflow_helpers.go`

Rename `postMergeActions` → `PostMergeActions`. The function signature and body are UNCHANGED — only the capitalization changes.

```go
// PostMergeActions switches to default branch, pulls, and optionally releases.
// Called by both handleAutoMergeForClone (autoMerge path) and reviewPoller.handleApproved
// (autoReview path) so both delivery paths share one implementation.
func PostMergeActions(
    gitCtx context.Context,
    ctx context.Context,
    deps WorkflowDeps,
    title string,
) error {
    // ... identical body ...
}
```

Update the only internal caller `handleAutoMergeForClone` (line 223):

```go
return PostMergeActions(gitCtx, ctx, deps, title)
```

No other changes in this file.

## 2. Add fields and params to `reviewPoller` in `pkg/review/poller.go`

### 2a. Add three fields to `reviewPoller` struct

Add immediately after the `prMerger` field:

```go
brancher    git.Brancher
releaser    git.Releaser
autoRelease bool
```

### 2b. Update `NewReviewPoller` constructor

Add three new params at the end of the parameter list (before the closing paren), after `n notifier.Notifier`:

```go
brancher    git.Brancher,
releaser    git.Releaser,
autoRelease bool,
```

Wire them into the returned struct:

```go
return &reviewPoller{
    // ... existing fields unchanged ...
    brancher:    brancher,
    releaser:    releaser,
    autoRelease: autoRelease,
}
```

### 2c. Add import for `pkg/processor`

Add to the import block in `poller.go`:

```go
"github.com/bborbe/dark-factory/pkg/processor"
```

(The `pkg/review` package does NOT currently import `pkg/processor`. Verify there is no import cycle: `pkg/processor` imports `pkg/git`, `pkg/prompt`, `pkg/spec` — it does NOT import `pkg/review`, so this import is safe.)

### 2d. Update `handleApproved` signature to accept `pf`

Change the signature from:

```go
func (p *reviewPoller) handleApproved(ctx context.Context, path string, prURL string) {
```

to:

```go
func (p *reviewPoller) handleApproved(ctx context.Context, path string, prURL string, pf *prompt.PromptFile) {
```

### 2e. Call `PostMergeActions` inside `handleApproved`

After the existing `MoveToCompleted` block, add:

```go
title := pf.Title()
if title == "" {
    title = strings.TrimSuffix(filepath.Base(path), ".md")
}
gitCtx := context.WithoutCancel(ctx)
deps := processor.WorkflowDeps{
    Brancher:    p.brancher,
    Releaser:    p.releaser,
    AutoRelease: p.autoRelease,
}
if err := processor.PostMergeActions(gitCtx, ctx, deps, title); err != nil {
    slog.Warn("post-merge actions failed", "file", filepath.Base(path), "error", err)
}
```

**The `slog.Warn` (not return) is intentional**: if the merge already happened, we cannot undo it. We log the failure and continue — the prompt is already in completed state. This mirrors how the existing code handles `MoveToCompleted` errors.

Ensure `"strings"` and `"path/filepath"` are imported (they are already present in `poller.go`; verify before adding).

### 2f. Pass `pf` from `processPrompt` to `handleApproved`

In `processPrompt`, the `case git.ReviewVerdictApproved:` branch currently reads:

```go
case git.ReviewVerdictApproved:
    p.handleApproved(ctx, path, prURL)
```

Change to:

```go
case git.ReviewVerdictApproved:
    p.handleApproved(ctx, path, prURL, pf)
```

`pf` is loaded at the top of `processPrompt` and is in scope at this point.

## 3. Update `CreateReviewPoller` in `pkg/factory/factory.go`

### 3a. Add params to `CreateReviewPoller`

Add two new params to the function signature, after `currentDateTimeGetter libtime.CurrentDateTimeGetter`:

```go
func CreateReviewPoller(
    ctx context.Context,
    cfg config.Config,
    promptManager *prompt.Manager,
    projectName project.Name,
    n notifier.Notifier,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    releaser git.Releaser,
    autoRelease bool,
) review.ReviewPoller {
```

### 3b. Wire into `NewReviewPoller` call

In `CreateReviewPoller`, the `review.NewReviewPoller(...)` call already passes `deps.collaboratorFetcher`, `deps.reviewFetcher`, `deps.prMerger`. Add `deps.brancher`, `releaser`, `autoRelease` as the three final arguments (after `n`):

```go
return review.NewReviewPoller(
    cfg.Prompts.InProgressDir,
    cfg.Prompts.InboxDir,
    deps.collaboratorFetcher,
    cfg.MaxReviewRetries,
    time.Duration(cfg.PollIntervalSec)*time.Second,
    deps.reviewFetcher,
    deps.prMerger,
    promptManager,
    review.NewFixPromptGenerator(),
    projectName,
    n,
    deps.brancher,
    releaser,
    autoRelease,
)
```

### 3c. Update the call site in `CreateRunner` (line ~355)

```go
poller = CreateReviewPoller(ctx, cfg, promptManager, projectName, n, currentDateTimeGetter, releaser, cfg.AutoRelease)
```

`releaser` is created at line ~318 by `createPromptManager(...)` and is in scope at line ~355 where `CreateReviewPoller` is called. `cfg.AutoRelease` is directly on the config struct.

## 4. Update `pkg/review/poller_test.go`

### 4a. Add `brancher` and `releaser` mock variables

In the `BeforeEach`, declare and initialize two new mocks:

```go
var (
    // ... existing vars ...
    brancher *mocks.Brancher
    releaser *mocks.Releaser
)
```

In the `BeforeEach` body, initialize them:

```go
brancher = &mocks.Brancher{}
releaser = &mocks.Releaser{}
// Default: post-merge actions succeed — brancher returns a default branch and pull succeeds
brancher.DefaultBranchReturns("master", nil)
brancher.SwitchReturns(nil)
brancher.PullReturns(nil)
// Default: no CHANGELOG → no release tag created
releaser.HasChangelogReturns(false)
```

### 4b. Update `review.NewReviewPoller(...)` call in `BeforeEach`

Add the three new trailing args:

```go
poller = review.NewReviewPoller(
    queueDir,
    inboxDir,
    collaboratorFetcher,
    maxRetries,
    1*time.Millisecond,
    fetcher,
    prMerger,
    manager,
    generator,
    "",
    notifier.NewMultiNotifier(),
    brancher,
    releaser,
    false, // autoRelease=false in default test setup; individual tests override
)
```

### 4c. Update the existing "calls WaitAndMerge and MoveToCompleted when review is approved" test

This test currently asserts `WaitAndMergeCallCount >= 1` and `MoveToCompletedCallCount >= 1`. After the fix, `PostMergeActions` also runs, calling `brancher.DefaultBranch`, `brancher.Switch`, `brancher.Pull`. Add an assertion:

```go
Eventually(func() bool {
    return prMerger.WaitAndMergeCallCount() >= 1 &&
        manager.MoveToCompletedCallCount() >= 1 &&
        brancher.PullCallCount() >= 1
}).Should(BeTrue())
```

### 4d. Add new test: autoRelease=true + CHANGELOG triggers CommitAndRelease

```go
It("calls CommitAndRelease after approval when autoRelease=true and CHANGELOG exists", func() {
    pollerWithRelease := review.NewReviewPoller(
        queueDir,
        inboxDir,
        collaboratorFetcher,
        maxRetries,
        1*time.Millisecond,
        fetcher,
        prMerger,
        manager,
        generator,
        "",
        notifier.NewMultiNotifier(),
        brancher,
        releaser,
        true, // autoRelease=true
    )

    fetcher.FetchLatestReviewReturns(&git.ReviewResult{
        Verdict: git.ReviewVerdictApproved,
    }, nil)
    prMerger.WaitAndMergeReturns(nil)
    manager.MoveToCompletedReturns(nil)
    brancher.DefaultBranchReturns("master", nil)
    brancher.SwitchReturns(nil)
    brancher.PullReturns(nil)
    releaser.HasChangelogReturns(true)
    releaser.CommitAndReleaseReturns(nil)

    runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer runCancel()
    go func() { _ = pollerWithRelease.Run(runCtx) }()

    Eventually(func() bool {
        return releaser.CommitAndReleaseCallCount() >= 1
    }).Should(BeTrue())
})
```

### 4e. Add new test: autoRelease=false — Pull is called but CommitAndRelease is NOT

```go
It("pulls default branch but does not tag when autoRelease=false", func() {
    fetcher.FetchLatestReviewReturns(&git.ReviewResult{
        Verdict: git.ReviewVerdictApproved,
    }, nil)
    prMerger.WaitAndMergeReturns(nil)
    manager.MoveToCompletedReturns(nil)
    // default setup: brancher.Pull/Switch/DefaultBranch return nil, releaser.HasChangelog=false

    runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer runCancel()
    go func() { _ = poller.Run(runCtx) }()

    Eventually(func() bool {
        return brancher.PullCallCount() >= 1
    }).Should(BeTrue())
    runCancel()

    Expect(releaser.CommitAndReleaseCallCount()).To(Equal(0))
})
```

### 4f. Add new test: WaitAndMerge fails — PostMergeActions is NOT called (existing behavior preserved)

```go
It("does not call PostMergeActions when WaitAndMerge fails", func() {
    fetcher.FetchLatestReviewReturns(&git.ReviewResult{
        Verdict: git.ReviewVerdictApproved,
    }, nil)
    prMerger.WaitAndMergeReturns(stderrors.New("merge failed"))

    runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer runCancel()
    Expect(poller.Run(runCtx)).To(Succeed())

    Expect(brancher.PullCallCount()).To(Equal(0))
    Expect(brancher.DefaultBranchCallCount()).To(Equal(0))
})
```

### 4g. Update the `pollerWithNotifier` inline construction in the notification test

The test "fires review_limit notification when retry limit is reached" constructs a `review.NewReviewPoller(...)` inline. Add the three new trailing params to that call as well:

```go
pollerWithNotifier := review.NewReviewPoller(
    queueDir,
    inboxDir,
    collaboratorFetcher,
    maxRetries,
    1*time.Millisecond,
    fetcher,
    prMerger,
    manager,
    generator,
    "test-project",
    fakeNotifier,
    brancher,   // new
    releaser,   // new
    false,      // autoRelease=false
)
```

## 5. Add CHANGELOG entry

In `CHANGELOG.md`, add under `## Unreleased` (create the section at the top if it does not exist yet, above the latest versioned heading):

```markdown
- fix: `autoReview` approval now runs `postMergeActions` after merge — master is pulled locally, `## Unreleased` is promoted to `## vX.Y.Z`, tag is created and pushed (matches `autoMerge`-only path)
```

## 6. Run `make test` iteratively

After each meaningful change, run:

```bash
cd /workspace && make test
```

Fix any compilation errors before proceeding. Run `make precommit` once at the very end.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT change `WaitAndMerge` behavior — merge continues via `gh pr merge --merge --delete-branch`.
- Do NOT change `allowedReviewers` / `useCollaborators` filtering — the "is this approval valid" check is unchanged.
- Do NOT change prompt-status transitions (`in_review` → `completed` on approve, `in_review` → `failed` on retry-limit changes-requested).
- Do NOT introduce a separate "release after autoReview" config flag — `autoRelease: true` already means "release on merge"; the poller must honor it.
- Do NOT stuff `postMergeActions` logic inline into the poller — use the shared `PostMergeActions` export from `pkg/processor`.
- Do NOT introduce a parallel git-aware abstraction in `pkg/review/` — reuse `Releaser`/`Brancher` injected via `WorkflowDeps`-equivalent pattern (pass them directly into `processor.WorkflowDeps{...}` when calling `PostMergeActions`).
- Do NOT auto-tag a release without `CHANGELOG.md` present (matches existing `handleDirectWorkflow` behavior; `HasChangelog` gate is inside `PostMergeActions`).
- Do NOT regress the autoMerge-only path — `handleAutoMergeForClone` continues to call `PostMergeActions` (just renamed from `postMergeActions`).
- `PostMergeActions` failure logs a warning but does NOT abort the poller loop — prompt is already completed; we cannot undo the merge.
- Wrap all non-nil errors with `errors.Wrap`/`errors.Wrapf`/`errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Do NOT touch `go.mod` / `go.sum` / `vendor/`.
- Existing tests must still pass; `poller_test.go` constructor calls must all be updated.
- `make precommit` must exit 0.
- The `autoReview: false` path (existing happy path `handleAutoMergeForClone`) must not regress — run `make test ./pkg/processor/...` after renaming `postMergeActions`.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot-checks after implementation:

1. `grep -n "PostMergeActions\|postMergeActions" pkg/processor/workflow_helpers.go` — two occurrences: the exported function definition and the call inside `handleAutoMergeForClone`.
2. `grep -n "PostMergeActions" pkg/review/poller.go` — one occurrence (inside `handleApproved`).
3. `grep -n "brancher\|releaser\|autoRelease" pkg/review/poller.go` — at least six occurrences (struct fields + constructor params + WorkflowDeps literal).
4. `grep -n "PostMergeActions" pkg/processor/workflow_helpers.go pkg/review/poller.go` — exactly two files, exactly two occurrences total in those files.
5. `grep -nE "CreateReviewPoller\(.*releaser.*cfg\.AutoRelease" pkg/factory/factory.go` — one occurrence at the `CreateReviewPoller` call site.
6. `grep -n "CommitAndRelease\|HasChangelog" pkg/review/poller_test.go` — at least two occurrences (new test case 4d).
7. `grep -n "func postMergeActions" pkg/processor/workflow_helpers.go` — zero occurrences (must be renamed to `PostMergeActions`).
8. `cd /workspace && go test ./pkg/review/... ./pkg/processor/... ./pkg/factory/...` — all pass.
</verification>
