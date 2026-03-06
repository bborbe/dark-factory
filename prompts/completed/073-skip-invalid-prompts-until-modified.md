---
spec: ["015"]
status: completed
summary: Implemented skip-invalid-prompts-until-modified feature to prevent log spam from repeatedly validating failed prompts
container: dark-factory-073-skip-invalid-prompts-until-modified
dark-factory-version: v0.17.2
created: "2026-03-05T18:34:46Z"
queued: "2026-03-05T18:34:46Z"
started: "2026-03-05T18:34:46Z"
completed: "2026-03-05T18:43:42Z"
---

# Skip invalid prompts until file is modified

## Problem

When a prompt in `queue/` fails validation (e.g. missing required fields), the 5-second ticker retries it every cycle, spamming WARN logs endlessly. Example: `023-json-verdict-parser.md` logged "skipping prompt" thousands of times in under 2 minutes.

## Expected behavior

After a prompt fails `ValidateForExecution`, don't retry it until:
1. The file is modified (detected via mod time change), OR
2. A fsnotify event fires for that file

The ticker scan should silently skip previously-failed prompts whose mod time hasn't changed.

## Implementation

In `processor.go`:
1. Add a `skippedPrompts map[string]time.Time` field to `processor` struct (filename → mod time when skipped)
2. Initialize it in `NewProcessor`
3. After `ValidateForExecution` fails, record `skippedPrompts[path] = file.ModTime()`
4. Before calling `ValidateForExecution`, check if `skippedPrompts[path]` exists and mod time matches — if so, skip silently (Debug level, not Warn)
5. On fsnotify trigger (watcher signal), clear the `skippedPrompts` map so all files get re-evaluated
6. Log at Debug level when silently skipping a previously-failed prompt

## Verification

- `make precommit` must pass
- Test: invalid prompt is logged once at Warn, then silently skipped until modified
