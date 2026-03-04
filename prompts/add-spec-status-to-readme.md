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
