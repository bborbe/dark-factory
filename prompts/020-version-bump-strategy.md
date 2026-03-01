---
status: queued
---
# Add minor version bump support (never major)

## Goal

Dark-factory currently always bumps the patch version (`v0.2.9` → `v0.2.10`).
Add support for minor version bumps (`v0.2.11` → `v0.3.0`) based on the type of
change. **Never bump the major version** — Go module paths would need renaming
to `.../v2` which is extremely disruptive.

## Version Rules

| Change type | Bump | Example |
|-------------|------|---------|
| Bug fix, refactor, docs, tests | Patch | `v0.2.9` → `v0.2.10` |
| New feature, new exported API | Minor | `v0.2.11` → `v0.3.0` |
| Breaking change | **Never** — not supported | — |

## Implementation (post-016 architecture)

### 1. Add `VersionBump` type in `pkg/git/`

```go
type VersionBump int

const (
    PatchBump VersionBump = iota
    MinorBump
)
```

### 2. Add `BumpMinorVersion` in `pkg/git/`

```go
func BumpMinorVersion(tag string) (string, error) {
    // vX.Y.Z → vX.(Y+1).0
}
```

### 3. Update `Releaser` interface

Change `CommitAndRelease` signature to accept bump type:

```go
type Releaser interface {
    GetNextVersion(ctx context.Context, bump VersionBump) (string, error)
    CommitAndRelease(ctx context.Context, title string, bump VersionBump) error
    // ... other methods
}
```

Update `releaser` struct methods to use bump parameter internally.

### 4. Add `determineBump` in runner

This is business logic — belongs in runner, NOT factory:

```go
func determineBump(title string) git.VersionBump {
    lower := strings.ToLower(title)
    for _, kw := range []string{"add", "implement", "new", "support", "feature"} {
        if strings.Contains(lower, kw) {
            return git.MinorBump
        }
    }
    return git.PatchBump
}
```

Call in runner's `processPrompt()`:

```go
bump := determineBump(title)
if err := r.releaser.CommitAndRelease(ctx, title, bump); err != nil { ... }
```

### 5. Regenerate mocks + update tests

- `go generate ./...` to update Releaser mock
- Test `BumpMinorVersion("v0.2.11")` → `"v0.3.0"`
- Test `BumpMinorVersion("v1.0.0")` → `"v1.1.0"`
- Test `determineBump("Add container name tracking")` → `MinorBump`
- Test `determineBump("Fix frontmatter parser")` → `PatchBump`
- Test runner: verify correct bump type passed to mock releaser

## Constraints

- **Never** add `BumpMajorVersion`
- Run `make precommit` for validation only — do NOT commit, tag, or push
- Follow `~/.claude-yolo/docs/go-composition.md` — bump logic in runner, not factory
- Follow `~/.claude-yolo/docs/go-patterns.md` for counterfeiter
