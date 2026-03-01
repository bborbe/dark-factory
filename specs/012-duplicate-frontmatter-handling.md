---
status: completed
---

# Strip Duplicate Frontmatter from Prompt Content

## Problem

Prompts created with empty frontmatter (`---\n---`) end up with a duplicate frontmatter block in the content body after the processor prepends real frontmatter. The YOLO container receives `---` as the first line of the prompt and interprets it as a CLI flag, failing with `error: unknown option '---'`.

## Goal

When extracting prompt content (body after frontmatter), strip any additional empty or duplicate frontmatter blocks from the beginning of the content.

## Non-goals

- No validation of frontmatter content (only strip empty/duplicate blocks)
- No modification of the file on disk (stripping happens at read time)
- No prevention at write time (handle gracefully at read time)

## Desired Behavior

1. After splitting off the YAML frontmatter, examine the remaining content
2. If content starts with an empty frontmatter block (`---\n---\n` or `---\n[whitespace lines]\n---\n`), strip it
3. Trim leading whitespace after stripping
4. Return clean content to the executor

## Constraints

- Only strip frontmatter blocks where all lines between `---` delimiters are empty or whitespace
- Non-empty frontmatter blocks in content are left as-is (could be intentional markdown)
- Stripping is read-time only — original file is not modified

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| No duplicate frontmatter | Content returned as-is | None needed |
| Multiple duplicate blocks | Only first one stripped (unlikely edge case) | None needed |
| Content is entirely empty after stripping | Treated as empty prompt (moved to completed without execution) | None needed |

## Acceptance Criteria

- [ ] `---\n---\n# Title` produces `# Title` as content
- [ ] `---\n  \n---\n# Title` (whitespace between delimiters) also stripped
- [ ] Valid frontmatter-like content (`---\nkey: value\n---`) is NOT stripped
- [ ] Normal prompts without duplicate frontmatter are unaffected

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Every prompt with empty frontmatter fails with `error: unknown option '---'`. User must manually edit the file to remove the duplicate block. Breaks unattended operation.
