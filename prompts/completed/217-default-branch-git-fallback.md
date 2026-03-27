---
status: completed
summary: Added git-native fallback to DefaultBranch() using git symbolic-ref refs/remotes/origin/HEAD so dark-factory works with non-GitHub remotes without requiring defaultBranch in config
container: dark-factory-217-default-branch-git-fallback
dark-factory-version: v0.67.9
created: "2026-03-27T15:21:31Z"
queued: "2026-03-27T15:21:31Z"
started: "2026-03-27T15:35:16Z"
completed: "2026-03-27T15:42:58Z"
---

<summary>
- Default branch discovery works with any git remote, not just GitHub
- When no `defaultBranch` is configured, tries `gh` first, then falls back to `git symbolic-ref`
- Existing behavior unchanged for GitHub repos and configured default branches
- Non-GitHub repos (Bitbucket, local bare repos, GitLab) no longer require explicit `defaultBranch` config
- Test coverage added for the git fallback path
</summary>

<objective>
Add a git-native fallback to `DefaultBranch()` so dark-factory works with any git remote (local bare repos, Bitbucket, GitLab) without requiring `defaultBranch` in config.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Key files to read before making changes:
- `pkg/git/brancher.go` — `DefaultBranch()` method (~line 154), currently only tries `gh repo view`
- `pkg/git/brancher_test.go` — existing `Describe("DefaultBranch", ...)` tests (~line 242), test setup creates local git repos with bare remotes

The current `DefaultBranch()` flow:
1. If `configuredDefaultBranch` is set → return it
2. Else → run `gh repo view --json defaultBranchRef` → fails for non-GitHub repos

The desired flow:
1. If `configuredDefaultBranch` is set → return it
2. Else → try `gh repo view` → if succeeds, return result
3. Else → try `git symbolic-ref refs/remotes/origin/HEAD` → parse branch name from output like `refs/remotes/origin/main`
4. Else → return error
</context>

<requirements>
1. In `pkg/git/brancher.go`, modify the `DefaultBranch()` method:
   - Keep the existing `configuredDefaultBranch` check unchanged
   - Keep the existing `gh repo view` attempt
   - When `gh` fails, instead of returning the error immediately, try a git-native fallback:
     - Run `git symbolic-ref refs/remotes/origin/HEAD`
     - Parse the output: strip the `refs/remotes/origin/` prefix to get the branch name (e.g., `refs/remotes/origin/main` → `main`)
     - If this succeeds and produces a non-empty branch name, log it and return it
     - If this also fails, return the original `gh` error (so error messages stay useful for GitHub users)
   - Add debug logging: `slog.Debug("default branch from git symbolic-ref", "branch", branch)`

2. In `pkg/git/brancher_test.go`, add tests for the git fallback:
   - Test: "falls back to git symbolic-ref when gh is unavailable" — set up a local repo with a bare remote that has a HEAD ref, verify `DefaultBranch()` returns the correct branch without `gh`
     - Create bare repo: `git init --bare`
     - Push master to it: `git remote add origin <bare>` + `git push origin master`
     - The `git push` sets `HEAD` in the bare repo, so `git symbolic-ref refs/remotes/origin/HEAD` works after a fetch
     - Note: you may need to run `git remote set-head origin --auto` after fetch to create the local symbolic ref
   - Test: "returns error when both gh and git symbolic-ref fail" — repo with no remote at all, verify error is returned
</requirements>

<constraints>
- Do NOT change the `Brancher` interface — no signature changes
- Do NOT change `WithDefaultBranch` behavior — configured branch still takes priority
- Do NOT modify any files outside `pkg/git/`
- Do NOT commit — dark-factory handles git
- The `gh` attempt must happen before the git fallback (preserve existing behavior for GitHub users)
- Error from git fallback should not mask the `gh` error — if both fail, return the `gh` error
</constraints>

<verification>
```bash
make precommit
```
</verification>
