---
created: "2026-03-06T13:21:49Z"
---
<objective>
Change the `spec` frontmatter field in prompts from a single string to an array of strings, so a prompt can belong to multiple specs. Update the PROMPTS counter in spec list to count prompts that include the spec's number in their `spec` array.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/prompt/prompt.go for the Frontmatter struct and existing Spec field (added in a previous prompt).
Read pkg/spec/ for how spec prompt counts are currently computed.
</context>

<requirements>
1. In `pkg/prompt/prompt.go`, change the `Spec` field in `Frontmatter` from `string` to `[]string`:
   ```go
   Specs []string `yaml:"spec,omitempty,flow"`
   ```
   Update the `Spec() string` getter to `Specs() []string` returning the slice (nil → empty slice).
   Add a `HasSpec(id string) bool` helper that returns true if the id is in the Specs slice.

2. Update any existing callers of `Spec()` to use `Specs()` or `HasSpec()`.

3. Update the prompt count logic in `pkg/spec/` to use `HasSpec(specNumber)` when scanning prompts.
   The spec number is the NNN prefix of the spec filename (e.g. `"017"` from `017-continue-on-existing-branch.md`).

4. Update mocks with `go generate ./...`.

5. Update tests:
   - `spec: "017"` (single string) parses as `["017"]` — backward compatible
   - `spec: ["017", "019"]` (array) parses as `["017", "019"]`
   - `HasSpec("017")` returns true when "017" is in the array
   - Prompt count for spec 017 includes prompts with `spec: ["017"]` and `spec: ["017", "019"]`
</requirements>

<constraints>
- Single string `spec: "017"` must still parse correctly (YAML treats single string as scalar, not array — handle both)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
