---
spec: ["016"]
status: completed
summary: Fixed changelog entry creation when no Unreleased section exists by passing title through CommitAndRelease
container: dark-factory-079-fix-changelog-entry-before-tag
dark-factory-version: v0.17.9
created: "2026-03-05T21:39:56Z"
queued: "2026-03-05T21:39:56Z"
started: "2026-03-05T21:39:56Z"
completed: "2026-03-05T21:53:01Z"
---

<objective>
Fix: when CHANGELOG.md exists but has no `## Unreleased` section, `CommitAndRelease` still creates a tag without any changelog entry. The tag should always have a corresponding changelog entry.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/git/git.go` and `pkg/processor/processor.go` before making changes.

The bug: `updateChangelog` in `pkg/git/git.go` silently does nothing when no `## Unreleased` section exists (line 331). But `CommitAndRelease` proceeds to commit, tag, and push — resulting in a tagged version with no changelog entry.

The fix should ensure that when no `## Unreleased` section exists, a new version section is created with the prompt title as the changelog entry, inserted before the first existing `## v` section.

Current flow in `handleDirectWorkflow` (processor.go ~line 677):
1. `determineBump()` — analyzes CHANGELOG content
2. `GetNextVersion()` — calculates next version
3. `CommitAndRelease()` — commits, tags, pushes (but title is NOT passed)

The `title` variable is available in `handleDirectWorkflow` but never passed to `CommitAndRelease`.
</context>

<requirements>
1. Add a `title string` parameter to `CommitAndRelease(ctx, bump)` → `CommitAndRelease(ctx, bump, title)`
2. Pass `title` through to `updateChangelog(ctx, version)` → `updateChangelog(ctx, version, title)`
3. In `updateChangelog`: when no `## Unreleased` section is found, create a new version section:
   - Find the first line matching `^## v[0-9]`
   - Insert `## vX.Y.Z\n\n- <title>\n` immediately before it
   - If no existing version sections, append after all content
4. Update the `Releaser` interface to match the new signature
5. Update the counterfeiter mock by running `go generate ./...`
6. Update all callers of `CommitAndRelease` to pass the title
7. Update existing tests and add a new test case: "creates changelog entry when no Unreleased section exists"
</requirements>

<constraints>
- Do NOT change behavior when `## Unreleased` section exists — existing rename logic must stay the same
- The title parameter must be the prompt title (e.g. "install uv and updater tool")
- Changelog entry format: `- <title>` (single bullet point with the title)
- Keep the version section format consistent with existing entries (## vX.Y.Z followed by bullet points)
- Use Ginkgo v2 for tests
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
