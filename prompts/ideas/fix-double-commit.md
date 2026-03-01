---
status: completed
container: dark-factory-001-fix-double-commit
---



# Fix: completed/ prompt file missing from release commit

## Root Cause

There are two commits per prompt execution:

1. **YOLO's internal commit** — made from inside the container by `git.CommitAndRelease()` called by YOLO itself (as part of the task). This commit includes code changes but NOT the completed/ file — because `MoveToCompleted()` hasn't run yet.

2. **Dark-factory's release commit** — called after `Execute()` returns. By this point YOLO already committed, so `git add -A` in dark-factory's `CommitAndRelease()` finds nothing new to stage (the code changes are already committed).

Result: `MoveToCompleted()` runs after YOLO's commit and before/after dark-factory's commit, but either way the moved file ends up in an untracked state.

## Fix

After `Execute()` returns and `MoveToCompleted()` runs, dark-factory must do a separate **follow-up commit** specifically for the completed file — even if the main release commit was already made by YOLO.

### Option A: Always commit the completed file separately

In `processPrompt()`, after `MoveToCompleted()`:

```go
// Always stage and commit the completed file, even if YOLO already committed
if err := git.CommitCompletedFile(ctx, completedPath); err != nil {
    return errors.Wrap(ctx, err, "commit completed file")
}
```

Where `CommitCompletedFile` does:
```go
func CommitCompletedFile(ctx context.Context, path string) error {
    cmd := exec.CommandContext(ctx, "git", "add", path)
    if err := cmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "git add completed file")
    }
    // Check if there's anything to commit
    statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
    out, _ := statusCmd.Output()
    if len(strings.TrimSpace(string(out))) == 0 {
        return nil // nothing to commit
    }
    return gitCommit(ctx, "move prompt to completed")
}
```

### Option B: Remove dark-factory's `CommitAndRelease` call entirely

Let YOLO handle all commits. Dark-factory only calls `MoveToCompleted()` and then does a simple `git add <completedPath> && git commit "move to completed"`.

This is cleaner but requires YOLO to always commit (it already does).

## Recommended: Option A

It is minimal, safe (no-op if nothing to stage), and doesn't change the overall release flow.

## Constraints

- The follow-up commit must NOT create a new tag — just a plain commit
- Run `make precommit` for validation only — do NOT commit, tag, or push (dark-factory handles all git operations)
- Add a test: after `processPrompt`, the completed file exists in git history
