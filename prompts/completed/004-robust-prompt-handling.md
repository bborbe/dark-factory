---
status: failed
---

# Make prompt handling robust for minimal files

## Goal

dark-factory should handle minimal prompt files gracefully — no heading, no frontmatter, just plain text. Currently it fails with "no heading found" when a prompt has no `# heading`.

## Current Behavior (broken)

A file like this:
```
read the Makefile and count the words
```

Causes: `prompt failed: get prompt title: no heading found`

The factory treats missing title as a fatal error and marks the prompt as failed.

## Expected Behavior

- **No `# heading`** → use filename as title (e.g. `004-test.md` → `004-test`)
- **No frontmatter** → already handled (prompt 003 fixed this)
- **Empty file** → skip it, log warning, do NOT mark as failed (user might still be writing)
- **File with only whitespace** → same as empty, skip

## Implementation

### pkg/prompt/prompt.go

1. **`Title()`**: Instead of returning error when no heading found, fall back to filename (without `.md` extension). Never return error for missing heading — it's not an error condition.

2. **`Content()`**: Add a check — if content (after trimming whitespace) is empty, return a specific error (e.g. `ErrEmptyPrompt`). The factory can use this to skip without marking as failed.

### pkg/factory/factory.go

3. **`processPrompt()`**: If `Content()` returns `ErrEmptyPrompt`, log a warning and return nil (not an error). The prompt stays in place — user can keep editing it.

4. **`processExistingQueued()`** and **`handleFileEvent()`**: Same — skip empty prompts without marking as failed.

### pkg/prompt/prompt_test.go

5. Add test cases:
   - `Title()` with no heading → returns filename without extension
   - `Title()` with heading → returns heading (existing behavior)
   - `Content()` on empty file → returns `ErrEmptyPrompt`
   - `Content()` on whitespace-only file → returns `ErrEmptyPrompt`
   - `Content()` on file with content → returns content (existing behavior)

### Validation

After this change, test with `examples/001-test.md`:
```
read the Makefile and count the words
```

Copy to `prompts/` → dark-factory should pick it up with title `001-test`, pass content to executor. It should NOT fail.

## Constraints

- Don't change the Prompt struct
- `ErrEmptyPrompt` should be a sentinel error (`var ErrEmptyPrompt = errors.New(...)`) for clean comparison
- Run `make precommit` before finishing
