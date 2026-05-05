---
status: draft
spec: [073-simplify-merge-gate-by-relying-on-mergestatestatus]
created: "2026-05-06T00:00:00Z"
branch: dark-factory/simplify-merge-gate-by-relying-on-mergestatestatus
---

<summary>
- The entire `pkg/review/` package (7 files: doc.go, fix_prompt_generator.go, poller.go, prompt_manager.go, and their test counterparts) is deleted
- `pkg/git/review_fetcher.go` and `pkg/git/bitbucket_review_fetcher.go` are deleted; the `ReviewFetcher` interface and both implementations disappear
- Four generated mocks (`mocks/review_fetcher.go`, `mocks/review_poller.go`, `mocks/fix_prompt_generator.go`, `mocks/review-prompt-manager.go`) are deleted
- `CreateReviewPoller` is removed from `pkg/factory/factory.go`; the conditional `if cfg.AutoReview { poller = CreateReviewPoller(...) }` block in `CreateRunner` is removed
- `reviewPoller` is removed from `pkg/runner/runner.go`'s `Runner` struct and `NewRunner` constructor; all 11 call sites in `runner_test.go` are updated
- `reviewFetcher` and `collaboratorFetcher` are removed from the `providerDeps` struct and their construction in the GitHub and Bitbucket provider paths; the GitHub-only `git.NewCollaboratorFetcher(...)` call disappears
- `autoReview bool` is removed from `CreateWorkflowExecutor` and `CreateProcessor` signatures in `factory.go`, and from the two `CreateProcessor` call sites inside `CreateRunner` and `CreateOneShotRunner`
- `AutoReview bool` is removed from `WorkflowDeps` in `pkg/processor/workflow_executor.go`
- The `if deps.AutoReview { ... }` precedence branch is removed from `handleAfterIsolatedCommit` in `pkg/processor/workflow_helpers.go`; `if deps.AutoMerge` becomes the first and only merge branch
- `pkg/processor/processor_test.go` helper functions drop the `autoReview bool` parameter; all callers in `processor_automerge_test.go` are updated
- `make precommit` passes; config fields `AutoReview`, `AllowedReviewers`, `UseCollaborators`, `MaxReviewRetries`, `PollIntervalSec` still exist on `config.Config` (removed in prompt 2)
</summary>

<objective>
Delete the `ReviewPoller`, all review-fetching code, and every wiring point that feeds into it, so that the codebase no longer contains the autoReview code path. Config fields are left in place for now (prompt 2 removes them and adds friendly migration errors). After this prompt, `autoMerge: true` with branch protection is the only merge gate.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read in full before editing:
- `pkg/factory/factory.go` — `providerDeps` struct (~line 183), `createGitHubProviderDeps` (~line 202), `createBitbucketProviderDeps` (~line 235), `CreateRunner` (~line 300), `CreateWorkflowExecutor` (~line 763), `CreateProcessor` (~line 808), `CreateReviewPoller` (~line 970)
- `pkg/runner/runner.go` — `NewRunner` signature (~line 43), `runner` struct (~line 115), `Run` method (~line 214)
- `pkg/processor/workflow_executor.go` — `WorkflowDeps` struct (~line 70)
- `pkg/processor/workflow_helpers.go` — `handleAfterIsolatedCommit` (~line 234), especially the `if deps.AutoReview` branch (~line 264)
- `pkg/processor/processor_test.go` — `newTestWorkflowExecutor` (~line 44) and `newTestProcessor` (~line 75) helper functions
- `pkg/processor/processor_automerge_test.go` — `autoReview`-specific It blocks (~lines 610 and 724)
- `pkg/runner/runner_test.go` — all 11 `runner.NewRunner(...)` call sites

Files to DELETE entirely (use `rm` via Bash):
- `pkg/review/doc.go`
- `pkg/review/fix_prompt_generator.go`
- `pkg/review/fix_prompt_generator_test.go`
- `pkg/review/poller.go`
- `pkg/review/poller_test.go`
- `pkg/review/prompt_manager.go`
- `pkg/review/review_suite_test.go`
- `pkg/git/review_fetcher.go`
- `pkg/git/bitbucket_review_fetcher.go`
- `mocks/review_fetcher.go`
- `mocks/review_poller.go`
- `mocks/fix_prompt_generator.go`
- `mocks/review-prompt-manager.go`
</context>

