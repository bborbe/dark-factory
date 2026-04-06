---
status: completed
summary: Replaced fmt.Errorf with errors.Errorf/errors.Wrapf from github.com/bborbe/errors in pkg/git/validate.go, pkg/git/bitbucket_pr_merger.go, and pkg/executor/executor.go; added ctx context.Context to ValidateBranchName, ValidatePRTitle, and parseBitbucketPRID; updated all callers including brancher.go, pr_creator.go, bitbucket_review_fetcher.go, and test files.
container: dark-factory-271-review-dark-factory-fix-fmt-errorf
dark-factory-version: v0.104.2-dirty
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T17:04:41Z"
started: "2026-04-06T17:56:55Z"
completed: "2026-04-06T18:08:48Z"
---

<summary>
- Three packages use the standard library fmt.Errorf instead of the project error library
- Using fmt.Errorf loses stack traces and context values that the project-wide error library attaches
- Affected functions include branch name and PR title validators, a PR ID parser, and an auth validator
- Some affected functions lack a ctx parameter entirely and need one added
- The fix is mechanical: add ctx where missing, swap fmt.Errorf for errors.Errorf, swap fmt.Errorf wrapping for errors.Wrapf
</summary>

<objective>
Replace all `fmt.Errorf` calls in production code with `errors.Errorf` or `errors.Wrapf` from `github.com/bborbe/errors`, and add `ctx context.Context` parameters to functions that currently have none. This ensures consistent stack trace and context propagation across all error paths.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/dod.md` for Definition of Done.

Files to read before making changes (read ALL first):
- `pkg/git/validate.go` — contains `ValidateBranchName` (~line 14) and `ValidatePRTitle` (~line 26) using `fmt.Errorf`
- `pkg/git/bitbucket_pr_merger.go` — contains `parseBitbucketPRID` (~line 115) using `fmt.Errorf`; function has no ctx parameter
- `pkg/executor/executor.go` — contains `validateClaudeAuth` (~line 504) using `fmt.Errorf`; function receives `_ context.Context` (discards ctx)
- `pkg/git/git.go` or callers of `ValidateBranchName` / `ValidatePRTitle` to understand how they are called and what ctx is available
- `pkg/git/pr_merger.go` or `pkg/git/bitbucket_pr_merger.go` callers of `parseBitbucketPRID` to confirm ctx availability
</context>

<requirements>
1. In `pkg/git/validate.go`:
   a. Change `ValidateBranchName(name string) error` to `ValidateBranchName(ctx context.Context, name string) error` — add `ctx` as first param.
   b. Replace `fmt.Errorf("invalid branch name %q: ...")` with `errors.Errorf(ctx, "invalid branch name %q: ...", ...)` using import `github.com/bborbe/errors`.
   c. Change `ValidatePRTitle(title string) error` to `ValidatePRTitle(ctx context.Context, title string) error`.
   d. Replace both `fmt.Errorf("invalid PR title: ...")` calls with `errors.Errorf(ctx, "invalid PR title: ...")`.
   e. Update all callers of `ValidateBranchName` and `ValidatePRTitle` throughout the repository to pass `ctx` as the first argument.

2. In `pkg/git/bitbucket_pr_merger.go`:
   a. Change `parseBitbucketPRID(prURL string) (int, error)` to `parseBitbucketPRID(ctx context.Context, prURL string) (int, error)`.
   b. Replace `fmt.Errorf("invalid PR ID %q in URL %q", ...)` with `errors.Errorf(ctx, "invalid PR ID %q in URL %q", ...)`.
   c. Replace `fmt.Errorf("could not extract PR ID from URL %q", prURL)` with `errors.Errorf(ctx, "could not extract PR ID from URL %q", prURL)`.
   d. Update all callers of `parseBitbucketPRID` (within the same file) to pass `ctx`.

3. In `pkg/executor/executor.go`:
   a. In `validateClaudeAuth`, change the parameter from `_ context.Context` to `ctx context.Context` (stop discarding the context).
   b. Replace `fmt.Errorf(...)` calls inside `validateClaudeAuth` with `errors.Errorf(ctx, ...)` or `errors.Wrapf(ctx, err, ...)` as appropriate.

4. Remove the `"fmt"` import from any file where it is no longer used after the above changes.

5. Add `"context"` and `"github.com/bborbe/errors"` imports to any file that needs them.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Errorf(ctx, ...)` and `errors.Wrapf(ctx, err, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- Never use `context.Background()` in pkg/ code
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
