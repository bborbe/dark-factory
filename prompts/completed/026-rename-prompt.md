---
status: completed
container: dark-factory-026-rename-prompt
---


# Use git mv for prompt file renames and moves

## Goal

Prompt file operations (renumbering, move to completed/) should use `git mv` instead of OS rename + separate git add/delete. This preserves git history and produces cleaner diffs.

## Current Behavior

`pkg/prompt/prompt.go` uses `os.Rename()` for:
- `NormalizeFilenames()` — renumber prompts (e.g., `foo.md` → `001-foo.md`)
- `MoveToCompleted()` — move prompt to `prompts/completed/`

`pkg/git/git.go` uses `wt.Add()` after the move, which shows as delete + add in git history.

## Expected Behavior

Use go-git's worktree move (or `git mv` equivalent) so git tracks the rename:
- `git mv prompts/foo.md prompts/001-foo.md` (renumber)
- `git mv prompts/001-foo.md prompts/completed/001-foo.md` (complete)

This gives `git log --follow` full history through renames.

## Implementation

1. Add a `MoveFile(ctx, oldPath, newPath)` method to the `Releaser` interface that:
   - Opens the repo with go-git
   - Uses `wt.Move(oldRel, newRel)` (go-git's `git mv` equivalent)
   - Falls back to `os.Rename()` + `wt.Add()` + `wt.Remove()` if `wt.Move()` fails

2. Update `pkg/prompt/prompt.go`:
   - `NormalizeFilenames()` — accept a file mover function or interface for renaming
   - `MoveToCompleted()` — use the git-aware move instead of `os.Rename()`

3. Ensure `prompts/completed/` directory is created if it doesn't exist before move

4. Add tests:
   - Git mv preserves tracking (file appears as renamed, not delete+add)
   - Fallback to os.Rename when not in a git repo
   - MoveToCompleted uses git mv
   - NormalizeFilenames uses git mv

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow `~/.claude-yolo/docs/go-patterns.md` (interface + struct + New*)
- Follow `~/.claude-yolo/docs/go-composition.md` (inject dependencies)
- Coverage ≥80% for changed packages
