---
status: completed
summary: Added Specs section to README.md listing all 16 spec files with their problem statements
container: dark-factory-067-add-spec-status-to-readme
dark-factory-version: v0.16.0
created: "2026-03-04T17:50:28Z"
queued: "2026-03-04T23:30:20Z"
started: "2026-03-04T23:30:20Z"
completed: "2026-03-04T23:31:45Z"
---
<objective>
Add a "Specs" section to README.md that lists all spec files from specs/ with their filename and Problem section as a one-line summary.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read ALL markdown files in ~/Documents/workspaces/coding-guidelines/ for Go patterns.
</context>

<requirements>
1. Add a "## Specs" section to README.md after the "## How It Works" section
2. List all spec files from specs/ directory
3. For each spec, show: number, title (derived from filename), and first sentence of the Problem section
4. Format as a markdown table: | Spec | Problem |
</requirements>

<constraints>
- Only modify README.md
- Do not modify any Go code
- Do not run make precommit (no code changes)
- Keep the section concise — one line per spec
</constraints>

<verification>
The README.md should contain a ## Specs section with a table listing all 12 specs.
</verification>

<success_criteria>
- README.md has a ## Specs section
- All 12 spec files are listed
- Table is readable and concise
</success_criteria>
