---
status: draft
created: "2026-01-01T00:00:00Z"
---

<summary>
- Reads the Makefile and adds a word count comment at the top
- Simple smoke test to verify the dark-factory pipeline works
</summary>

<objective>
Read the project's Makefile and print the total word count as a comment at the top.
</objective>

<requirements>
1. Read `Makefile`, count words, add a comment `# Word count: N` as the first line
2. If word count comment already exists, update it (don't duplicate)
</requirements>

<constraints>
- Run `make precommit` for validation only
- Do NOT commit, tag, or push
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