<requirements>

## 1. Delete all review-related files

Run the following to delete all files in one shot:

```bash
rm -rf /workspace/pkg/review
rm /workspace/pkg/git/review_fetcher.go
rm /workspace/pkg/git/bitbucket_review_fetcher.go
rm /workspace/mocks/review_fetcher.go
rm /workspace/mocks/review_poller.go
rm /workspace/mocks/fix_prompt_generator.go
rm /workspace/mocks/review-prompt-manager.go
```

Verify with:
```bash
ls /workspace/pkg/review 2>&1  # expected: "No such file or directory"
ls /workspace/pkg/git/review_fetcher.go 2>&1  # expected: "No such file or directory"
ls /workspace/pkg/git/bitbucket_review_fetcher.go 2>&1  # expected: "No such file or directory"
ls /workspace/mocks/review_poller.go 2>&1  # expected: "No such file or directory"
```

## 2. Update `pkg/factory/factory.go`

### 2a. Remove `reviewFetcher` and `collaboratorFetcher` from `providerDeps`

The `providerDeps` struct currently has:
```go
type providerDeps struct {
    prCreator           git.PRCreator
    prMerger            git.PRMerger
    reviewFetcher       git.ReviewFetcher
    collaboratorFetcher git.CollaboratorFetcher
    brancher            git.Brancher
}
```

Change to:
```go
type providerDeps struct {
    prCreator git.PRCreator
    prMerger  git.PRMerger
    brancher  git.Brancher
}
```

### 2b. Update `createGitHubProviderDeps`

Remove the GitHub `collaboratorFetcher` construction entirely. The function currently builds a `collaboratorFetcher` from `git.NewCollaboratorFetcher(...)` — that block is dead code once the review poller is gone.

The updated function body should only initialize `prCreator`, `prMerger`, and `brancher`:

```go
func createGitHubProviderDeps(
    cfg config.Config,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
    ghToken := cfg.ResolvedGitHubToken()
    return providerDeps{
        prCreator: git.NewPRCreator(ghToken),
        prMerger:  git.NewPRMerger(ghToken, currentDateTimeGetter),
        brancher:  git.NewBrancher(git.WithDefaultBranch(cfg.DefaultBranch)),
    }
}
```

Remove the `repoNameFetcher`, `collaboratorLister`, and `collaboratorFetcher` local variables and the `reviewFetcher` assignment. **FREEZE all other logic in this function.**

### 2c. Update `createBitbucketProviderDeps`

Remove `reviewFetcher` and `collaboratorFetcher` from the returned `providerDeps` struct literal. Keep the local `collaboratorFetcher` variable — it is still passed to `NewBitbucketPRCreator`. Do NOT remove `userFetcher` or the `collaboratorFetcher` local.

The `return providerDeps{...}` becomes:
```go
return providerDeps{
    prCreator: git.NewBitbucketPRCreator(
        baseURL, token, coords.Project, coords.Repo, cfg.DefaultBranch, collaboratorFetcher,
    ),
    prMerger: git.NewBitbucketPRMerger(
        baseURL,
        token,
        coords.Project,
        coords.Repo,
        currentDateTimeGetter,
    ),
    brancher: git.NewBrancher(git.WithDefaultBranch(cfg.DefaultBranch)),
}
```

**FREEZE all other logic in `createBitbucketProviderDeps`.**

### 2d. Remove `CreateReviewPoller` function

Delete the entire `CreateReviewPoller` function (from its comment line to its closing brace). It starts at approximately line 970 with:
```go
// CreateReviewPoller creates a ReviewPoller...
func CreateReviewPoller(
```

### 2e. Remove review poller wiring from `CreateRunner`

In `CreateRunner`, remove:
1. The `var poller review.ReviewPoller` declaration
2. The entire `if cfg.AutoReview { poller = CreateReviewPoller(...) }` block (approximately 11 lines)

