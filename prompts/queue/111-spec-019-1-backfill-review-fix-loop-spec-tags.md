---
status: executing
spec: ["019"]
container: dark-factory-111-spec-019-1-backfill-review-fix-loop-spec-tags
dark-factory-version: v0.18.2
created: "2026-03-06T17:30:00Z"
queued: "2026-03-06T16:41:33Z"
started: "2026-03-06T16:41:33Z"
---
<objective>
Backfill `spec: ["019"]` into the six completed prompts that implement the review-fix loop (spec 019), so the auto-complete mechanism can detect that all spec-019 prompts are done and mark the spec as completed.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read specs/019-review-fix-loop.md for the spec number ("019").
Read the following completed prompt files to see their current frontmatter before editing:
- prompts/completed/100-spec-019-in-review-status-and-config.md
- prompts/completed/101-spec-019-retry-count-frontmatter.md
- prompts/completed/102-spec-019-review-fetcher.md
- prompts/completed/103-spec-019-fix-prompt-generator.md
- prompts/completed/104-spec-019-review-poller.md
- prompts/completed/105-spec-019-wire-into-factory.md

These prompts implement the review-fix loop features (in_review status, retry count, review fetcher, fix prompt generator, review poller, wiring into factory) but were completed without a `spec:` frontmatter field.
</context>

<requirements>
For each of the six prompt files listed above:
1. Read the file's current frontmatter.
2. Add `spec: ["019"]` as a new frontmatter field, placed after the existing fields and before the closing `---`.
3. Do NOT modify any other frontmatter field (status, summary, container, timestamps, etc.).
4. Do NOT modify the prompt body below the closing `---`.

The resulting frontmatter for each file should include a line exactly like:
```
spec: ["019"]
```

After updating all six files, verify by reading back each file and confirming the `spec:` field is present.
</requirements>

<constraints>
- Read each file before editing — do not overwrite existing frontmatter
- Only add the `spec:` field — do not change status, summary, or any other fields
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
