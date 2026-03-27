---
description: Create a dark-factory scenario file for end-to-end validation
argument-hint: <feature-or-scenario-description>
allowed-tools: [Read, Write, Glob, Bash, AskUserQuestion]
---

Create a dark-factory scenario following `docs/scenario-writing.md`.

1. Read `docs/scenario-writing.md` for format, status lifecycle, and writing rules
2. Determine next scenario number from existing files in `scenarios/`
3. Gather requirements from $ARGUMENTS (or interactively if empty)
4. Write scenario file to `scenarios/NNN-<name>.md` with:
   - Frontmatter: `status: idea` (or `draft` if steps are provided)
   - Title: `# <Short description>`
   - Description: one sentence starting with "Validates that..."
   - Setup, Action, Expected sections (if `draft`)
   - Optional Cleanup section
5. Validate against writing rules (observable outcomes, self-contained, one journey, under 20 checkboxes)
