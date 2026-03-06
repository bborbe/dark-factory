---
status: completed
summary: Created ReviewFetcher in pkg/git/review_fetcher.go with FetchLatestReview and FetchPRState methods, internal NDJSON parsing, mocks regenerated, and test coverage at 80.9%
container: dark-factory-102-spec-019-review-fetcher
dark-factory-version: v0.17.29
created: "2026-03-06T14:38:10Z"
queued: "2026-03-06T14:38:10Z"
started: "2026-03-06T14:38:10Z"
completed: "2026-03-06T14:49:44Z"
spec: ["019"]
---
<objective>
Create a `ReviewFetcher` that polls a GitHub PR for reviews from trusted reviewers and returns the latest verdict. This is the GitHub API layer for spec 018 — no processor wiring yet, just the fetching logic.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/git/pr_merger.go for the existing gh CLI subprocess patterns to follow exactly.
The fetcher uses `gh pr view` to get review state and reviewer login.
</context>

<requirements>
1. Create `pkg/git/review_fetcher.go` with:

   ```go
   // ReviewVerdict represents the outcome of a PR review.
   type ReviewVerdict string

   const (
       ReviewVerdictNone            ReviewVerdict = ""
       ReviewVerdictApproved        ReviewVerdict = "approved"
       ReviewVerdictChangesRequested ReviewVerdict = "changes_requested"
   )

   // ReviewResult holds the latest review from a trusted reviewer.
   type ReviewResult struct {
       Verdict ReviewVerdict
       Body    string // full review body text
   }
   ```

   Interface:
   ```go
   //counterfeiter:generate -o ../../mocks/review_fetcher.go --fake-name ReviewFetcher . ReviewFetcher
   type ReviewFetcher interface {
       // FetchLatestReview returns the latest review from a trusted reviewer.
       // Returns ReviewVerdictNone if no trusted review exists yet.
       FetchLatestReview(ctx context.Context, prURL string, allowedReviewers []string) (*ReviewResult, error)
   }
   ```

   Implementation using `gh pr view`:
   - Run: `gh pr view <prURL> --json reviews --jq '.reviews[] | {state: .state, author: .author.login, body: .body}'`
   - Parse the JSON output (one JSON object per line)
   - Filter to only reviews where author is in `allowedReviewers`
   - Take the LAST matching review (most recent)
   - Map state: `"APPROVED"` → `ReviewVerdictApproved`, `"CHANGES_REQUESTED"` → `ReviewVerdictChangesRequested`
   - Any other state → `ReviewVerdictNone`
   - If no trusted review found → return `&ReviewResult{Verdict: ReviewVerdictNone}`, nil

2. Also add `FetchPRState(ctx context.Context, prURL string) (string, error)` to the same interface:
   - Run: `gh pr view <prURL> --json state --jq '.state'`
   - Returns the raw state string: "OPEN", "MERGED", "CLOSED"
   - Used to detect externally merged/closed PRs

3. Create `pkg/git/review_fetcher_test.go` and test:
   - `FetchLatestReview` returns approved verdict when trusted reviewer approved
   - `FetchLatestReview` returns changes_requested when trusted reviewer requested changes
   - `FetchLatestReview` returns none when only untrusted reviewers
   - `FetchLatestReview` returns none when no reviews
   - `FetchPRState` returns correct state string

4. Regenerate mocks with `go generate ./...`.
</requirements>

<constraints>
- Use exec.CommandContext subprocess pattern from pkg/git/pr_merger.go exactly
- Add `// #nosec G204` on exec.CommandContext calls
- Do NOT modify existing files in pkg/git
- Do NOT commit — dark-factory handles git
- Coverage ≥ 80% for new file
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
