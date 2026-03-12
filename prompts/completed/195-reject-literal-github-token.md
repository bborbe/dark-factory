---
status: completed
summary: Added validateGitHubToken method that rejects literal token values, accepting only empty strings or ${VAR_NAME} env var references, and updated tests accordingly.
container: dark-factory-195-reject-literal-github-token
dark-factory-version: v0.48.0
created: "2026-03-11T20:22:56Z"
queued: "2026-03-11T20:22:56Z"
started: "2026-03-12T01:42:07Z"
completed: "2026-03-12T01:47:45Z"
---

<summary>
- Literal GitHub tokens in config are rejected at validation time
- Only environment variable references like ${GITHUB_TOKEN} are accepted
- Prevents accidental token leakage via git history
- Clear error message guides users to use env var references
- Existing env var resolution continues to work unchanged
</summary>

<objective>
Reject literal GitHub token values in `.dark-factory.yaml` to prevent accidental credential leakage. The `github.token` field should only accept `${VAR_NAME}` references, never plaintext tokens.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — find the `GitHubConfig` struct (has `Token string` field) and the `Validate` method on `Config`.
Read `pkg/config/config.go` — find `resolveEnvVar` which handles `${VAR}` expansion.
Read `pkg/config/config_test.go` — find existing tests for GitHub token validation.
</context>

<requirements>
1. In `pkg/config/config.go`, add a `validateGitHubToken` method on `Config` that:
   - Returns nil if `GitHub.Token` is empty (no token configured)
   - Returns nil if `GitHub.Token` matches the pattern `^\$\{[A-Za-z_][A-Za-z0-9_]*\}$` (env var reference)
   - Returns a descriptive error otherwise: `"github.token must be an env var reference like ${GITHUB_TOKEN}, not a literal value"`
2. Call `validateGitHubToken` from the existing `Validate` method
3. Update `pkg/config/config_test.go`:
   - Add test: literal token string `"ghp_abc123"` → validation error
   - Add test: env var reference `"${GITHUB_TOKEN}"` → no error
   - Add test: empty string → no error
   - Update the existing test that uses a literal `"literal-token"` to use `"${GITHUB_TOKEN}"` instead
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass (update the literal-token test)
- Do not change the `resolveEnvVar` function — it handles runtime resolution
- This is compile-time/load-time validation only
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