The `poller` variable must also be removed from the `runner.NewRunner(...)` call — see requirement 2f.

### 2f. Remove `poller` argument from `runner.NewRunner(...)` call in `CreateRunner`

In the `runner.NewRunner(...)` call inside `CreateRunner`, remove the `poller` positional argument. It appears between the `srv` (server) argument and the `specWatcher` argument. The relevant lines:

```go
// BEFORE:
return runner.NewRunner(
    inboxDir, inProgressDir, completedDir, cfg.Prompts.LogDir,
    cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir, cfg.Specs.LogDir,
    promptManager, CreateLocker("."), watcher, proc, srv, poller,
    specWatcher, projectName,
    ...
)

// AFTER:
return runner.NewRunner(
    inboxDir, inProgressDir, completedDir, cfg.Prompts.LogDir,
    cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir, cfg.Specs.LogDir,
    promptManager, CreateLocker("."), watcher, proc, srv,
    specWatcher, projectName,
    ...
)
```

### 2g. Remove `autoReview bool` from `CreateWorkflowExecutor`

Current signature includes `autoReview bool` after `autoRelease bool`. Remove it. Also remove `AutoReview: autoReview` from the `WorkflowDeps{...}` literal inside `CreateWorkflowExecutor`.

### 2h. Remove `autoReview bool` from `CreateProcessor`

Current signature includes `autoReview bool` after `autoRelease bool`. Remove it. Also remove the `autoReview` argument in the `CreateWorkflowExecutor(...)` call inside `CreateProcessor`.

### 2i. Update two `CreateProcessor` call sites

**Call site 1** (`CreateRunner`, approximately line 420): Remove `cfg.AutoReview,` from the argument list.

**Call site 2** (`CreateOneShotRunner`, approximately line 572): Remove `cfg.AutoReview,` from the argument list.

### 2j. Remove the `pkg/review` import

After all changes, `pkg/review` should no longer be imported. Remove it from the import block. Also check that `git.ReviewFetcher` and `git.CollaboratorFetcher` references are gone (they should be, since they were only in `providerDeps`).

Run `grep -n '"github.com/bborbe/dark-factory/pkg/review"' pkg/factory/factory.go` — must return zero matches.

## 3. Update `pkg/runner/runner.go`

### 3a. Remove `reviewPoller review.ReviewPoller` from `NewRunner` parameters

The current signature has `reviewPoller review.ReviewPoller` between `server server.Server` and `specWatcher specwatcher.SpecWatcher`. Remove it.

### 3b. Remove `reviewPoller: reviewPoller` from the struct initializer

In the `return &runner{...}` block inside `NewRunner`, remove the line `reviewPoller: reviewPoller,`.

### 3c. Remove `reviewPoller review.ReviewPoller` from the `runner` struct

In the `runner` struct definition (~line 115), remove the `reviewPoller review.ReviewPoller` field.

### 3d. Remove the `reviewPoller` run loop in `Run`

In the `Run` method, remove:
```go
if r.reviewPoller != nil {
    runners = append(runners, r.reviewPoller.Run)
}
```

### 3e. Remove the `pkg/review` import from `runner.go`

After making these changes, `review` is no longer referenced. Remove the import.

## 4. Update `pkg/processor/workflow_executor.go`

Remove `AutoReview bool` from the `WorkflowDeps` struct. The field appears between `AutoMerge bool` and `AutoRelease bool`:

```go
// BEFORE:
AutoMerge     bool
AutoReview    bool
AutoRelease   bool

// AFTER:
AutoMerge   bool
AutoRelease bool
```

## 5. Update `pkg/processor/workflow_helpers.go`

In the `handleAfterIsolatedCommit` function (or whichever function contains the routing logic at approximately line 234), remove the `if deps.AutoReview { ... }` branch entirely. The full block to remove is approximately:

