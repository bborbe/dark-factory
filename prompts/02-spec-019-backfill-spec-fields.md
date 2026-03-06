<objective>
Backfill the `spec` frontmatter field in all completed prompts that belong to a known spec, so the PROMPTS counter in `dark-factory spec list` shows correct counts instead of `0/0`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read specs/ to understand which spec covers which work area.
Read prompts/completed/ to see existing prompts and their current frontmatter.
The spec number is the 3-digit prefix of the spec filename (e.g. "017").
Use the prompt content and filename to determine which spec(s) it belongs to.
</context>

<requirements>
Add `spec:` frontmatter to the following completed prompts. Use array format `spec: ["NNN"]` for single spec, `spec: ["NNN", "MMM"]` for multiple.

Mapping (read each prompt file to confirm before editing):

- `084-branch-frontmatter-field.md` → spec: ["017"]
- `085-brancher-fetch-and-verify.md` → spec: ["017"]
- `086-wire-existing-branch-into-processor.md` → spec: ["017"]
- `087-spec-018-spec-model.md` → spec: ["018"]
- `089-spec-018-two-level-commands.md` → spec: ["018"]
- `090-spec-018-spec-commands.md` → spec: ["018"]
- `091-spec-018-prompt-spec-field.md` → spec: ["018"]
- `092-spec-018-combined-views.md` → spec: ["018"]
- `093-spec-018-auto-complete-spec.md` → spec: ["018"]

For each file: read the current frontmatter, add the `spec:` field after the existing fields, write back. Do not change any other frontmatter fields or the prompt body.

After updating all files, run `dark-factory list` (or equivalent) to verify the PROMPTS counts are no longer `0/0` for specs 017 and 018.
</requirements>

<constraints>
- Read each file before editing — do not overwrite existing frontmatter
- Only add the `spec:` field — do not modify status, summary, or any other fields
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
