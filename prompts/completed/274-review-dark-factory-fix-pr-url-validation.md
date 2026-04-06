---
status: completed
summary: Added ValidatePRURL function to pkg/git/validate.go and guarded gh subprocess calls in pr_merger.go and review_fetcher.go with URL validation before each invocation.
container: dark-factory-274-review-dark-factory-fix-pr-url-validation
dark-factory-version: v0.104.2-dirty
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T17:05:26Z"
started: "2026-04-06T18:28:31Z"
completed: "2026-04-06T18:36:19Z"
---

<summary>
- PR URLs read from prompt frontmatter are passed directly to gh CLI subprocess calls
- There is no validation that the URL is a well-formed GitHub PR URL before use
- A manually edited frontmatter file could pass unexpected flags or paths to the gh binary
- Adding a ValidatePRURL function is consistent with the existing ValidateBranchName and ValidatePRTitle guards
- The fix adds validation and calls it before each gh subprocess invocation that takes a PR URL
</summary>

<objective>
Add a `ValidatePRURL(ctx context.Context, prURL string) error` function to `pkg/git/validate.go` that enforces a well-formed GitHub PR URL pattern, and call it in `pkg/git/pr_merger.go` and `pkg/git/review_fetcher.go` before passing the URL to `gh` CLI subprocess calls.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `pkg/git/validate.go` — existing `ValidateBranchName` and `ValidatePRTitle` functions; follow the same pattern
- `pkg/git/pr_merger.go` — `gh pr view` at ~line 84 and `gh pr merge` at ~line 105 using `prURL`
- `pkg/git/review_fetcher.go` — `gh pr view` at ~lines 74–83, 97–98 using `prURL`
- `pkg/git/pr_creator.go` — any additional locations where `prURL` is passed to subprocess calls
</context>

<requirements>
1. In `pkg/git/validate.go`:
   a. Add a package-level compiled regexp:
      ```go
      var prURLRegexp = regexp.MustCompile(`^https://github\.com/[^/\s]+/[^/\s]+/pull/[0-9]+$`)
      ```
   b. Add the function:
      ```go
      // ValidatePRURL returns an error if the given PR URL is not a valid GitHub pull request URL.
      func ValidatePRURL(ctx context.Context, prURL string) error {
          if !prURLRegexp.MatchString(prURL) {
              return errors.Errorf(ctx, "invalid PR URL %q: must be https://github.com/owner/repo/pull/N", prURL)
          }
          return nil
      }
      ```
   c. Add `"regexp"` to imports if not already present.

2. In `pkg/git/pr_merger.go`:
   - Before the `exec.CommandContext(ctx, "gh", "pr", "view", prURL, ...)` call (~line 84), add:
     ```go
     if err := ValidatePRURL(ctx, prURL); err != nil {
         return errors.Wrap(ctx, err, "validate PR URL")
     }
     ```
   - Apply the same guard before the `gh pr merge` call (~line 105).

3. In `pkg/git/review_fetcher.go`:
   - Add the same `ValidatePRURL` guard before each `gh pr view` call that takes `prURL`.

4. Add tests in the existing `pkg/git/` test files (or a new `validate_test.go`) covering:
   - Valid GitHub PR URL → no error.
   - URL with `--flag-like` value → returns error.
   - URL with HTTP instead of HTTPS → returns error.
   - Empty string → returns error.
   - Bitbucket-style URL → returns error.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
