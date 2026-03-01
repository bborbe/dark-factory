---
status: completed
---


# Add prompt filename validation and auto-renumbering

## Goal

Prevent duplicate or missing numbers in prompt filenames. Enforce the naming
convention `NNN-slug.md` (3-digit zero-padded number + kebab-case slug).

## Problem

Prompts must be named `NNN-slug.md` (e.g. `007-container-name-tracking.md`).
Two failure modes:

1. **No number**: a file like `fix-something.md` gets picked up by the watcher
   but sorted incorrectly (alphabetically after all numbered files).
2. **Duplicate number**: two files `009-foo.md` and `009-bar.md` — only one
   runs, the other is silently skipped or runs out of order.

## Expected Behavior

On startup (before `processExistingQueued`) and on each file event, validate
prompt filenames. For invalid files:

- **No number prefix**: auto-assign the next available number and rename the file.
  e.g. `fix-something.md` → `011-fix-something.md`
- **Duplicate number**: rename the later file to the next available number.
  e.g. `009-bar.md` → `011-bar.md` (if 009 is taken by `009-foo.md`)
- **Wrong format** (e.g. `9-foo.md` instead of `009-foo.md`): normalize to
  zero-padded 3-digit form.

Log a warning whenever a file is renamed.

## Implementation

### pkg/prompt/prompt.go

Add a `NormalizeFilenames(ctx, dir)` function:

1. Scan all `.md` files in `dir` (not `completed/`)
2. Parse the numeric prefix with regex `^(\d+)-(.+)\.md$`
3. Collect used numbers, detect duplicates and missing-number files
4. For each invalid file: determine next free number, rename with `os.Rename`
5. Return list of renames performed (for logging)

### pkg/factory/factory.go

Call `prompt.NormalizeFilenames(ctx, f.promptsDir)` in `Run()` before
`processExistingQueued()` (after `ResetExecuting`).

Also call it in `handleFileEvent()` when a new `.md` file is detected with
an invalid name — rename first, then process.

### Tests

- No-number file gets next available number assigned
- Duplicate number: second file gets renumbered
- `9-foo.md` normalized to `009-foo.md`
- Already-valid files are unchanged
- Rename is logged

## Constraints

- Only rename files in `prompts/` root, not `prompts/completed/` or `prompts/ideas/`
- Renaming must happen before sorting/processing so the renamed file is processed
  in correct order
- Run `make precommit` before finishing
