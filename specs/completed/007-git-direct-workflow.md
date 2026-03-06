---
status: completed
---

# Git Direct Workflow: Commit, Tag, Push

## Problem

After the Docker container makes changes, those changes need to be committed and pushed. For projects with a CHANGELOG.md, this includes version bumping, tagging, and changelog updates. Without automation, this is manual and error-prone.

## Goal

After successful prompt execution, automatically commit all changes, and if a CHANGELOG.md exists, bump the version, update the changelog, create a git tag, and push everything.

## Non-goals

- No major version bumps (manual only)
- No custom commit message format (uses prompt title)
- No interactive rebase or squash
- No push to specific remote (uses default)

## Desired Behavior

1. After container exits successfully: `git add -A`
2. **If CHANGELOG.md exists:**
   - Determine bump type from prompt title:
     - Keywords "add", "implement", "new", "support", "feature" -> minor bump
     - Everything else -> patch bump
   - Find latest version from git tags (semver format `vX.Y.Z`)
   - Calculate next version
   - Update CHANGELOG.md: replace `## Unreleased` with `## vX.Y.Z`, add entry
   - `git commit -m "release vX.Y.Z"`
   - `git tag vX.Y.Z`
   - `git push`
   - `git push origin vX.Y.Z`
3. **If no CHANGELOG.md:**
   - `git commit -m "<prompt title>"`
   - No tag, no push (local commit only)
4. Git operations use non-cancellable context to prevent mid-commit interruption

## Constraints

- Version format: `vX.Y.Z` (semantic versioning)
- CHANGELOG.md format follows Keep a Changelog conventions
- Git tags are annotated (not lightweight)
- All git operations happen on the host (never inside Docker container)
- Push uses default remote (`origin`)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| No changes to commit | Commit is skipped, prompt still moves to completed | None needed |
| Push fails (network) | Log error; commit and tag exist locally | Manual `git push` |
| Tag already exists | Error; prompt stays as executing | Manual version fix |
| CHANGELOG.md has no Unreleased section | Create version section at top | None needed |

## Acceptance Criteria

- [ ] Changes committed after successful execution
- [ ] CHANGELOG.md updated with version and prompt title
- [ ] Git tag created matching changelog version
- [ ] Both commits and tags pushed to remote
- [ ] Without CHANGELOG.md, simple commit without tag/push
- [ ] Version bump is patch by default, minor for "add/new/feature" keywords
- [ ] Shutdown signal doesn't interrupt mid-commit

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Manual git operations after each prompt. Defeats the purpose of unattended execution.
