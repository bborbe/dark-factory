---
status: completed
spec: [065-bug-pr-create-missing-head-flag-in-isolated-workflows]
summary: Added explicit `branch string` parameter to `PRCreator.Create`, pass `--head <branch>` to `gh pr create`, removed `currentGitBranch` from Bitbucket impl, threaded branch through all callers, regenerated mock, updated all test call sites.
container: dark-factory-370-spec-065-fix-pr-create-missing-head-flag
dark-factory-version: v0.147.2-1-g30ba42f
created: "2026-05-03T20:00:00Z"
queued: "2026-05-03T19:39:42Z"
started: "2026-05-03T19:47:11Z"
completed: "2026-05-03T20:06:04Z"
branch: dark-factory/bug-pr-create-missing-head-flag-in-isolated-workflows
---

<summary>
- `PRCreator.Create` interface gains an explicit `branch string` parameter so callers cannot accidentally omit it
- `gh pr create` is invoked with `--head <branch>` for the GitHub implementation, removing its dependence on the working directory's current branch
- Bitbucket `Create` implementation uses the passed `branch` argument instead of calling `currentGitBranch(ctx)` from the cwd; the now-unused `currentGitBranch` helper is removed
- `findOrCreatePR` in `pkg/processor/workflow_helpers.go` passes the already-present `branchName` through to `Create`
- `completePRWorkflow` in `pkg/cmd/prompt_complete.go` passes the already-resolved `branch` variable through to `Create`
- Counterfeiter mock is regenerated to match the new 4-arg signature
- Unit test for `prCreator.Create` asserts the `gh` argv contains `--head <branch>`
- All existing `Create` call sites in tests are updated to supply the new branch argument
- `worktree + pr: true` and `clone + pr: true` happy paths are no longer blocked by a "head==base" error
</summary>

<objective>
Fix `PRCreator.Create` to accept an explicit `branch string` parameter and pass `--head <branch>` to `gh pr create`, so that PR creation succeeds when the working directory has already been reset to the master worktree. The bug is that `workflow: worktree` (and `workflow: clone`) resets cwd back to the original master worktree before calling PR create, so `gh` infers `--head` as `master` and rejects with "head branch is the same as base branch".
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `docs/workflows.md` for behavioral context on worktree and clone workflows.

Files to read fully before editing:
- `pkg/git/pr_creator.go` — `PRCreator` interface, `prCreator.Create`, `FindOpenPR`
- `pkg/git/pr_creator_test.go` — all existing `Create` call sites to update
- `pkg/git/bitbucket_pr_creator.go` — `bitbucketPRCreator.Create` (uses `currentGitBranch`), `currentGitBranch` helper (lines 171–175)
- `pkg/processor/workflow_helpers.go` — `findOrCreatePR` (line 88 calls `deps.PRCreator.Create`)
- `pkg/cmd/prompt_complete.go` — `completePRWorkflow` (line 162 calls `c.prCreator.Create`)
- `pkg/cmd/prompt_complete_test.go` — any `CreateArgsForCall` assertions that need updating

Key fact: `findOrCreatePR` already receives `branchName string` as a parameter (line 67 of `workflow_helpers.go`). The fix is threading it the final one hop into `Create`. Similarly, `completePRWorkflow` already has a `branch` variable (lines 149–152 of `prompt_complete.go`).
</context>

<requirements>

## 1. Update `PRCreator` interface in `pkg/git/pr_creator.go`

Change the `Create` method signature to include `branch string` as the fourth parameter:

```go
type PRCreator interface {
    // Create creates a pull request on the given branch and returns the PR URL.
    Create(ctx context.Context, title string, body string, branch string) (string, error)
    // FindOpenPR returns the URL of an open PR for the given branch, or "" if none exists.
    FindOpenPR(ctx context.Context, branch string) (string, error)
}
```

## 2. Update `prCreator.Create` in `pkg/git/pr_creator.go`

Add `branch string` as the fourth parameter and pass `--head`, branch to the `gh` command **before** `--title`:

