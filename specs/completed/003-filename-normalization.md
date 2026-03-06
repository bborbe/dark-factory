---
status: completed
---

# Filename Normalization to NNN-slug.md

## Problem

Users create prompt files with arbitrary names (`add-health-check.md`, `9-fix-bug.md`, `my task.md`). Processing order depends on alphabetical sort, but inconsistent naming makes ordering unpredictable and hard to track.

## Goal

All prompt files in the queue follow a consistent `NNN-slug.md` convention (e.g., `001-add-health-check.md`). Files are automatically renamed on detection. Sequence numbers never conflict with completed prompts.

## Non-goals

- No renaming of files in inbox (inbox is passive)
- No renaming of completed files (already archived)
- No gaps in numbering (sequential from 001)
- No user-configurable naming format

## Desired Behavior

1. When the watcher detects a file change in queueDir, normalization runs
2. Files already matching `NNN-slug.md` (3-digit prefix) keep their number
3. Files with wrong prefix format (`9-slug.md`, `99-slug.md`) get reformatted to 3 digits
4. Files with no prefix (`slug.md`) get the next available number
5. Files with duplicate numbers get reassigned to the next available number
6. Numbers already used by completed prompts are never reused
7. Slugs are derived from the filename: lowercase, hyphens, no special characters
8. Renames are logged for visibility

## Constraints

- Only `.md` files are processed
- Numbers are 3 digits, zero-padded (001-999)
- Completed directory is scanned to determine used numbers
- Normalization is idempotent — running twice produces no additional renames

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| File with number 999+ | Unlikely (999 prompts); error if exceeded | Manual renumbering |
| File rename fails (permissions) | Log error, skip file | Fix permissions |
| Race condition (file deleted during rename) | Log error, skip file | Next scan picks up changes |

## Acceptance Criteria

- [ ] `my-task.md` renamed to `NNN-my-task.md` with next available number
- [ ] `9-task.md` renamed to `009-task.md`
- [ ] `001-task.md` keeps its name if 001 is available
- [ ] Completed prompt numbers are not reused
- [ ] Duplicate numbers resolved by reassignment
- [ ] Inbox files are never touched

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Require users to manually name files with correct NNN- prefix. Error-prone and tedious for long sequences.
