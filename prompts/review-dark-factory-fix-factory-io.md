---
status: draft
created: "2026-04-06T00:00:00Z"
---

<summary>
- The factory performs live HTTP calls during object graph construction
- One call fetches the current Bitbucket user over HTTP at startup
- Another eagerly fetches collaborator reviewers before any PR is created
- Startup fails if the remote is unreachable, even if no PR will be created
- Reviewer data fetched at startup grows stale and is never refreshed
- The fix moves HTTP logic out of the factory and makes resolution lazy at PR creation time
</summary>

<objective>
Move the Bitbucket current-user HTTP call out of the factory into a lazy fetcher in the git package, and defer collaborator-reviewer resolution from factory construction to PR creation time. The factory must perform zero I/O.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read the coding plugin's `go-factory-pattern.md` guide for factory rules (zero I/O in constructors).

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
   a. The struct has a `reviewers []string` field (no `currentUser` field). Change `reviewers []string` to accept a reviewer-fetching interface (e.g., `collaboratorFetcher` or a `ReviewerFetcher` interface) so reviewers are resolved lazily at PR creation time.
   b. In the PR creation method, call the fetcher to get the reviewer list instead of using the pre-resolved `[]string`.
   c. The `currentUser` string is passed to `NewBitbucketCollaboratorFetcher` in the factory — change the factory to pass the `BitbucketCurrentUserFetcher` instead, and let the collaborator fetcher call it lazily.

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
