---
status: completed
summary: Added repo name validation using regexp before constructing gh api URL in collaborator_fetcher.go to prevent path traversal
container: dark-factory-exec-421-review-dark-factory-17-fix-repo-name-validation
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
started: "2026-05-25T18:50:47Z"
completed: "2026-05-25T18:54:03Z"
---

<summary>
- Added repo name validation before constructing gh api URLs
- Prevents potential path traversal through malicious repo names
</summary>

<objective>
Add validation for repo name before embedding in gh api URL.
</objective>

<context>
Files to read before making changes:
- `pkg/git/collaborator_fetcher.go` — lines 121-129 where repoName is embedded in URL
</context>

<requirements>
1. In `pkg/git/collaborator_fetcher.go`, add validation before the gh api call:
   ```go
   var repoNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+$`)
   if !repoNameRegexp.MatchString(repoName) {
       return nil, errors.Errorf(ctx, "invalid repo name %q", repoName)
   }
   ```

2. Ensure `regexp` and `github.com/bborbe/errors` are imported.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
