---
status: queued
---

# Fix CommitCompletedFile to only stage the completed file

## Problem

`CommitCompletedFile` in `pkg/git/git.go` runs `git add -A` which stages ALL changes in the working directory, not just the completed prompt file. This means all code changes made by YOLO get committed with the message "move prompt to completed" instead of being committed later by `handleDirectWorkflow` with proper versioning.

## Current code (broken)

```go
func CommitCompletedFile(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "git", "add", "-A")
	// ...
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", "move prompt to completed")
```

## Expected behavior

`CommitCompletedFile` should ONLY stage and commit the specific completed prompt file (the `path` argument). All other changes (YOLO code, CHANGELOG, etc.) should remain unstaged for `handleDirectWorkflow` / `handlePRWorkflow` to handle.

## Fix

In `pkg/git/git.go`, change `CommitCompletedFile` to:
1. Replace `git add -A` with `git add <path>` (only stage the completed file)
2. Keep the rest of the logic (status check, commit) the same

## Verification

- `make precommit` must pass
- Test that only the completed file path is staged, not other working directory changes
