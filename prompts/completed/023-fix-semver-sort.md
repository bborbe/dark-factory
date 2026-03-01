---
status: completed
container: dark-factory-023-fix-semver-sort
---


# Fix semver sorting with SemanticVersionNumber type

## Goal

The `getNextVersion` function in `pkg/git/git.go` uses `sort.Strings()` to find the latest tag. This is **lexicographic**, not **semver** sort. It caused a version regression from v0.2.25 → v0.1.3.

## Current Behavior (BUG)

```go
sort.Strings(tagNames)
latestTag := tagNames[len(tagNames)-1]
```

Lexicographic sort is wrong for semver: `v0.10.0 < v0.9.0`, `v0.1.2 > v0.2.25` depending on context.

## Implementation

### 1. Create `SemanticVersionNumber` type in `pkg/git/`

```go
// SemanticVersionNumber represents a parsed semantic version.
type SemanticVersionNumber struct {
    Major int
    Minor int
    Patch int
}

// ParseSemanticVersionNumber parses "vX.Y.Z" into a SemanticVersionNumber.
// Returns error if format is invalid.
func ParseSemanticVersionNumber(tag string) (SemanticVersionNumber, error) { ... }

// String returns the "vX.Y.Z" representation.
func (v SemanticVersionNumber) String() string { ... }

// BumpPatch returns a new version with patch incremented.
func (v SemanticVersionNumber) BumpPatch() SemanticVersionNumber { ... }

// BumpMinor returns a new version with minor incremented and patch reset to 0.
func (v SemanticVersionNumber) BumpMinor() SemanticVersionNumber { ... }

// Less returns true if v is lower than other.
func (v SemanticVersionNumber) Less(other SemanticVersionNumber) bool { ... }
```

### 2. Refactor `getNextVersion` to use it

- Iterate tags, parse each with `ParseSemanticVersionNumber` (skip invalid)
- Find the max version using `Less()`
- Call `BumpPatch()` or `BumpMinor()` on the result

### 3. Remove standalone `BumpPatchVersion` / `BumpMinorVersion` functions

Replace with `SemanticVersionNumber.BumpPatch()` / `BumpMinor()`.

### 4. Tests

- `ParseSemanticVersionNumber("v0.2.25")` → `{0, 2, 25}`
- `ParseSemanticVersionNumber("invalid")` → error
- `ParseSemanticVersionNumber("v1")` → error
- `v0.2.25` is greater than `v0.1.9`
- `v0.10.0` is greater than `v0.9.0`
- `v1.0.0` is greater than `v0.99.99`
- `BumpPatch()`: `v0.2.25` → `v0.2.26`
- `BumpMinor()`: `v0.2.25` → `v0.3.0`
- `String()`: `{0, 2, 25}` → `"v0.2.25"`
- Non-semver tags filtered out
- Empty tag list returns `v0.1.0`

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow patterns in `~/.claude-yolo/docs/go-patterns.md` (interface + struct + New*)
- Follow `~/.claude-yolo/docs/go-testing.md` for tests
- Coverage ≥80% for changed code
