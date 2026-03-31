---
status: completed
spec: [017-continue-on-existing-branch]
summary: Added Branch field to Frontmatter struct, Branch()/PRURL() getters and SetBranch() method to PromptFile, SetBranch package-level function, SetBranch to Manager interface and manager implementation, regenerated mocks, and added tests covering all new functionality.
container: dark-factory-084-branch-frontmatter-field
dark-factory-version: v0.17.12
created: "2026-03-06T09:58:55Z"
queued: "2026-03-06T09:58:55Z"
started: "2026-03-06T09:58:55Z"
completed: "2026-03-06T10:05:38Z"
---
<objective>
Add a `branch` field to prompt frontmatter so prompts can declare an existing branch to run on. This is the data layer for spec 017 (continue on existing branch).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/prompt/prompt.go for the existing PromptFile struct and frontmatter field patterns.
Read pkg/prompt/prompt_test.go for existing test patterns.
</context>

<requirements>
1. Add `Branch string` field to the `Frontmatter` struct in `pkg/prompt/prompt.go` with YAML tag `branch,omitempty`.

2. Add a `Branch() string` getter to `PromptFile` that returns the frontmatter Branch value (empty string if unset). Follow the same pattern as the existing `PRURL()` getter.

3. Add a `SetBranch(ctx context.Context, path string, branch string) error` function to the prompt package that updates the `branch` field in a prompt file's frontmatter. Follow the exact same pattern as the existing `SetPRURL` function (read file → update frontmatter → write file).

4. Add `SetBranch` to the `Manager` interface in `pkg/prompt/prompt.go` and implement it on the concrete type. Follow the same pattern as `SetPRURL`.

5. Regenerate mocks with `go generate ./...`.

6. Add tests to `pkg/prompt/prompt_test.go`:
   - Parsing a prompt with `branch: dark-factory/042-add-feature` in frontmatter returns that value from `Branch()`
   - Parsing a prompt with no `branch` field returns empty string from `Branch()`
   - `SetBranch` writes the branch value into frontmatter correctly
</requirements>

<constraints>
- Follow existing patterns exactly — getter naming, function signatures, YAML tags
- Do NOT modify existing passing tests
- Do NOT commit — dark-factory handles git
- Coverage must not decrease for pkg/prompt
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
