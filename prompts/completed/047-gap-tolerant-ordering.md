---
status: completed
container: dark-factory-047-gap-tolerant-ordering
dark-factory-version: v0.11.1
created: "2026-03-02T12:09:04Z"
queued: "2026-03-02T14:04:32Z"
started: "2026-03-02T14:04:32Z"
completed: "2026-03-02T14:11:21Z"
---

# Count completed directory files as completed regardless of frontmatter

`AllPreviousCompleted` checks if all prompts with numbers less than N exist in `completedDir`. Currently it reads directory entries and extracts numbers from filenames — this already works by file presence, not by status field.

## Problem

Old projects have prompt files in `completed/` that lack frontmatter entirely (no `status` field). Other code paths that read or validate these files may reject them because they have no valid frontmatter. The principle should be: **if a `.md` file is in `completed/`, it counts as completed — no frontmatter required.**

## Required Changes

1. **`AllPreviousCompleted`** in `pkg/prompt/prompt.go` — verify it only checks file presence in `completedDir` by filename number, never reads or validates frontmatter. Currently looks correct, but ensure no caller or helper adds status-field checks.

2. **`NormalizeFilenames`** — when scanning `completedDir`, do not fail or skip files that lack frontmatter. Files without frontmatter in `completed/` should be left as-is.

3. **`ResetExecuting` / `ResetFailed`** — these scan `queueDir` only; confirm they never touch `completedDir` files.

4. **Any file listing/parsing** that reads from `completedDir` (e.g. status display, API endpoints) — must not crash or skip files without frontmatter. Treat missing frontmatter as `status: completed` implicitly.

5. **Tests** — add cases for:
   - File in `completed/` with no frontmatter → counts as completed
   - File in `completed/` with `status: failed` → still counts as completed (it's in the directory)
   - File in `completed/` with valid `status: completed` → counts as completed (existing behavior)
   - Mix of frontmatter and no-frontmatter files in `completed/` → all count

## Verification

Run `make precommit` — must pass.
