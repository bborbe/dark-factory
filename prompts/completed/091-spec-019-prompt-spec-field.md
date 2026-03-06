---
status: completed
summary: Added spec field to prompt frontmatter, prompt.Counter interface for counting prompts by spec, updated spec_list.go and spec_status.go to show linked prompt counts, added LinkedPromptsCompleted/LinkedPromptsTotal to spec.Summary, wired factory, and added tests.
container: dark-factory-091-spec-019-prompt-spec-field
dark-factory-version: v0.17.15
created: "2026-03-06T10:57:15Z"
queued: "2026-03-06T10:57:15Z"
started: "2026-03-06T11:38:40Z"
completed: "2026-03-06T11:51:17Z"
---
<objective>
Add a `spec` field to prompt frontmatter that links a prompt to its parent spec. This enables spec-prompt linkage for status tracking and auto-completion.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/prompt/prompt.go for the Frontmatter struct and how fields are parsed/saved.
Read pkg/spec/spec.go for the spec model.
Read pkg/cmd/spec_list.go for how specs are listed (to add linked prompt counts).
</context>

<requirements>
1. Add `Spec string \`yaml:"spec,omitempty"\`` to the `Frontmatter` struct in `pkg/prompt/prompt.go`.

2. Update `pkg/cmd/spec_list.go` to show linked prompt counts per spec:
   - After loading specs, scan prompts (inbox + queue + completed) for matching `spec` field
   - Output format: `STATUS     PROMPTS  FILE` where PROMPTS shows "3/5" (completed/total)
   - Example: `completed  5/5      001-core-pipeline.md`
   - Example: `approved   0/0      017-continue-on-existing-branch.md`

3. Update `pkg/cmd/spec_status.go` to include prompt linkage info:
   - Add total linked prompts count and completed count to summary output
   - Example: `Specs: 19 total (2 draft, 3 approved, 0 prompted, 14 completed) | Linked prompts: 77/80`

4. Add tests:
   - Prompt with `spec: "017"` field loads and saves correctly
   - Spec list shows correct prompt counts
   - Prompt without `spec` field still works (backward compatible)
</requirements>

<constraints>
- The `spec` field is optional — prompts without it are valid
- The field value matches the spec number as string (e.g. "017", "019")
- Existing prompts are not modified (field added only to new prompts going forward)
- `make precommit` must pass
- Do NOT commit, tag, or push
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
