---
status: queued
---
# Make CHANGELOG.md and git release optional

## Goal

Dark-factory currently assumes every project has a `CHANGELOG.md` and always
runs `git.CommitAndRelease()` after executing a prompt. This makes it unusable
for projects without a changelog or without semantic versioning.

Add support for projects that have no `CHANGELOG.md` — in that case, skip all
version-related steps (no changelog update, no tag, no push).

## Current Behavior

`processPrompt()` always calls:
1. `git.GetNextVersion()` — fails if no tags exist
2. `git.CommitAndRelease()` — requires `CHANGELOG.md`, creates tag, pushes

If either fails the prompt is marked as failed.

## Expected Behavior

On startup, detect whether the project has a `CHANGELOG.md`:

- **With CHANGELOG.md**: current behavior — update changelog, bump version, tag, push
- **Without CHANGELOG.md**: simple commit only — `git add -A && git commit -m "<title>"`, no tag, no push, no changelog update

## Implementation

### pkg/git/git.go

Add `HasChangelog(ctx context.Context) bool`:
```go
func HasChangelog(ctx context.Context) bool {
    _, err := os.Stat("CHANGELOG.md")
    return err == nil
}
```

Add `CommitOnly(ctx context.Context, message string) error`:
```go
// CommitOnly stages all changes and commits without tagging or pushing.
func CommitOnly(ctx context.Context, message string) error {
    if err := gitAddAll(ctx); err != nil {
        return errors.Wrap(ctx, err, "git add")
    }
    return gitCommit(ctx, message)
}
```

### pkg/factory/factory.go

In `processPrompt()`, branch on changelog presence:

```go
if git.HasChangelog(ctx) {
    if err := git.CommitAndRelease(ctx, title); err != nil {
        return errors.Wrap(ctx, err, "commit and release")
    }
} else {
    if err := git.CommitOnly(ctx, title); err != nil {
        return errors.Wrap(ctx, err, "commit")
    }
}
```

Remove the `git.GetNextVersion()` call from `processPrompt()` — it is redundant
(called again inside `CommitAndRelease`) and should not run for no-changelog projects.

### Tests

- `HasChangelog()` returns true when `CHANGELOG.md` exists in cwd
- `HasChangelog()` returns false when `CHANGELOG.md` is absent
- `CommitOnly()` commits without creating a tag
- Factory integration: project without `CHANGELOG.md` → commit created, no tag

## Constraints

- `HasChangelog` check is per-run (on startup or per prompt) — not cached
- Run `make precommit` before finishing
