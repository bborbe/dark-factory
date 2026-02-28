---
status: completed
---

# Make frontmatter optional for prompt pickup

## Goal

dark-factory should pick up any `.md` file in `prompts/` that doesn't have an explicit skip status. Humans should be able to drop a plain markdown file without frontmatter and dark-factory picks it up automatically, adding the frontmatter itself.

## Current Behavior (broken)

- Files without `---` frontmatter delimiters → ignored (treated as invalid)
- Files without `status` field → ignored
- Only `status: queued` is picked up

## Expected Behavior

- **No frontmatter at all** → pick it up (treat as queued)
- **Frontmatter but no status field** → pick it up (treat as queued)
- **`status: queued`** → pick it up
- **`status: executing`** → skip (in progress)
- **`status: completed`** → skip
- **`status: failed`** → skip

In other words: pick up everything UNLESS it has an explicit skip status (`executing`, `completed`, `failed`).

## Implementation

### pkg/prompt/prompt.go

1. **`ListQueued()`**: Change logic from "must have `status: queued`" to "must NOT have `status: executing/completed/failed`":
   - If `readFrontmatter()` returns error (no frontmatter) → include in results
   - If frontmatter has no status field (empty string) → include in results
   - If `status: queued` → include
   - If `status: executing/completed/failed` → skip

2. **`readFrontmatter()`**: Return a valid empty Frontmatter when file has no frontmatter delimiters (instead of error). Only return error on actual I/O failures.

3. **`SetStatus()`**: When file has no frontmatter, ADD frontmatter with the status field (prepend `---\nstatus: executing\n---\n` before existing content).

4. **`Title()`**: When file has no frontmatter, scan from the beginning of the file for `# heading`. Currently assumes frontmatter exists and skips it — needs to handle both cases.

5. **`Content()`**: No changes needed (returns full file content regardless).

### pkg/factory/factory.go

6. **`handleFileEvent()`**: Same logic change — pick up files without frontmatter or without status, not just `status: queued`.

### pkg/prompt/prompt_test.go

7. Update existing tests and add new test cases:
   - File with no frontmatter at all (just plain markdown) → listed as queued
   - File with frontmatter but no status field → listed as queued
   - File with `status: queued` → listed as queued (existing behavior)
   - File with `status: executing` → not listed
   - File with `status: failed` → not listed
   - `SetStatus()` on file without frontmatter → adds frontmatter
   - `Title()` on file without frontmatter → finds heading from start of file

### Integration test validation

Use `examples/001-test.md` as reference — this is a real-world prompt with no frontmatter:

```
read the Makefile and count the words
```

After this change, if you copy `examples/001-test.md` to `prompts/`, dark-factory should pick it up.

### Bug fix: MoveToCompleted must set status

Currently `MoveToCompleted()` only moves the file. It does NOT set status to `completed`. The factory calls `SetStatus()` then `MoveToCompleted()` separately, but YOLO's manual `mv` bypassed `SetStatus()` — leaving files in `completed/` with wrong status (`queued`, `failed`).

Fix: `MoveToCompleted()` must call `SetStatus(ctx, path, "completed")` before moving. This makes it impossible to have a file in `completed/` with wrong status, regardless of how it got there.

Also fix the existing files in `prompts/completed/` — both currently have wrong status:
- `001-mvp-main-loop-20260228-224953.md` has `status: queued`
- `002-fsnotify-watch-loop.md` has `status: failed`

Both should be `status: completed`.

## Constraints

- Don't break existing tests for files WITH frontmatter
- Keep the Prompt struct unchanged
- Run `make precommit` before finishing
