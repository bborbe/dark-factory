---
tags:
  - dark-factory
  - spec
status: draft
---

## Summary

- Dark-factory detects duplicate prompt/spec numbers automatically during normal operation (daemon watch loop, run command)
- When duplicates are found, auto-resolves by renumbering: earlier `created` date keeps the number, later gets next available
- When a spec is renumbered, all prompts referencing that spec are updated (frontmatter `spec:` field and filename pattern)
- No new CLI command — integrated into existing daemon/run flow
- Transparent self-healing after branch merges

## Problem

When two branches independently create prompts or specs, dark-factory assigns the next available number in each branch. After merging, both branches may have used the same number (e.g., two different files both numbered 220). Dark-factory currently has no mechanism to detect or resolve this. The existing `renameInvalidFiles` only handles duplicates within a single directory scan order, without cross-directory awareness or stable tie-breaking. Since merging happens outside dark-factory, the tool must self-heal on next use.

## Goal

After this work, dark-factory self-heals number collisions during normal operation. The daemon watch loop and `run` command detect duplicates across prompt and spec lifecycle directories, deterministically resolve them using creation timestamps, and propagate spec number changes to all referencing prompts. No manual intervention required.

## Non-goals

- Preventing conflicts at branch creation time (that would require git hooks, out of scope)
- Renumbering to close gaps (e.g., if 5 is missing between 4 and 6, that is fine)
- Handling merge conflicts in file content (only number collisions in filenames)
- Changing how numbers are initially assigned during `approve`
- A separate CLI command — detection and resolution are part of existing daemon/run flow

## Desired Behavior

1. Before processing prompts, the daemon watch loop and `run` command scan all prompt directories (inbox, in-progress, completed, log) and all spec directories (inbox, in-progress, completed, log) for files sharing the same numeric prefix.
2. When two or more files share the same number, the file with the earliest `created` frontmatter date keeps the number. Later files get the next available number. If `created` is missing or tied, alphabetical filename order breaks the tie (earlier name keeps number).
3. When a spec file is renumbered (e.g., `035-foo.md` becomes `043-foo.md`), all prompts that reference the old spec number are updated: the frontmatter `spec:` field value changes from the old number to the new number, and if the prompt filename contains the old spec number in a `spec-NNN` pattern, the filename is updated too.
4. Duplicate detection works across all lifecycle directories for each type. A prompt numbered 220 in `completed/` and another 220 in `in-progress/` are a conflict.
5. Renumbering is logged (old name → new name) so the user can see what was resolved.
6. The check is idempotent. Running it when no duplicates exist produces no changes or renames.

## Constraints

- Existing prompt and spec file formats must not change (same frontmatter fields, same `NNN-slug.md` naming convention)
- The `findNextAvailableNumber` logic must remain consistent: new numbers assigned by reindex use the same gap-filling approach
- Specs use 3-digit zero-padded numbers; prompts use 3-digit zero-padded numbers
- `specnum.Parse()` remains the canonical way to extract spec numbers
- All existing tests must continue to pass
- The `created` frontmatter field format is ISO date (YYYY-MM-DD) or ISO datetime
- Reindex must not modify file content beyond the frontmatter `spec:` field (no body changes)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| File has no `created` field and no frontmatter | Falls back to alphabetical tie-breaking | None needed |
| Filesystem rename fails mid-reindex | Stop immediately, report which files were already renamed | Re-run reindex to fix remaining |
| Prompt references a spec number that doesn't exist | Leave the reference unchanged, log a warning | Manual fix |
| Three-way number collision (3 files with same number) | Oldest keeps number, other two get sequential new numbers | Deterministic ordering ensures idempotency |
| Reindex runs while daemon is processing a prompt | Reindex should only run at startup before processing begins, not concurrently | Sequential execution prevents races |

## Security / Abuse Cases

- Reindex only operates on configured prompt/spec directories, never arbitrary paths
- Filenames are validated against the existing `NNN-slug.md` pattern before rename
- No user input beyond the `--apply` flag; all paths come from config

## Acceptance Criteria

- [ ] Daemon detects duplicate numbers before processing prompts and auto-resolves them
- [ ] `dark-factory run` detects and resolves duplicates before processing
- [ ] When spec 035 is renumbered to 043, prompts with `spec: ["035"]` are updated to `spec: ["043"]`
- [ ] When spec 035 is renumbered, prompt filenames containing `spec-035` are updated to `spec-043`
- [ ] Tie-breaking prefers earlier `created` date, then alphabetical filename
- [ ] Cross-directory duplicates are detected (e.g., same number in `in-progress/` and `completed/`)
- [ ] Renames are logged with old → new name
- [ ] No changes when no duplicates exist (idempotent)
- [ ] All existing tests pass after implementation

## Verification

```
make precommit
```

## Do-Nothing Option

Users must manually detect and fix number collisions after branch merges. This requires knowing which files conflict, choosing which to renumber, finding the next available number, renaming files, and updating all spec cross-references in prompts. Error-prone and tedious, especially for projects with many prompts. The daemon would fail or produce undefined behavior when encountering duplicate numbers.
