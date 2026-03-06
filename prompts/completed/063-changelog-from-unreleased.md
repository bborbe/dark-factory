---
spec: ["007"]
status: completed
summary: 'Simplified changelog handling to only rename ## Unreleased section, preserving all content'
container: dark-factory-063-changelog-from-unreleased
dark-factory-version: v0.14.2
created: "2026-03-03T20:43:09Z"
queued: "2026-03-03T20:43:09Z"
started: "2026-03-03T20:43:09Z"
completed: "2026-03-03T20:57:14Z"
---
<objective>
Change dark-factory to stop generating changelog entries. Instead, only rename an existing `## Unreleased` section to `## vX.Y.Z` at release time. The YOLO container (Claude) writes the actual changelog content — dark-factory should never invent changelog text.
</objective>

<context>
Read these files:
- pkg/git/git.go — CommitAndRelease, updateChangelog, processUnreleasedSection, insertNewVersionSection
- pkg/processor/processor.go — handleDirectWorkflow, handlePostExecution

Current broken flow:
1. YOLO container makes code changes (may or may not update CHANGELOG.md)
2. Dark-factory calls `CommitAndRelease(title)` where title = prompt filename
3. `updateChangelog()` inserts `### Added\n- <filename>` — useless entry like "- 003-add-text-marshaler-to-all-types"
4. If no `## Unreleased` exists, `insertNewVersionSection()` creates a whole new section with the filename

Correct flow (after this change):
1. YOLO container makes code changes AND adds meaningful entries under `## Unreleased` in CHANGELOG.md
2. Dark-factory calls `CommitAndRelease()`
3. If `## Unreleased` exists: rename it to `## vX.Y.Z` (keep existing entries as-is)
4. If `## Unreleased` does NOT exist: skip changelog entirely (don't invent entries)

The YOLO container is already instructed to write `## Unreleased` entries (see `/home/node/.claude/docs/git-workflow.md` line 88-96). Dark-factory just needs to stop overriding them.
</context>

<requirements>
1. **Simplify `CommitAndRelease`**: Remove the `changelogEntry string` parameter. The function no longer needs external text — it only renames `## Unreleased`.

2. **Simplify `updateChangelog`**:
   - If `## Unreleased` section exists: rename it to `## vX.Y.Z`, keep all existing entries below it unchanged
   - If `## Unreleased` does NOT exist: do nothing (return without error)
   - Remove all logic that inserts `### Added` or `- entry` lines

3. **Remove `insertNewVersionSection`**: No longer needed. Dark-factory never creates changelog sections from scratch.

4. **Update `processUnreleasedSection`**: Only rename `## Unreleased` → `## vX.Y.Z`. Do NOT insert any entry lines. Keep everything between `## Unreleased` and the next `## v` section exactly as-is.

5. **Update all callers** of `CommitAndRelease` in processor.go to remove the title/entry argument.

6. **Update tests**:
   - Test: `## Unreleased` with entries → renamed to `## vX.Y.Z`, entries preserved
   - Test: `## Unreleased` empty (no entries) → renamed to `## vX.Y.Z` with empty section
   - Test: No `## Unreleased` → changelog unchanged
   - Test: Multiple entries under Unreleased → all preserved after rename
   - Remove tests for `insertNewVersionSection` (function deleted)
   - Remove tests for entry insertion behavior

7. **Update `determineBump`**: This function may use the changelog entry to determine patch vs minor. After this change, it should analyze the `## Unreleased` content directly from CHANGELOG.md instead of relying on the title parameter.
</requirements>

<constraints>
- Do NOT change CLAUDE.md or git-workflow.md (they already instruct YOLO correctly)
- Do NOT change the DARK-FACTORY-REPORT format
- Do NOT change the prompt file format
- Do NOT change how `CommitOnly` works (non-changelog workflows unchanged)
- Commit message format stays as "release vX.Y.Z"
- Keep the HasChangelog check — if no CHANGELOG.md exists, skip all changelog logic
</constraints>

<verification>
Run: `make test`
All tests must pass.
Run: `make precommit`
Must pass with exit code 0.
</verification>

<success_criteria>
Given a CHANGELOG.md with:
```markdown
## Unreleased

- Add TextMarshaler/TextUnmarshaler to DateTime, UnixTime, Duration, TimeOfDay
- Add comprehensive JSON and YAML struct regression tests
```

After dark-factory release, it becomes:
```markdown
## v1.25.0

- Add TextMarshaler/TextUnmarshaler to DateTime, UnixTime, Duration, TimeOfDay
- Add comprehensive JSON and YAML struct regression tests
```

No "### Added", no filename entries, no invented text. Just rename the header.
</success_criteria>