```go
func (p *prCreator) Create(ctx context.Context, title string, body string, branch string) (string, error) {
    if err := ValidatePRTitle(ctx, title); err != nil {
        return "", errors.Wrap(ctx, err, "validate PR title")
    }
    // #nosec G204 -- title is from prompt frontmatter, body is static text, branch is validated
    cmd := exec.CommandContext(
        ctx,
        "gh", "pr", "create",
        "--head", branch,
        "--title", title,
        "--body", body,
    )
    if p.ghToken != "" {
        cmd.Env = append(os.Environ(), "GH_TOKEN="+p.ghToken)
    }
    var stderr strings.Builder
    cmd.Stderr = &stderr
    output, err := p.commandOutputFn(cmd)
    if err != nil {
        return "", errors.Errorf(ctx, "create pull request: %v: %s", err, stderr.String())
    }
    return strings.TrimSpace(string(output)), nil
}
```

**FREEZE all other methods and functions in this file.**

## 3. Update `bitbucketPRCreator.Create` in `pkg/git/bitbucket_pr_creator.go`

Add `branch string` as the fourth parameter. Replace the `currentGitBranch(ctx)` call with the passed `branch` argument:

```go
func (b *bitbucketPRCreator) Create(
    ctx context.Context,
    title string,
    body string,
    branch string,
) (string, error) {
    targetBranch := b.defaultBranch
    if targetBranch == "" {
        targetBranch = "master"
    }

    fetchedReviewers := b.reviewerFetcher.Fetch(ctx)
    reviewers := make([]bbReviewer, 0, len(fetchedReviewers))
    for _, r := range fetchedReviewers {
        reviewers = append(reviewers, bbReviewer{User: bbUser{Name: r}})
    }

    repo := bbRepository{
        Slug:    b.repo,
        Project: bbProject{Key: b.project},
    }

    reqBody := bbPRRequest{
        Title:       title,
        Description: body,
        FromRef: bbRef{
            ID:         "refs/heads/" + branch,
            Repository: repo,
        },
        ToRef: bbRef{
            ID:         "refs/heads/" + targetBranch,
            Repository: repo,
        },
        Reviewers: reviewers,
    }

    var prResp bbPRResponse
    path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests", b.project, b.repo)
    if err := b.client.do(ctx, "POST", path, reqBody, &prResp); err != nil {
        return "", errors.Wrap(ctx, err, "create pull request")
    }

    if len(prResp.Links.Self) > 0 {
        slog.Info("created Bitbucket PR", "id", prResp.ID, "url", prResp.Links.Self[0].Href)
        return prResp.Links.Self[0].Href, nil
    }
    prURL := fmt.Sprintf(
        "%s/projects/%s/repos/%s/pull-requests/%d",
        b.client.baseURL, b.project, b.repo, prResp.ID,
    )
    slog.Info("created Bitbucket PR", "id", prResp.ID, "url", prURL)
    return prURL, nil
}
```

After updating `Create`, **delete the `currentGitBranch` function** (lines 171–175) — it is no longer called anywhere and would cause a lint error (`unused function`).

**FREEZE `FindOpenPR` and all other methods in this file.**

## 4. Update caller in `pkg/processor/workflow_helpers.go`

In `findOrCreatePR` (around line 88), pass `branchName` as the fourth argument to `Create`:

```go
prURL, err = deps.PRCreator.Create(gitCtx, title, buildPRBody(issue), branchName)
```

No other changes to this file.

## 5. Update caller in `pkg/cmd/prompt_complete.go`

In `completePRWorkflow` (around line 162), pass `branch` as the fourth argument to `Create`:

```go
prURL, err := c.prCreator.Create(gitCtx, title, "Automated by dark-factory", branch)
```

The `branch` variable is already resolved on lines 149–152 of the same function. No other changes to this file.

## 6. Regenerate the Counterfeiter mock

After updating the interface in step 1, run:

```bash
cd /workspace && go generate ./pkg/git/...
```

This regenerates `mocks/pr_creator.go` with the new 4-arg `Create(ctx, title, body, branch)` signature. Verify the mock compiled correctly:

```bash
grep -A 5 "func (fake \*PRCreator) Create" mocks/pr_creator.go
```

Expected: the stub call and args-tracking struct include a fourth `arg4 string` field.

