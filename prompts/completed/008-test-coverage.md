---
status: completed
---


# Increase test coverage to ≥80% for all packages

## Goal

Every package under `pkg/` must have ≥80% test coverage. Current state:

| Package | Coverage | Target |
|---------|----------|--------|
| executor | 27.6% | ≥80% |
| factory | 67.9% | ≥80% |
| git | 8.5% | ≥80% |
| prompt | 72.5% | ≥80% |

## Approach

Use Ginkgo v2 + Gomega. Use counterfeiter for mocks. Follow patterns in `/home/node/.claude/docs/go-testing.md`.

Check coverage with: `go test -cover ./pkg/...`

### pkg/prompt (72.5% → ≥80%)

Add tests for:
- `splitFrontmatter()` edge cases: no frontmatter, inline `---` in content, `---` at EOF without newline, empty file
- `ResetExecuting()`: directory with mix of statuses
- `SetStatus()`: file without frontmatter gets frontmatter added
- `Content()`: empty file returns `ErrEmptyPrompt`
- `Title()`: file without heading falls back to filename
- `MoveToCompleted()`: creates `completed/` dir, sets status before moving

### pkg/git (8.5% → ≥80%)

Use temp directories with `git init` (same pattern as factory tests):
- `GetNextVersion()`: no tags → v0.1.0, existing tag → bump patch
- `BumpPatchVersion()`: v0.1.0 → v0.1.1, v1.2.3 → v1.2.4
- `CommitAndRelease()`: creates commit, tag, updates CHANGELOG
- Error paths: not a git repo, no remote

### pkg/executor (27.6% → ≥80%)

The `DockerExecutor` calls Docker directly — don't try to unit test that. Instead:
- Test temp file creation and cleanup logic
- Test log file directory creation
- Extract testable helper functions if needed
- Consider testing with a mock command runner if the interface allows

### pkg/factory (67.9% → ≥80%)

Add tests for:
- `processExistingQueued()`: processes multiple prompts in order
- `handleFileEvent()`: skips files with executing/completed/failed status
- `handleFileEvent()`: picks up files without frontmatter
- `handleFileEvent()`: picks up files with `status: queued`
- Error paths: executor returns error → prompt marked as failed

## Constraints

- Every suite file needs `//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate`
- Run `make generate` after any interface changes
- Run `make precommit` before finishing
- Verify coverage with `go test -cover ./pkg/...` — all must be ≥80%
