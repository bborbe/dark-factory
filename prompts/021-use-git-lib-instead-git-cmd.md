---
status: queued
---
# Replace git subprocess calls with go-git library

## Goal

Replace all `exec.CommandContext(ctx, "git", ...)` calls in `pkg/git/git.go` with
the `github.com/go-git/go-git/v5` library. This eliminates the dependency on a
`git` binary being present in `$PATH` and makes the git operations testable without
a real git subprocess.

## Current Behavior

`pkg/git/git.go` shells out to git for every operation:
- `git add -A`
- `git commit -m ...`
- `git tag ...`
- `git push`
- `git push origin <tag>`
- `git describe --tags --abbrev=0`

This is fragile: git must be installed, stderr parsing is brittle, and unit tests
require a real git repo.

## Expected Behavior

Use `github.com/go-git/go-git/v5` for all git operations:

```go
import "github.com/go-git/go-git/v5"

repo, err := gogit.PlainOpen(".")
wt, err := repo.Worktree()
wt.Add(".")
wt.Commit("release v0.1.0", &gogit.CommitOptions{...})
repo.CreateTag("v0.1.0", ...)
```

## Benefits

- No `git` binary dependency
- Testable with in-memory or temp-dir repos
- Structured error types instead of stderr parsing
- Works in environments without git installed

## Implementation

1. Add dependency: `go get github.com/go-git/go-git/v5`
2. Rewrite `pkg/git/git.go` functions using go-git API:
   - `gitAddAll` → `wt.Add(".")`
   - `gitCommit` → `wt.Commit(msg, opts)`
   - `gitTag` → `repo.CreateTag(tag, ref, opts)`
   - `gitPush` → `repo.Push(opts)`
   - `getNextVersion` → iterate `repo.Tags()`
3. Keep the same public API (`CommitAndRelease`, `GetNextVersion`, `BumpPatchVersion`)
4. Update tests to use go-git in-memory repo instead of `git init` subprocess

## Reference

- Docs: https://pkg.go.dev/github.com/go-git/go-git/v5
- Examples: https://github.com/go-git/go-git/tree/master/_examples

## Constraints

- Keep public API unchanged
- Run `make precommit` for validation only — do NOT commit, tag, or push (dark-factory handles all git operations)
- Verify tests pass with `make test`
