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

## Implementation

### pkg/git/git.go

1. Add `BumpMinorVersion(tag string) (string, error)` alongside `BumpPatchVersion`:
   ```go
   func BumpMinorVersion(tag string) (string, error) {
       // vX.Y.Z → vX.(Y+1).0
   }
   ```

2. Add a `VersionBump` type:
   ```go
   type VersionBump int
   const (
       PatchBump VersionBump = iota
       MinorBump
   )
   ```

3. Update `CommitAndRelease` to accept a `bump VersionBump` parameter:
   ```go
   func CommitAndRelease(ctx context.Context, changelogEntry string, bump VersionBump) error
   ```

4. Update `GetNextVersion` to also accept `bump VersionBump`.

### pkg/factory/factory.go

Determine bump type from prompt title keywords:
- Keywords suggesting **minor**: `add`, `implement`, `new`, `support`, `feature`
- Everything else: **patch**

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

Call in `processPrompt()`:
```go
bump := determineBump(title)
if err := git.CommitAndRelease(ctx, title, bump); err != nil { ... }
```

### Update tests

- `BumpMinorVersion("v0.2.11")` → `"v0.3.0"`
- `BumpMinorVersion("v1.0.0")` → `"v1.1.0"`
- `determineBump("Add container name tracking")` → `MinorBump`
- `determineBump("Fix frontmatter parser")` → `PatchBump`
- `determineBump("Implement go-git library")` → `MinorBump`

## Constraints

- **Never** add `BumpMajorVersion` — major bumps require Go module path changes
- Update counterfeiter mocks after interface changes (`make generate`)
- Run `make precommit` for validation only — do NOT commit, tag, or push (dark-factory handles all git operations)
