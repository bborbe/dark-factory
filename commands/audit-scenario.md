---
description: Audit dark-factory scenario file against Scenario Writing Guide
argument-hint: <scenario-file-path>
---

Audit the dark-factory scenario at $ARGUMENTS against `docs/scenario-writing.md`.

1. Parse scenario path from $ARGUMENTS
   - If no path prefix, prepend `scenarios/`
   - If no `.md` extension, append it
2. Read the scenario file
3. Read `docs/scenario-writing.md` for the writing rules and format
4. Evaluate:
   - **Frontmatter**: Has explicit `status` field (`idea`, `draft`, `active`, `outdated`)
   - **Description**: Has one-sentence description line after title (starts with "Validates that...")
   - **Structure**: Has Setup, Action, Expected sections (Cleanup optional)
   - **Title**: Uses `# Scenario NNN:` format with numeric prefix
   - **Checkboxes**: All items in Setup/Action/Expected use `- [ ]` format
   - **Observable outcomes**: Expected section tests files, git state, CLI output — not internals
   - **Self-contained**: Setup creates its own preconditions
   - **One journey**: Does not mix happy and failure paths
   - **Length**: Under 20 checkboxes total
5. Report findings with severity levels, scores, and recommendations
