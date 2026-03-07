---
status: queued
created: "2026-03-07T16:19:54Z"
queued: "2026-03-07T16:19:54Z"
---
<summary>
- Stops dark-factory from writing prompt filenames as changelog entries
- YOLO container now writes descriptive changelog entries under `## Unreleased`
- Each prompt gets a short instruction appended: "update the changelog per your guide"
- Only appended when the project has a CHANGELOG.md
- If YOLO fails to create `## Unreleased`, the prompt fails and retries
- Removes dead code (`insertVersionSection`) that inserted mechanical entries
</summary>

<objective>
Fix bad changelog entries. Currently dark-factory writes the prompt filename as the changelog entry (e.g. "124-fix-specwatcher-skip-non-approved"). Instead, the YOLO container should write descriptive changelog entries, and dark-factory should stop inserting mechanical ones.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read all relevant docs in /home/node/.claude/docs/ before making changes.

The YOLO container already has `/home/node/.claude/docs/changelog-guide.md` with detailed instructions on writing good changelog entries with conventional prefixes.

Currently:
- `pkg/report/suffix.go` — `Suffix()` appends completion report instructions to every prompt
- `pkg/git/git.go` — `updateChangelog()` mechanically inserts the prompt title as a changelog entry
- `pkg/git/git.go` — `HasChangelog()` checks if CHANGELOG.md exists
- `pkg/processor/processor.go` line ~315 — `content = content + report.Suffix()` assembles final prompt content
- `pkg/processor/processor.go` line ~782 — `CommitAndRelease(ctx, bump, title)` passes title for mechanical changelog entry
</context>

<requirements>
1. In `pkg/report/suffix.go`, add a new function `ChangelogSuffix()` that returns a short instruction telling the YOLO agent: "Update CHANGELOG.md following `/home/node/.claude/docs/changelog-guide.md`. Create `## Unreleased` if missing, extend if exists." Two sentences max — the guide teaches the format and conventions.

2. In `pkg/processor/processor.go`, when assembling prompt content (around line 315), check if the project has a CHANGELOG.md (`p.releaser.HasChangelog(ctx)`). If yes, also append `report.ChangelogSuffix()` to the content.

3. In `pkg/git/git.go`, modify `updateChangelog()`: when `processUnreleasedSection()` finds no `## Unreleased` section, return an error instead of calling `insertVersionSection()`. This means YOLO failed to write changelog entries — the prompt should fail and be retried. Remove `insertVersionSection()` entirely (dead code after this change). Remove the `title` parameter from `updateChangelog()` and update all call sites.
</requirements>

<constraints>
- Do NOT change the completion report suffix format (`report.Suffix()`)
- Do NOT change `HasChangelog()` logic
- Do NOT modify test files without updating assertions to match new behavior
- `processUnreleasedSection()` rename logic (## Unreleased → ## vX.Y.Z) must stay unchanged
- `make precommit` must pass
</constraints>

<verification>
```bash
make precommit
```
</verification>
