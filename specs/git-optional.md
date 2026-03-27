---
status: idea
---

## Summary

- Dark-factory works on projects without git by skipping all git operations
- A `git: false` config flag disables git entirely
- Prompt lifecycle still works: approved → executing → completed
- No commit, tag, push, branch, or PR operations

## Problem

Dark-factory currently requires git at every stage — startup (`ResolveGitRoot`), pre-execution (`Fetch`, `MergeOriginDefault`), and post-execution (commit, tag, push). Projects without git (e.g., config repos, documentation, infrastructure-as-code without version control) cannot use dark-factory at all.

## Goal

Allow dark-factory to run prompts against non-git directories. Claude edits files, prompt moves to completed — no version control operations.

## Scope

### Must skip when `git: false`

- `git.ResolveGitRoot()` at startup
- `brancher.Fetch()` and `brancher.MergeOriginDefault()` pre-execution
- All post-execution git: commit, tag, push, branch creation
- PR creation and merge
- Clone/worktree setup (implies `worktree: false`, `pr: false`)

### Must still work

- Prompt discovery and status transitions
- Container execution (claude edits files in workspace)
- Log capture
- Prompt move to `completed/`
- Verification gate (if enabled)

## Open questions

- Should `git: false` be inferred when not in a git repo, or always explicit?
- Should prompt files still move to `completed/` without a commit, or stay in place with only status change?
- Is there demand beyond the idea stage?
