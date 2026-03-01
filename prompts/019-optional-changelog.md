---
status: queued
---
# Make CHANGELOG.md and git release optional

## Goal

Dark-factory currently assumes every project has a `CHANGELOG.md` and always
calls `Releaser.CommitAndRelease()` after executing a prompt. This makes it unusable
for projects without a changelog or without semantic versioning.

Add support for projects that have no `CHANGELOG.md` — in that case, skip all
version-related steps (no changelog update, no tag, no push).

## Current Behavior (post-016)

Runner calls `r.releaser.CommitAndRelease()` which requires `CHANGELOG.md`, creates tag, pushes.
If it fails the prompt is marked as failed.

## Expected Behavior

Runner detects whether the project has a `CHANGELOG.md`:

- **With CHANGELOG.md**: current behavior — update changelog, bump version, tag, push
- **Without CHANGELOG.md**: simple commit only — `git add -A && git commit`, no tag, no push

## Implementation

### 1. Extend `Releaser` interface in `pkg/git/`

Add `CommitOnly` method to the existing `Releaser` interface:

```go
type Releaser interface {
    GetNextVersion(ctx context.Context) (string, error)
    CommitAndRelease(ctx context.Context, title string) error
    CommitOnly(ctx context.Context, message string) error
    HasChangelog(ctx context.Context) bool
}
```

Implement on `releaser` struct:

```go
func (r *releaser) HasChangelog(ctx context.Context) bool {
    _, err := os.Stat("CHANGELOG.md")
    return err == nil
}

func (r *releaser) CommitOnly(ctx context.Context, message string) error {
    if err := gitAddAll(ctx); err != nil {
        return errors.Wrap(ctx, err, "git add")
    }
    return gitCommit(ctx, message)
}
```

### 2. Update runner's `processPrompt()`

In `pkg/runner/runner.go`, branch on changelog presence:

```go
if r.releaser.HasChangelog(ctx) {
    if err := r.releaser.CommitAndRelease(ctx, title); err != nil {
        return errors.Wrap(ctx, err, "commit and release")
    }
} else {
    if err := r.releaser.CommitOnly(ctx, title); err != nil {
        return errors.Wrap(ctx, err, "commit")
    }
}
```

### 3. Update factory — no changes needed

Factory just wires `git.NewReleaser()` — zero logic.

### 4. Regenerate mocks + update tests

- `go generate ./...` to update Releaser mock with new methods
- Test runner with mock: `HasChangelog` returns false → verify `CommitOnly` called, `CommitAndRelease` NOT called
- Test runner with mock: `HasChangelog` returns true → verify `CommitAndRelease` called
- Test `releaser.HasChangelog()` with/without CHANGELOG.md
- Test `releaser.CommitOnly()` commits without tag

## Constraints

- Run `make precommit` for validation only — do NOT commit, tag, or push
- Keep all existing behavior identical for projects WITH CHANGELOG.md
- Follow `~/.claude-yolo/docs/go-composition.md` — all logic in runner, not factory
- Follow `~/.claude-yolo/docs/go-patterns.md` for counterfeiter