## 7. Update `pkg/git/pr_creator_test.go`

### 7a. Add `--head branch` assertion test

Add a new `It` block inside the existing `Describe("Create", ...)` block that asserts the generated `gh` command includes `--head`:

```go
It("passes --head branch to gh pr create", func() {
    var capturedArgs []string
    p := git.NewPRCreatorWithCommandOutput("", func(cmd *exec.Cmd) ([]byte, error) {
        capturedArgs = cmd.Args
        return []byte("https://github.com/owner/repo/pull/1\n"), nil
    })
    _, err := p.Create(ctx, "Test PR", "Test body", "dark-factory/my-branch")
    Expect(err).NotTo(HaveOccurred())
    Expect(capturedArgs).To(ContainElements("--head", "dark-factory/my-branch"))
})
```

### 7b. Update all existing `Create` call sites

Every existing call to `p.Create(ctx, ...)` in this file currently passes 3 arguments. Add a branch string as the fourth argument. Use `"dark-factory/test-branch"` as the placeholder branch for tests that don't care about its value:

- `p.Create(ctx, "Test PR", "Test body")` → `p.Create(ctx, "Test PR", "Test body", "dark-factory/test-branch")`
- `p.Create(ctx, "--title-injection", "body")` → `p.Create(ctx, "--title-injection", "body", "dark-factory/test-branch")`
- `p.Create(ctx, "-bad-title", "body")` → `p.Create(ctx, "-bad-title", "body", "dark-factory/test-branch")`
- `p.Create(ctx, "Valid title", "body")` → `p.Create(ctx, "Valid title", "body", "dark-factory/test-branch")`
- The GH_TOKEN test: `p.Create(ctx, "Test PR", "body")` → `p.Create(ctx, "Test PR", "body", "dark-factory/test-branch")`

Run `grep -n "\.Create(" pkg/git/pr_creator_test.go` first to find all occurrences exactly.

## 8. Check `pkg/cmd/prompt_complete_test.go` for `CreateArgsForCall` usage

Run:

```bash
grep -n "CreateArgsForCall\|CreateCallCount\|CreateReturns" pkg/cmd/prompt_complete_test.go
```

If there are any `CreateArgsForCall` assertions that check the branch argument by index, update them (the branch is now the 4th arg, index 3 in 0-based). Based on a read of the file, the existing tests only check `prCreator.CreateCallCount()` and set `prCreator.CreateReturns(...)` — neither needs updating. Verify and proceed.

## 9. Run `make test` to verify

After all changes, run `make test` in `/workspace`. All existing tests must pass. The new test (step 7a) must also pass.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Wrap all non-nil errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf`, never bare `return err`
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Do NOT change `FindOpenPR` — it already passes `branch` explicitly and is correct
- The "2 uncommitted changes" `gh` warning mentioned in the spec is pre-existing noise from `.dark-factory.yaml` and prompt status writes in the master worktree; it is benign and out of scope for this prompt — do NOT add git stash or clean logic
- The `currentGitBranch` function in `bitbucket_pr_creator.go` must be removed after the Bitbucket fix; leaving it causes a lint failure (`unused function`)
- Existing tests must still pass; the mock regeneration in step 6 must happen before running tests
- Test package for `pkg/git/pr_creator_test.go` is `package git_test` (external test package)
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "\-\-head" pkg/git/pr_creator.go` — exactly one occurrence, in the `Create` method's `exec.CommandContext` call
2. `grep -n "currentGitBranch" pkg/git/bitbucket_pr_creator.go` — must return NO matches (function deleted)
3. `grep -n "\.Create(" pkg/processor/workflow_helpers.go` — one occurrence; verify it ends with `, branchName)`
4. `grep -n "\.Create(" pkg/cmd/prompt_complete.go` — one occurrence; verify it ends with `, branch)`
5. `grep -A 5 "arg4 string" mocks/pr_creator.go` — mock has the new fourth argument
6. `grep -c "dark-factory/test-branch\|dark-factory/my-branch" pkg/git/pr_creator_test.go` — ≥6 occurrences (all updated call sites plus the new `--head` test)
</verification>
