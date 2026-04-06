---
status: approved
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T20:04:24Z"
---

<summary>
- Review bodies from pull requests are inserted verbatim into generated prompt files
- A malicious reviewer could craft a review body that breaks out of the intended structure
- This could cause the agent to execute attacker-controlled instructions
- The fix sanitizes the review body by stripping or escaping markup tags before insertion
- A helper function keeps the sanitization logic testable and reusable
</summary>

<objective>
Add a `sanitizeReviewBody` function in `pkg/review/fix_prompt_generator.go` that strips or escapes XML/HTML-like tags from the PR review body before it is embedded into the generated prompt file. Call this function on `opts.ReviewBody` before insertion to prevent prompt injection attacks.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `pkg/review/fix_prompt_generator.go` — the `Generate` function (~lines 51–63) where `opts.ReviewBody` is embedded into the prompt content; understand the surrounding template structure
- `pkg/review/fix_prompt_generator_test.go` — understand the existing test structure to add tests for the sanitization
</context>

<requirements>
1. Add a `sanitizeReviewBody(body string) string` function in `pkg/review/fix_prompt_generator.go` that:
   a. Strips or escapes XML/HTML-like open and close tags (e.g., `<foo>`, `</foo>`, `<foo/>`) that could break out of the surrounding `<review_feedback>` context frame.
   b. A safe approach: use `regexp.MustCompile(`<[^>]+>`)` to replace all angle-bracket tag patterns with their escaped equivalent `&lt;...&gt;`, or simply strip them.
   c. The function must preserve all other content unchanged (code snippets, plain text, markdown, backticks, etc.).

2. In `Generate` (or whatever function inserts `opts.ReviewBody` into the prompt string):
   - Change the insertion to: `sanitizeReviewBody(opts.ReviewBody)`.

3. Add tests in `pkg/review/fix_prompt_generator_test.go` using the existing Ginkgo/Gomega pattern (external test package `review_test`, `Describe`/`It` blocks) covering:
   - Review body with no tags → returned unchanged.
   - Review body containing `<requirements>` tag → tag is stripped or escaped.
   - Review body containing `</review_feedback>` → tag is stripped or escaped.
   - Review body with code in backticks → code is preserved unchanged.
   - Empty review body → returns empty string.

4. Compile the regexp at package level (not inside the function) for efficiency:
   ```go
   var xmlTagPattern = regexp.MustCompile(`<[^>]{0,100}>`)
   ```
   Use a bounded quantifier `{0,100}` to avoid catastrophic backtracking on adversarial input.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- New tests for sanitizeReviewBody must pass
- All paths are repo-relative
- Do not parse HTML — a simple regexp strip is sufficient for this threat model
</constraints>

<verification>
make precommit
</verification>
