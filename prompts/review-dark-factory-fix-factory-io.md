---
status: draft
created: "2026-04-06T00:00:00Z"
---

<summary>
- The factory package performs live HTTP calls at object graph construction time
- One function fetches the current Bitbucket user over HTTP during factory wiring
- Another function eagerly fetches collaborator reviewers during factory construction
- Executing I/O at wire time makes startup fail if the remote is unreachable
- Reviewer data fetched at startup is never refreshed, causing stale reviewer lists
- The fix moves HTTP logic to pkg/git and makes reviewer resolution lazy
</summary>

<objective>
Extract `fetchBitbucketCurrentUser` from `pkg/factory/factory.go` into a proper `CurrentUserFetcher` interface and implementation in `pkg/git/`, and change `collaboratorFetcher.Fetch(ctx)` in `createBitbucketProviderDeps` to be called lazily (at PR creation time) rather than eagerly at factory construction time.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` for factory rules.

Files to read before making changes (read ALL first):
- `pkg/factory/factory.go` — `fetchBitbucketCurrentUser` function (~lines 182–207) and `createBitbucketProviderDeps` function (~lines 134–180); understand what `currentUser` and `reviewers` are used for and which struct receives them
- `pkg/git/bitbucket_http.go` — existing Bitbucket HTTP utilities; understand the HTTP client and auth pattern already in use
- `pkg/git/bitbucket_pr_creator.go` — the struct that ultimately uses `currentUser` and `reviewers`; check whether it could fetch these lazily
</context>

<requirements>
1. Create `pkg/git/bitbucket_current_user_fetcher.go` with:
   ```go
   //counterfeiter:generate -o ../../mocks/bitbucket-current-user-fetcher.go --fake-name BitbucketCurrentUserFetcher . BitbucketCurrentUserFetcher

   // BitbucketCurrentUserFetcher fetches the current authenticated Bitbucket user.
   type BitbucketCurrentUserFetcher interface {
       FetchCurrentUser(ctx context.Context) string
   }

   // NewBitbucketCurrentUserFetcher creates a BitbucketCurrentUserFetcher for the given base URL and token.
   func NewBitbucketCurrentUserFetcher(baseURL, token string) BitbucketCurrentUserFetcher {
       return &bitbucketCurrentUserFetcher{baseURL: baseURL, token: token}
   }

   type bitbucketCurrentUserFetcher struct {
       baseURL string
       token   string
   }

   func (f *bitbucketCurrentUserFetcher) FetchCurrentUser(ctx context.Context) string {
       // move the HTTP logic from factory.fetchBitbucketCurrentUser here
   }
   ```

2. In `pkg/factory/factory.go`:
   a. Remove the `fetchBitbucketCurrentUser` function entirely.
   b. In `createBitbucketProviderDeps`, replace the direct `fetchBitbucketCurrentUser(ctx, baseURL, token)` call with `git.NewBitbucketCurrentUserFetcher(baseURL, token)` to create a fetcher object. Pass the fetcher to the PR creator constructor instead of the resolved string.
   c. Change `collaboratorFetcher.Fetch(ctx)` — remove this eager call. Instead, pass `collaboratorFetcher` itself to the PR creator so it can call `Fetch` lazily when a PR is actually being created.

3. In `pkg/git/bitbucket_pr_creator.go`:
   a. Change the `currentUser string` field/parameter to `currentUserFetcher git.BitbucketCurrentUserFetcher`.
   b. Change the `allowedReviewers []string` field/parameter to accept the `collaboratorFetcher` (or a combined interface).
   c. Call `f.currentUserFetcher.FetchCurrentUser(ctx)` and `f.collaboratorFetcher.Fetch(ctx)` at PR creation time (inside the method that creates a PR), not at construction time.

4. Run `make generate` to regenerate the counterfeiter mock for `BitbucketCurrentUserFetcher`.

5. Update any tests that construct `BitbucketPRCreator` directly to pass the new fetcher types instead of raw strings.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Factory functions must perform zero I/O — all HTTP calls must be in pkg/ types called at use time
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf`
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
