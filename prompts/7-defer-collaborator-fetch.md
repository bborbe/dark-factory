---
status: created
created: "2026-03-11T16:45:24Z"
---

<summary>
- GitHub collaborator resolution no longer happens at application startup
- The review poller resolves collaborators lazily on its first polling iteration using the runtime context
- Factory functions no longer execute I/O or use `context.Background()`
- Startup is faster and more resilient to GitHub API failures
- `NewReviewPoller` accepts a `CollaboratorFetcher` interface instead of `[]string`
- A private `resolveReviewers` method caches the result after first call
</summary>

<objective>
Remove the `fetcher.Fetch(context.Background())` call from `CreateReviewPoller` in the factory. Instead, pass the `CollaboratorFetcher` interface to `NewReviewPoller` and resolve collaborators lazily inside `Run()` on the first iteration, using the real context.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/factory/factory.go` — find `CreateReviewPoller` (~line 326). It currently calls `fetcher.Fetch(context.Background())` at ~line 337, which executes a GitHub API call via the `gh` CLI during factory initialization.
Read `pkg/review/poller.go` — find `NewReviewPoller` (~line 32) and `reviewPoller` struct (~line 57). The constructor currently accepts `allowedReviewers []string`. This needs to change to accept a `git.CollaboratorFetcher` instead.
Read `pkg/git/collaborator_fetcher.go` — the `CollaboratorFetcher` interface with `Fetch(ctx context.Context) []string`.
</context>

<requirements>
1. In `pkg/review/poller.go`, change `NewReviewPoller` to accept `collaboratorFetcher git.CollaboratorFetcher` instead of `allowedReviewers []string`.

2. In `pkg/review/poller.go`, update the `reviewPoller` struct: replace the `allowedReviewers []string` field with `collaboratorFetcher git.CollaboratorFetcher` and add `allowedReviewers []string` as a lazily-populated cache field (unexported).

3. In `pkg/review/poller.go`, add a private method to resolve collaborators on first use:
   ```go
   func (p *reviewPoller) resolveReviewers(ctx context.Context) []string {
       if p.allowedReviewers == nil {
           p.allowedReviewers = p.collaboratorFetcher.Fetch(ctx)
       }
       return p.allowedReviewers
   }
   ```

4. Update all usages of `p.allowedReviewers` inside `poller.go` to call `p.resolveReviewers(ctx)` instead.

5. In `pkg/factory/factory.go`, update `CreateReviewPoller` (~line 326): remove the `fetcher.Fetch(context.Background())` call and pass the `fetcher` (type `git.CollaboratorFetcher`) directly to `review.NewReviewPoller`.

6. Update all tests in `pkg/review/poller_test.go` to pass a `CollaboratorFetcher` fake instead of a `[]string` slice. Use the existing Counterfeiter-generated `mocks.CollaboratorFetcher` fake.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Use `github.com/bborbe/errors` for any new error wrapping.
- The `CollaboratorFetcher` interface already has a counterfeiter mock at `mocks/collaborator-fetcher.go`.
- Do not change the `CollaboratorFetcher` interface itself.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
