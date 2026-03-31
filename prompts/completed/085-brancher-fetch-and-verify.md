---
status: completed
spec: [017-continue-on-existing-branch]
summary: Added FetchAndVerifyBranch method to Brancher interface with implementation (git fetch + rev-parse verify), regenerated mocks, and added tests for success and error cases.
container: dark-factory-085-brancher-fetch-and-verify
dark-factory-version: v0.17.12
created: "2026-03-06T10:05:42Z"
queued: "2026-03-06T10:05:42Z"
started: "2026-03-06T10:05:42Z"
completed: "2026-03-06T10:10:57Z"
---
<objective>
Add a `FetchAndVerifyBranch` method to the Brancher interface that fetches from origin and verifies a branch exists remotely. This is needed so the processor can fail fast with a clear error when a declared branch does not exist (spec 017).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/git/brancher.go for the existing Brancher interface and implementation patterns.
Read pkg/git/brancher_test.go for existing test patterns.
</context>

<requirements>
1. Add `FetchAndVerifyBranch(ctx context.Context, branch string) error` to the `Brancher` interface in `pkg/git/brancher.go`.

2. Implement `FetchAndVerifyBranch` on the `brancher` struct:
   - First run `git fetch origin` (no specific branch — fetch all)
   - Then run `git rev-parse --verify origin/<branch>` to confirm the branch exists remotely
   - If `rev-parse` fails → return a descriptive error: `"branch not found at origin: <branch>"`
   - Add `// #nosec G204` comment on exec.CommandContext calls (branch name comes from validated frontmatter)

3. Regenerate mocks with `go generate ./...`.

4. Add tests to `pkg/git/brancher_test.go`:
   - `FetchAndVerifyBranch` returns nil when branch exists (mock git succeeds)
   - `FetchAndVerifyBranch` returns error containing branch name when `rev-parse` fails
</requirements>

<constraints>
- Follow existing subprocess patterns in brancher.go exactly (exec.CommandContext, slog.Debug, errors.Wrap)
- Do NOT modify existing passing tests
- Do NOT commit — dark-factory handles git
- Coverage must not decrease for pkg/git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
