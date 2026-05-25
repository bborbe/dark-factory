---
status: committing
summary: Removed unused SEMVER_LINE and MAJOR_LINE variables from scripts/check-changelog.sh
container: dark-factory-exec-417-review-dark-factory-13-fix-shell-script-declarations
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
started: "2026-05-25T15:03:47Z"
completed: "2026-05-25T15:04:02Z"
---

<summary>
- Removed unused SEMVER_LINE and MAJOR_LINE variables from scripts/check-changelog.sh
- These variables were set but never referenced (dead code)
</summary>

<objective>
Fix shellcheck SC2034 warnings in scripts/check-changelog.sh by removing unused variables.
</objective>

<context>
Files to read before making changes:
- `scripts/check-changelog.sh` — lines 20-21, SEMVER_LINE and MAJOR_LINE variables
</context>

<requirements>
1. In `scripts/check-changelog.sh`, remove lines 20-21:
   ```
   SEMVER_LINE='Please choose versions by [Semantic Versioning]'
   MAJOR_LINE='* MAJOR version when you make incompatible API changes,'
   ```

2. If these variables are referenced elsewhere in the script, either use them or remove all references.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
shellcheck scripts/check-changelog.sh
</verification>
