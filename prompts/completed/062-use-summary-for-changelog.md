---
status: completed
spec: [007-git-direct-workflow]
summary: 'Modified changelog generation to use DARK-FACTORY-REPORT summary instead of filename, removed ### Added subsections for cleaner format'
container: dark-factory-062-use-summary-for-changelog
dark-factory-version: v0.14.2
created: "2026-03-03T20:30:34Z"
queued: "2026-03-03T20:30:34Z"
started: "2026-03-03T20:30:34Z"
completed: "2026-03-03T20:37:32Z"
---
<objective>
Use the DARK-FACTORY-REPORT summary as the changelog entry instead of the prompt filename. The current behavior produces useless entries like "- 003-add-text-marshaler-to-all-types" instead of meaningful descriptions.
</objective>

<context>
Read these files:
- pkg/processor/processor.go — the handleDirectWorkflow and handlePostExecution functions
- pkg/git/git.go — CommitAndRelease, updateChangelog, processUnreleasedSection, insertNewVersionSection

Current flow:
1. `preparePromptForExecution()` sets `title` from `pf.Title()` or filename fallback (line 536-539)
2. `handlePostExecution()` extracts `summary` from DARK-FACTORY-REPORT (line 265)
3. `summary` is stored in frontmatter via `pf.SetSummary()` (line 272)
4. `handleDirectWorkflow()` passes `title` (not summary) to `CommitAndRelease()` (line 510)
5. `CommitAndRelease()` uses `title` as the changelog entry (line 113)

The summary is already available but unused for the changelog. Example summary from a real run:
"Added encoding.TextMarshaler/TextUnmarshaler to DateTime, UnixTime, Duration, TimeOfDay with comprehensive JSON and YAML regression tests"

The changelog format is:
```
## v1.24.0

### Added
- <entry goes here>
```

The entry should also NOT use "### Added" unconditionally — it should match the existing changelog style of the project. Many projects use simple `- description` without subsections.
</context>

<requirements>
1. In `handleDirectWorkflow` (processor.go), pass `summary` to `CommitAndRelease` when available, falling back to `title` when summary is empty.

2. The summary comes from `handlePostExecution` which runs before `handleDirectWorkflow`. Make the summary available by:
   - Adding a `summary` field to the workflow state or passing it through the call chain
   - Or reading it back from `pf.Frontmatter.Summary` since it was already saved

3. In `updateChangelog` / `insertNewVersionSection` (git.go), change the format to match standard changelog style:
   - Use `- <entry>` directly under `## vX.Y.Z` (no `### Added` subsection)
   - This matches the common format:
     ```
     ## v1.24.0

     - Description of what changed
     ```

4. Add/update tests:
   - Test that `CommitAndRelease` uses summary text in changelog
   - Test that `processUnreleasedSection` produces `- entry` without `### Added`
   - Test that `insertNewVersionSection` produces `- entry` without `### Added`

5. Also fix `handlePRWorkflow` and `handleWorktreeWorkflow` if they pass title to changelog — they should also prefer summary.
</requirements>

<constraints>
- Do NOT change the DARK-FACTORY-REPORT format
- Do NOT change the prompt file format
- Do NOT change how title is used for git commit messages (commit message can stay as "release vX.Y.Z")
- Do NOT break projects that have no DARK-FACTORY-REPORT (fallback to title must work)
- Keep backward compatibility with existing changelog formats
</constraints>

<verification>
Run: `make test`
All tests must pass including new tests for summary-based changelog entries.
</verification>

<success_criteria>
After this change, a prompt with DARK-FACTORY-REPORT summary "Added TextMarshaler to Date type" produces:
```
## v1.24.0

- Added TextMarshaler to Date type
```
Instead of:
```
## v1.24.0

### Added
- 003-add-text-marshaler-to-all-types
```
</success_criteria>
