---
status: completed
container: dark-factory-024-fix-commit-completed-file
---




# Fix CommitCompletedFile "entry not found" error

## Goal

`CommitCompletedFile()` in `pkg/git/git.go` fails with "entry not found" when trying to `wt.Add(relPath)` for files moved to `prompts/completed/`. This causes 6 test failures (3 in git, 3 in runner) and blocks `make precommit`.

## Current Behavior (BUG)

```go
func CommitCompletedFile(ctx context.Context, path string) error {
    repo, err := gogit.PlainOpen(".")
    wt, err := repo.Worktree()
    wtRoot := wt.Filesystem.Root()
    relPath := strings.TrimPrefix(path, wtRoot+string(os.PathSeparator))
    _, err = wt.Add(relPath)  // FAILS: "entry not found"
```

The go-git `wt.Add()` fails because:
- The file was moved from `prompts/NNN-name.md` to `prompts/completed/NNN-name.md`
- go-git's worktree doesn't track the move; it sees a delete + new untracked file
- `wt.Add()` may not handle this correctly for the new path

## Expected Behavior

`CommitCompletedFile` should:
1. Stage the deletion of the old path (if any)
2. Stage the new file at `prompts/completed/NNN-name.md`
3. Commit both changes

## Implementation

Investigate the go-git API to fix the staging:
- Try `wt.AddWithOptions(&gogit.AddOptions{All: true})` to stage all changes (delete + new)
- Or explicitly stage the old deletion + new file addition
- Or use `wt.AddGlob("prompts/")` to stage all prompt directory changes

Fix the 6 failing tests:
- `pkg/git/`: "stages and commits the file", "does nothing when file is already committed", "stages and commits the modification"
- `pkg/runner/`: "should process existing queued prompt on startup", "should process multiple queued prompts in order", "should process prompt and call git commit and release"

## Constraints

- Run `make precommit` for validation — all tests must pass
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow `~/.claude-yolo/docs/go-testing.md` for test patterns
- Coverage ≥80% for changed packages
