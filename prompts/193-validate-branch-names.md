---
status: approved
created: "2026-03-11T20:22:56Z"
queued: "2026-03-11T20:22:56Z"
---

<summary>
- Branch names are validated before use in git commands
- A regex allowlist rejects names with special characters or leading dashes
- PR titles are validated to prevent argument injection
- All git operations that accept external names go through validation
- Existing tests continue to pass
</summary>

<objective>
Add branch name and PR title sanitization to prevent argument injection via crafted YAML frontmatter. Currently, branch names from prompt frontmatter are passed directly to exec.CommandContext without validation.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/git/brancher.go` — the `Brancher` interface and `brancher` struct. Methods like `CreateAndSwitch`, `Push`, `Switch`, `FetchAndVerifyBranch` all accept a `name string` parameter that gets passed to git commands.
Read `pkg/git/pr_creator.go` — the `Create` method passes a title to `gh pr create --title`.
Read `pkg/processor/processor.go` — where branch names are constructed (look for `branchName` or `dark-factory/` prefix).
</context>

<requirements>
1. Add a `ValidateBranchName(name string) error` function in `pkg/git/validate.go` (new file)
2. The function must enforce: regex `^[a-zA-Z0-9][a-zA-Z0-9/_.-]*$` — alphanumeric start, safe characters only
3. Add a `ValidatePRTitle(title string) error` function in the same file — reject empty titles and titles starting with `-`
4. Call `ValidateBranchName` at the top of `brancher.CreateAndSwitch`, `brancher.Push`, `brancher.Switch`, `brancher.FetchAndVerifyBranch`, and `brancher.MergeToDefault` — return wrapped error on failure
5. Call `ValidatePRTitle` at the top of `prCreator.Create` — return wrapped error on failure
6. Add tests in `pkg/git/validate_test.go` covering: valid names, leading dash, special chars (`; rm -rf`), `--orphan`, empty string, names with `/` (valid for `dark-factory/feature`)
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do not change the `Brancher` or `PRCreator` interfaces
- Keep validation logic in a separate file (`validate.go`) not inline in each method
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
