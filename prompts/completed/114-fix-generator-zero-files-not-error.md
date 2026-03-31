---
status: completed
spec: [020-auto-prompt-generation]
summary: SpecGenerator now checks completedDir for linked prompts before erroring on zero new files, skipping generation when spec is already covered
container: dark-factory-114-fix-generator-zero-files-not-error
dark-factory-version: v0.18.6
created: "2026-03-06T18:25:00Z"
queued: "2026-03-06T17:23:38Z"
started: "2026-03-06T17:23:38Z"
completed: "2026-03-06T17:32:19Z"
---

<objective>
Fix SpecGenerator to not treat zero new prompt files as an error when the spec already has completed prompts covering it.
</objective>

<context>
Read pkg/generator/generator.go — the Generate method counts inbox files before and after running the container, and returns an error if the delta is zero ("generation produced no prompt files"). This is wrong when the generator correctly decides no new prompts are needed because all acceptance criteria are already covered by existing completed prompts.

The generator should only error on zero files if there are NO linked completed prompts for the spec at all. If completed prompts already exist for this spec, zero new files is valid.
</context>

<requirements>
1. In pkg/generator/generator.go, update the zero-files check:
   - Before erroring, count how many completed prompts already have this spec linked (scan completedDir for files with matching spec field)
   - If completed prompts exist for the spec → log at Info level "spec already has completed prompts, skipping generation" and return nil
   - If no completed prompts exist AND zero new files → return error as before

2. Add test cases:
   - Zero new files + existing completed prompts for spec → no error, logs "skipping"
   - Zero new files + no completed prompts → error as before

3. Read pkg/prompt/prompt.go for how to parse spec field from frontmatter.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- make precommit must pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
