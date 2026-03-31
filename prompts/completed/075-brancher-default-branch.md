---
status: completed
spec: [010-pr-workflow]
summary: Added DefaultBranch, Pull, and renamed MergeOriginMaster to MergeOriginDefault for dynamic default branch detection
container: dark-factory-075-brancher-default-branch
dark-factory-version: dev
created: "2026-03-05T19:23:06Z"
queued: "2026-03-05T19:23:06Z"
started: "2026-03-05T19:23:07Z"
completed: "2026-03-05T19:29:52Z"
---

# Add DefaultBranch, Pull, and rename MergeOriginMaster

## Goal

Remove hardcoded `master` references from the brancher. Add dynamic default branch detection via `gh` CLI. This is a pure refactor — no behavior change for existing workflows.

## Changes

### 1. Add `DefaultBranch(ctx) (string, error)` to Brancher interface

Detect the repo's default branch name via:
```
gh repo view --json defaultBranchRef --jq '.defaultBranchRef.name'
```

Return trimmed string. Error if empty.

### 2. Add `Pull(ctx) error` to Brancher interface

Simple `git pull` on the current branch.

### 3. Rename `MergeOriginMaster` → `MergeOriginDefault`

Use `DefaultBranch()` internally instead of hardcoded `origin/master`:
```go
func (b *brancher) MergeOriginDefault(ctx context.Context) error {
    defaultBranch, err := b.DefaultBranch(ctx)
    // ...
    cmd := exec.CommandContext(ctx, "git", "merge", "origin/"+defaultBranch)
}
```

Update all callers (currently `processPrompt` in `processor.go`).

### 4. Regenerate mocks

Run `go generate ./...` after updating the Brancher interface.

### 5. Update tests

- Fix all test references from `MergeOriginMaster` to `MergeOriginDefault`
- Add test for `DefaultBranch` (mock or unit)
- Add test for `Pull`

## Constraints

- `make precommit` must pass
- Coverage ≥80% for changed packages
- Follow existing patterns: subprocess via `exec.CommandContext`, `#nosec G204` for validated inputs
- Do NOT commit, tag, or push
