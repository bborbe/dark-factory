---
status: created
---

# Add autoMerge/autoRelease config and PRMerger interface

## Goal

Add config fields and the PRMerger interface. No wiring into the processor yet — that's the next prompt.

## Changes

### 1. Config fields

Add to `pkg/config/config.go` Config struct:
```go
AutoMerge   bool `yaml:"autoMerge"`
AutoRelease bool `yaml:"autoRelease"`
```

Both default `false` in `Defaults()`.

Validation rules:
- `autoMerge: true` requires `workflow: pr` or `workflow: worktree` → error: `"autoMerge requires workflow 'pr' or 'worktree'"`
- `autoRelease: true` requires `autoMerge: true` → error: `"autoRelease requires autoMerge"`

### 2. PRMerger interface

Add `pkg/git/pr_merger.go`:

```go
// PRMerger watches a PR until mergeable and merges it.
//
//counterfeiter:generate -o ../../mocks/pr_merger.go --fake-name PRMerger . PRMerger
type PRMerger interface {
    WaitAndMerge(ctx context.Context, prURL string) error
}
```

Implementation:
- Poll `gh pr view <prURL> --json mergeStateStatus` every 30 seconds
- `MERGEABLE` → merge with `gh pr merge <prURL> --merge --delete-branch`
- `CONFLICTING` → return error immediately (no waiting)
- Timeout 30 minutes → return error
- Does NOT touch local git state (no checkout, no pull)

### 3. Generate mocks

Run `go generate ./...` after adding the interface.

### 4. Tests

Config tests (`pkg/config/config_test.go`):
- `Defaults()` has `AutoMerge: false, AutoRelease: false`
- YAML parsing of both fields
- Validation: `autoMerge: true` + `workflow: direct` → error
- Validation: `autoRelease: true` + `autoMerge: false` → error
- Validation: `autoMerge: true` + `workflow: pr` → no error

PRMerger tests (`pkg/git/pr_merger_test.go`):
- Returns error on cancelled context
- Returns error on `CONFLICTING` state

## Constraints

- `make precommit` must pass
- Coverage ≥80% for changed packages
- Do NOT commit, tag, or push
