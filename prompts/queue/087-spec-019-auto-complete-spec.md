---
status: executing
container: dark-factory-087-spec-019-auto-complete-spec
dark-factory-version: v0.17.12
created: "2026-03-06T10:57:15Z"
queued: "2026-03-06T10:57:15Z"
started: "2026-03-06T10:57:18Z"
---
<objective>
Auto-complete a spec when all its linked prompts are completed. When the processor finishes a prompt and moves it to completed, check if all prompts referencing the same spec are now completed — if so, mark the spec as completed.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/processor/processor.go for where prompts are marked completed (look for StatusCompleted).
Read pkg/prompt/prompt.go for the Frontmatter struct and the `spec` field.
Read pkg/spec/spec.go for how to load and save specs.
Read pkg/spec/lister.go for how to list specs.
</context>

<requirements>
1. After a prompt is marked completed in the processor, check:
   - Does the completed prompt have a `spec` field?
   - If yes, scan all prompts (queue + completed) for the same `spec` value
   - If ALL linked prompts have status `completed`, load the spec file and set its status to `completed`
   - Log: `slog.Info("spec auto-completed", "spec", specID)`

2. Create a `spec.AutoCompleter` interface:
   - `CheckAndComplete(ctx context.Context, specID string) error`
   - Implementation scans prompt directories for matching `spec` field
   - Only completes if ALL linked prompts are completed (not just the current one)
   - No-op if spec is already completed
   - No-op if specID is empty

3. Wire `AutoCompleter` into processor:
   - Add to processor struct and constructor
   - Call after `moveToCompleted` succeeds
   - Pass `pf.Frontmatter.Spec` to `CheckAndComplete`

4. Add tests:
   - All linked prompts completed → spec marked completed
   - Some linked prompts still in queue → spec NOT completed
   - Prompt without spec field → no action
   - Spec already completed → no-op
</requirements>

<constraints>
- Auto-completion is best-effort — failure to complete spec must NOT fail the prompt
- Log warning on error, do not propagate
- `make precommit` must pass
- Do NOT commit, tag, or push
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
