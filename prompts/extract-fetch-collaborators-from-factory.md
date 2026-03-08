---
status: created
---

<objective>
Extract `fetchCollaborators` from `pkg/factory/factory.go` into a proper type in `pkg/git/`. This function executes `gh` CLI commands to resolve GitHub collaborators — it's business/infrastructure logic that violates the zero-logic factory rule.
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

2. Update `pkg/review/poller.go`:
   - Change `NewReviewPoller` to accept `CollaboratorFetcher` instead of a `[]string` for allowed reviewers (if currently accepting `[]string`)
   - OR: keep `NewReviewPoller` accepting `[]string` and have the factory call `fetcher.Fetch(ctx)` — whichever requires fewer changes

3. Update `pkg/factory/factory.go`:
   - Remove `fetchCollaborators` function entirely
   - In `CreateReviewPoller`, create and use the new fetcher:
     ```go
     fetcher := git.NewCollaboratorFetcher(ghToken, cfg.UseCollaborators, cfg.AllowedReviewers)
     // Pass fetcher or fetcher.Fetch(ctx) depending on approach from step 2
     ```

4. Run `make generate` to create the counterfeiter mock.

5. Add copyright header to the new file.
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
