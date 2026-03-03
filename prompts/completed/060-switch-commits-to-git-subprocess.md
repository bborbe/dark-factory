---
status: completed
summary: Switched git commit operations from go-git to subprocess calls for proper git config support
container: dark-factory-060-switch-commits-to-git-subprocess
dark-factory-version: v0.14.1
created: "2026-03-03T18:54:30Z"
queued: "2026-03-03T18:54:30Z"
started: "2026-03-03T19:59:35Z"
completed: "2026-03-03T20:04:29Z"
---
# Switch git commit from go-git to subprocess

## Goal

Replace the go-git `wt.Commit()` call with `exec.CommandContext(ctx, "git", "commit", ...)` so that git signing, hooks, and other git config settings are respected automatically.

## Current Behavior

`gitCommit` in `pkg/git/git.go` uses go-git's `wt.Commit()` which bypasses git config (e.g. `commit.gpgsign`). Commits are never signed even when the repo requires it.

## Expected Behavior

`gitCommit` uses `exec.CommandContext` to run `git commit -m "message"`, same pattern as `gitPush` and `gitPushTag` already use. Git handles signing, hooks, and config automatically.

## Implementation

### 1. Replace `gitCommit` in `pkg/git/git.go`

Replace the current go-git implementation with a subprocess call:

```go
func gitCommit(ctx context.Context, message string) error {
    slog.Debug("creating commit", "message", message)

    cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
    if err := cmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "create commit")
    }
    return nil
}
```

### 2. Replace `gitAddAll` in `pkg/git/git.go`

Also switch `gitAddAll` to subprocess for consistency:

```go
func gitAddAll(ctx context.Context) error {
    cmd := exec.CommandContext(ctx, "git", "add", "-A")
    if err := cmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "git add all")
    }
    return nil
}
```

### 3. Replace `gitTag` in `pkg/git/git.go`

Also switch to subprocess for consistency (enables signed tags if configured):

```go
func gitTag(ctx context.Context, tag string) error {
    if _, err := ParseSemanticVersionNumber(ctx, tag); err != nil {
        return errors.Wrap(ctx, err, "invalid tag format")
    }

    slog.Debug("creating tag", "tag", tag)

    cmd := exec.CommandContext(ctx, "git", "tag", tag)
    if err := cmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "create tag")
    }
    return nil
}
```

### 4. Clean up unused go-git imports

After switching `gitCommit`, `gitAddAll`, and `gitTag` to subprocess, remove any go-git imports that are no longer used in these functions. The go-git imports may still be needed by other functions like `CommitCompletedFile`, `getNextVersion`, and `MoveFile` — only remove truly unused imports.

### 5. Update `CommitCompletedFile` to use subprocess

`CommitCompletedFile` also uses go-git for staging and committing. Replace with subprocess:

```go
func CommitCompletedFile(ctx context.Context, path string) error {
    // Stage all changes (handles file moves properly)
    cmd := exec.CommandContext(ctx, "git", "add", "-A")
    if err := cmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "git add")
    }

    // Check if there's anything to commit
    statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
    output, err := statusCmd.Output()
    if err != nil {
        return errors.Wrap(ctx, err, "git status")
    }

    // Nothing to commit
    if len(strings.TrimSpace(string(output))) == 0 {
        return nil
    }

    // Commit
    commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", "move prompt to completed")
    if err := commitCmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "git commit")
    }
    return nil
}
```

### 6. Tests

Update `pkg/git/git_test.go` — tests should still pass since the behavior is identical, just the implementation changed. If tests mock go-git internals, update them to work with subprocess calls instead.

Ensure `getNextVersion` and `MoveFile` still work — they use go-git for reading (tags, worktree status) which is fine to keep.

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for changed packages
- Keep go-git for read-only operations (`getNextVersion`, `MoveFile`) — only switch write operations to subprocess
- The `object` and `plumbing` imports may still be needed by `getNextVersion` — don't remove prematurely