```go
if deps.AutoReview {
    // AutoReview takes precedence over AutoMerge: open PR, wait for human approval,
    // then the review poller handles merge + release.
    savePRURLToFrontmatter(gitCtx, deps, promptPath, prURL)
    if err := deps.PromptManager.SetStatus(ctx, promptPath, string(prompt.InReviewPromptStatus)); err != nil {
        return errors.Wrap(ctx, err, "set in_review status")
    }
    slog.Info("PR created, waiting for review", "url", prURL)
    return nil
}
```

After removal, the `if deps.AutoMerge { ... }` block that followed becomes the first (and only) conditional — no `else if` prefix needed.

Verify: `grep -n "AutoReview\|autoReview\|in_review\|InReview" pkg/processor/workflow_helpers.go` — must return zero matches.

Also remove the `prompt.InReviewPromptStatus` import reference if it is now unused in `workflow_helpers.go` (check the import block after editing).

## 6. Update `pkg/processor/processor_test.go`

### 6a. Update `newTestWorkflowExecutor`

Remove `autoReview bool` from the parameter list:
```go
// BEFORE:
func newTestWorkflowExecutor(
    workflow config.Workflow, pr, autoMerge, autoRelease, autoReview bool,
    ...
) processor.WorkflowExecutor {
    deps := processor.WorkflowDeps{
        ...
        PR: pr, AutoMerge: autoMerge, AutoReview: autoReview, AutoRelease: autoRelease,
    }
// AFTER:
func newTestWorkflowExecutor(
    workflow config.Workflow, pr, autoMerge, autoRelease bool,
    ...
) processor.WorkflowExecutor {
    deps := processor.WorkflowDeps{
        ...
        PR: pr, AutoMerge: autoMerge, AutoRelease: autoRelease,
    }
```

### 6b. Update `newTestProcessor`

Remove `autoReview bool` from the parameter list and the call to `newTestWorkflowExecutor`:
```go
// BEFORE (in newTestProcessor):
autoMerge, autoRelease, autoReview bool,
...
we := newTestWorkflowExecutor(
    workflow, pr, autoMerge, autoRelease, autoReview,
    ...
)
// AFTER:
autoMerge, autoRelease bool,
...
we := newTestWorkflowExecutor(
    workflow, pr, autoMerge, autoRelease,
    ...
)
```

## 7. Update `pkg/processor/processor_automerge_test.go`

### 7a. Remove the two autoReview-specific It blocks

Delete the entire `It` block that starts with:
```go
It("should set status to in_review and NOT move to completed when autoReview=true (PR workflow)",
```
(approximately lines 610–720)

Delete the entire `It` block that starts with:
```go
It("should take autoReview path (not autoMerge) when both autoReview=true and autoMerge=true",
```
(approximately lines 724–835)

### 7b. Update remaining `newTestProcessor` calls that pass `autoReview`

After removing the two It blocks, search for any remaining `newTestProcessor` calls in `processor_automerge_test.go` that pass a `false` value in the `autoReview` position. Remove that argument.

Run to find call sites:
```bash
grep -n "autoReview\|AutoReview" pkg/processor/processor_automerge_test.go
```
Remove all occurrences (positional `false` arguments in the `autoReview` slot, and comments referencing `autoReview`).

Also search other processor test files that call `newTestProcessor`:
```bash
grep -rn "newTestProcessor" pkg/processor/
```
Update every call site to drop the `autoReview bool` argument.

### 7c. Update `processor_test.go` autoMerge test that references `in_review` status

There is an It block that checks `status == string(prompt.InReviewPromptStatus)` (approximately line 934 in `processor_automerge_test.go`). This is in the `"should move to completed normally when autoReview=false"` test. After removing `autoReview`, this test still tests the `autoMerge=false` / `pr=true` completed path. Remove the `autoReview` parameter from its `newTestProcessor` call (pass `false` was the value; the parameter disappears). The status check for `InReviewPromptStatus` should be deleted from this test since that status can no longer be reached.

## 8. Update `pkg/runner/runner_test.go`

### 8a. Remove the `"should include reviewPoller in run loop when non-nil"` It block

Delete the entire It block (approximately lines 408–468) that creates `mockReviewPoller` and passes it to `NewRunner`.

