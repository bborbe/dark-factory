---
status: completed
summary: Added RetryCount field to Frontmatter struct, RetryCount() getter, IncrementRetryCount to Manager interface and implementation, regenerated mocks, and added tests covering the new functionality.
container: dark-factory-101-spec-019-retry-count-frontmatter
dark-factory-version: v0.17.29
created: "2026-03-06T14:32:00Z"
queued: "2026-03-06T14:32:00Z"
started: "2026-03-06T14:32:00Z"
completed: "2026-03-06T14:38:06Z"
spec: ["019"]
---
<objective>
Add `retryCount` field to prompt frontmatter so the review-fix loop can track how many fix iterations have been attempted. Depends on spec-018-in-review-status-and-config.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/prompt/prompt.go for Frontmatter struct, getter/setter, and Manager interface patterns.
Follow the exact same pattern as the existing PRURL field and SetPRURL function.
</context>

<requirements>
1. Add `RetryCount int` to the `Frontmatter` struct (yaml: "retryCount,omitempty").

2. Add `RetryCount() int` getter on PromptFile returning `pf.Frontmatter.RetryCount`.

3. Add `IncrementRetryCount(ctx context.Context, path string) error` to the `Manager` interface and implement it:
   - Reads the file, increments `RetryCount` by 1, writes it back
   - Follow the same read-modify-write pattern as `SetPRURL`

4. Regenerate mocks with `go generate ./...`.

5. Add tests to `pkg/prompt/prompt_test.go`:
   - Parsing a prompt with `retryCount: 2` in frontmatter returns 2 from `RetryCount()`
   - Parsing a prompt with no `retryCount` returns 0
   - `IncrementRetryCount` increments from 0 to 1 and from 2 to 3
</requirements>

<constraints>
- Follow PRURL pattern exactly — getter, setter, manager method
- Do NOT modify existing tests
- Do NOT commit — dark-factory handles git
- Coverage must not decrease for pkg/prompt
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
