---
status: completed
summary: Extracted fetchCollaborators from factory into a dedicated CollaboratorFetcher interface and implementation in pkg/git/collaborator_fetcher.go, with counterfeiter mock generated and factory updated to use zero-logic wiring
container: dark-factory-149-extract-fetch-collaborators-from-factory
dark-factory-version: v0.30.3
created: "2026-03-08T21:12:08Z"
queued: "2026-03-08T23:18:05Z"
started: "2026-03-09T00:04:11Z"
completed: "2026-03-09T00:14:45Z"
---

<summary>
- GitHub collaborator resolution logic extracted from factory into dedicated type
- Factory becomes pure wiring with zero business logic
- New `CollaboratorFetcher` interface enables mocking in tests
- `NewReviewPoller` signature unchanged — factory calls `fetcher.Fetch()` and passes result
</summary>

<objective>
Extract `fetchCollaborators` from the factory into a dedicated `CollaboratorFetcher` type in `pkg/git/`, eliminating business logic from the factory layer.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/factory/factory.go` — `fetchCollaborators` function (lines ~242-287) and its caller `CreateReviewPoller` (lines ~219-238).
Read `pkg/review/poller.go` — `ReviewPoller` interface and `NewReviewPoller` constructor to understand what accepts the collaborators list.
Read `/home/node/.claude/docs/go-factory-pattern.md` — zero business logic in factories.
Read `/home/node/.claude/docs/go-patterns.md` — interface + constructor pattern.
</context>

<requirements>
1. Create `pkg/git/collaborator_fetcher.go`:

   ```go
   // CollaboratorFetcher fetches GitHub repository collaborators.
   //
   //counterfeiter:generate -o ../../mocks/collaborator-fetcher.go --fake-name CollaboratorFetcher . CollaboratorFetcher
   type CollaboratorFetcher interface {
       Fetch(ctx context.Context) []string
   }
   ```

   Private struct:
   ```go
   type collaboratorFetcher struct {
       ghToken          string
       useCollaborators bool
       allowedReviewers []string
   }
   ```

   Constructor:
   ```go
   // NewCollaboratorFetcher creates a CollaboratorFetcher.
   // If useCollaborators is false or allowedReviewers is non-empty, collaborators are not fetched from GitHub.
   func NewCollaboratorFetcher(
       ghToken string,
       useCollaborators bool,
       allowedReviewers []string,
   ) CollaboratorFetcher {
       return &collaboratorFetcher{
           ghToken:          ghToken,
           useCollaborators: useCollaborators,
           allowedReviewers: allowedReviewers,
       }
   }
   ```

   `Fetch` method — move the logic from `fetchCollaborators` in factory.go:
   ```go
   func (f *collaboratorFetcher) Fetch(ctx context.Context) []string {
       if len(f.allowedReviewers) > 0 {
           return f.allowedReviewers
       }
       if !f.useCollaborators {
           return nil
       }
       // ... existing gh CLI logic from fetchCollaborators ...
   }
   ```

2. Update `pkg/factory/factory.go`:
   - Remove `fetchCollaborators` function entirely
   - In `CreateReviewPoller`, create the fetcher and call `Fetch` at startup:
     ```go
     fetcher := git.NewCollaboratorFetcher(ghToken, cfg.UseCollaborators, cfg.AllowedReviewers)
     allowedReviewers := fetcher.Fetch(context.Background())
     ```
   - `NewReviewPoller` signature stays unchanged — it still receives `[]string`
   - Add `"context"` import if missing

3. Do NOT change `pkg/review/poller.go` — `NewReviewPoller` signature stays the same.

4. Run `make generate` to create the counterfeiter mock.

5. Add copyright header to the new file (copy format from `pkg/git/git.go` L1-3).
</requirements>

<constraints>
- The `ReviewPoller` interface must NOT change
- Move ALL logic from `fetchCollaborators` — no business logic remains in factory
- The new `Fetch` method should handle errors gracefully (log warning, return nil) — matching current behavior
- `#nosec G204` annotations on `exec.CommandContext` calls
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Verify factory has no business logic:
```bash
grep -n "exec.Command\|os.Environ\|strings.Split\|strings.TrimSpace" pkg/factory/factory.go
# Expected: no output
```

Verify new file exists:
```bash
ls pkg/git/collaborator_fetcher.go mocks/collaborator-fetcher.go
# Expected: both exist
```

Verify fetchCollaborators removed from factory:
```bash
grep -n "fetchCollaborators" pkg/factory/factory.go
# Expected: no output
```
</verification>

<success_criteria>
- `fetchCollaborators` removed from `pkg/factory/factory.go`
- `CollaboratorFetcher` interface + implementation in `pkg/git/collaborator_fetcher.go`
- Counterfeiter mock generated
- Factory wires the new type with zero logic
- `make precommit` passes
</success_criteria>