### 8b. Remove the `"should not include reviewPoller in run loop when nil"` It block

Delete the entire It block (approximately lines 469–520) or, if it simply tests `nil` for `reviewPoller`, adapt it: remove the It block if its only purpose was to test nil vs non-nil behavior of `reviewPoller`.

### 8c. Update all remaining `runner.NewRunner(...)` calls

There are 11 total `NewRunner` call sites in `runner_test.go`. For each one, remove the `reviewPoller` positional argument. It appears between the `server` (or `nil`) argument and the `specWatcher` (or `nil`) argument.

Specifically, replace:
```go
runner.NewRunner(
    ...
    nil, // No server
    mockReviewPoller,  // or nil, // no reviewPoller
    nil, // no specWatcher
    ...
)
```
with:
```go
runner.NewRunner(
    ...
    nil, // No server
    nil, // no specWatcher
    ...
)
```

Run:
```bash
grep -n "runner\.NewRunner\|NewRunner(" pkg/runner/runner_test.go
```
to locate all call sites, then update each one.

### 8d. Remove mock imports that are now unused

If `&mocks.ReviewPoller{}` was the only use of `mocks.ReviewPoller` in runner_test.go, and after removing the two It blocks there are no more `mocks.ReviewPoller` references, remove that import if needed. Run:
```bash
grep -n "ReviewPoller\|mocks\." pkg/runner/runner_test.go | head -20
```
to check.

## 9. Run `make test` iteratively

After completing requirements 1–8, run:
```bash
cd /workspace && make test
```

Fix any compilation errors by tracking down remaining references to the deleted types:
```bash
grep -rn "review\.ReviewPoller\|review\.NewReviewPoller\|deps\.AutoReview\|AutoReview\|reviewPoller" pkg/ --include="*.go" | grep -v "_test.go"
grep -rn "deps\.AutoReview\|AutoReview\|autoReview" pkg/processor/ --include="*_test.go"
```

All results must be zero before running `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT remove `InReviewPromptStatus` from `pkg/prompt/prompt.go` — existing prompts may have `in_review` status and `prompt complete` accepts it; the status remains valid for manual intervention
- Do NOT remove `pkg/git/collaborator_fetcher.go` or `pkg/git/bitbucket_collaborator_fetcher.go` — Bitbucket PR creation still uses `CollaboratorFetcher`; only `review_fetcher.go` and `bitbucket_review_fetcher.go` are deleted
- Do NOT touch `pkg/config/config.go` fields (`AutoReview`, `AllowedReviewers`, `UseCollaborators`, `MaxReviewRetries`, `PollIntervalSec`) — those are removed in prompt 2
- Do NOT touch the Bitbucket path's local `collaboratorFetcher` variable in `createBitbucketProviderDeps` — it is still passed to `NewBitbucketPRCreator`
- Wrap all new errors with `errors.Wrap` / `errors.Errorf` from `github.com/bborbe/errors`
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Existing tests for `autoMerge` (non-autoReview paths) must still pass
- The `PostMergeActions` function in `pkg/processor/workflow_helpers.go` is NOT removed — it is the shared post-merge ceremony used by `WaitAndMerge`
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -rn "pkg/review" pkg/ --include="*.go"` — must return zero matches (no imports of deleted package)
2. `grep -rn "ReviewFetcher\|ReviewPoller\|FixPromptGenerator" pkg/ --include="*.go"` — must return zero matches
3. `grep -rn "AutoReview\|autoReview" pkg/ --include="*.go" | grep -v "_test.go" | grep -v "config/config.go" | grep -v "config/loader.go"` — must return zero matches (config fields still exist for prompt 2; no other non-test references)
4. `grep -n "reviewPoller" pkg/runner/runner.go` — must return zero matches
5. `grep -n "AutoReview" pkg/processor/workflow_executor.go` — must return zero matches
6. `ls pkg/review/ 2>&1` — must print "No such file or directory"
7. `ls pkg/git/review_fetcher.go 2>&1` — must print "No such file or directory"
</verification>
