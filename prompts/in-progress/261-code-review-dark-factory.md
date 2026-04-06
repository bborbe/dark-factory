---
status: executing
container: dark-factory-261-code-review-dark-factory
dark-factory-version: v0.104.0
created: "2026-04-06T14:34:46Z"
queued: "2026-04-06T14:38:43Z"
started: "2026-04-06T14:39:01Z"
---

<summary>
- Service reviewed using full automated code review with all specialist agents
- Fix prompts generated for each Critical or Important finding
- Each fix prompt is independently verifiable and scoped to one concern
- No code changes made — review-only prompt that produces fix prompts
- Clean services produce no fix prompts
</summary>

<objective>
Run a full code review of the dark-factory project root and generate a fix prompt for each Critical or Important finding.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/dod.md` for Definition of Done criteria (if exists).
Read `docs/code-review-prompts.md` for the code-review prompt pattern and conventions.

Read 3 recent completed prompts from `prompts/completed/` (highest-numbered) to understand prompt style and XML tag structure.

Service directory: root (`./`)
</context>

<requirements>

## 1. Read Config

Read `.dark-factory.yaml` to find `prompts.inboxDir` (default: `prompts`). Use this as the output directory for fix prompts.

## 2. Run Code Review

Run `/coding:code-review full .` to get a comprehensive review with all specialist agents.

Collect the consolidated findings categorized as:
- **Must Fix (Critical)** — will generate fix prompts
- **Should Fix (Important)** — will generate fix prompts
- **Nice to Have** — skip, do NOT generate prompts

## 3. Generate Fix Prompts

For each Critical or Important finding (or group of related findings in the same file/package), write a prompt file to the prompts inbox directory.

**Filename:** `review-dark-factory-<fix-description>.md`

Each fix prompt must follow this exact structure:

```
---
status: draft
created: "<current UTC timestamp in ISO8601>"
---

<summary>
5-10 plain-language bullets. No file paths, struct names, or function signatures.
</summary>

<objective>
What to fix and why (1-3 sentences). End state, not steps.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- list specific files with line numbers as hints
</context>

<requirements>
Numbered, specific, unambiguous steps.
Anchor by function/type name (~line N as hint only).
Include function signatures where helpful.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
```

**Grouping rules:**
- One concern per prompt (e.g., "fix error wrapping in package X")
- Group coupled findings that must change together
- Split unrelated findings into separate prompts
- If order matters, prefix filenames with `1-`, `2-`, `3-`

## 4. Summary

Print a summary of findings and generated prompt files.

</requirements>

<constraints>
- Do NOT modify any source code — this is a review-only prompt
- Only write files to the prompts inbox directory
- Never write to `in-progress/` or `completed/` subdirectories
- Never assign execution numbers to filenames — dark-factory assigns those on approve. Ordering prefixes (`1-`, `2-`, `3-`) for dependent prompts are fine.
- Repo-relative paths only in generated prompts (no absolute, no `~/`)
- If no findings at Critical/Important level — report clean bill of health, generate no prompts
</constraints>

<verification>
After generating fix prompts, list them:
ls prompts/review-dark-factory-*.md
</verification>
