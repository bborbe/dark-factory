# Fix semver sorting in getNextVersion

## Goal

The `getNextVersion` function in `pkg/git/git.go` uses `sort.Strings()` to find the latest tag. This is **lexicographic**, not **semver** sort. It caused a version regression from v0.2.25 → v0.1.3 because lexicographically `v0.9.x > v0.2.x` and `v0.1.2` appeared on the same commit.

## Current Behavior (BUG)

```go
// Line 241 in pkg/git/git.go
sort.Strings(tagNames)
latestTag := tagNames[len(tagNames)-1]
```

Lexicographic: `v0.1.0 < v0.1.2 < v0.2.25 < v0.9.0`
But `v0.10.0` would sort BEFORE `v0.9.0` (wrong!).
And when `v0.1.2` and `v0.2.25` are both present, the result depends on sort stability.

## Expected Behavior

Parse all tags as semver, sort numerically by major.minor.patch, return the highest.

## Implementation

1. In `getNextVersion()`, replace `sort.Strings(tagNames)` with a proper semver sort:
   - Filter tagNames to only valid semver tags (`vX.Y.Z`)
   - Parse each into (major, minor, patch) integers
   - Sort by major desc, then minor desc, then patch desc
   - Take the first (highest) as `latestTag`

2. Create a helper function `sortSemverTags(tags []string) []string` that:
   - Filters to valid `vX.Y.Z` format
   - Sorts numerically (not lexicographically)
   - Returns sorted slice (highest first)

3. Add tests:
   - `v0.2.25` should be higher than `v0.1.9`
   - `v0.10.0` should be higher than `v0.9.0`
   - `v1.0.0` should be higher than `v0.99.99`
   - Non-semver tags (e.g., `latest`, `beta`) should be filtered out
   - Empty tag list returns `v0.1.0`

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow patterns in `~/.claude-yolo/docs/go-testing.md` for tests
- Coverage ≥80% for changed code
