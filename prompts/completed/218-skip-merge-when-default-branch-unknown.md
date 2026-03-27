---
status: completed
summary: MergeOriginDefault now logs a warning and returns nil when DefaultBranch fails, instead of propagating the error; test updated to expect no error.
container: dark-factory-218-skip-merge-when-default-branch-unknown
dark-factory-version: v0.68.0
created: "2026-03-27T15:52:06Z"
queued: "2026-03-27T15:52:06Z"
started: "2026-03-27T15:52:11Z"
completed: "2026-03-27T16:01:55Z"
---

<summary>
- Prompt execution no longer fails when the default branch cannot be determined
- MergeOriginDefault logs a warning and skips gracefully instead of returning an error
- Local bare repos and non-GitHub remotes work without explicit defaultBranch config
- Merge still runs normally when default branch is known (configured or discovered)
- Existing test updated to expect skip instead of error
</summary>

<objective>
Make MergeOriginDefault skip gracefully when the default branch cannot be determined, instead of failing the entire prompt execution.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Key files to read before making changes:
- `pkg/git/brancher.go` — `MergeOriginDefault()` method (~line 228), currently returns error when `DefaultBranch()` fails
- `pkg/git/brancher_test.go` — `Describe("MergeOriginDefault", ...)` (~line 335), test "returns error when not in a GitHub repository" needs updating

Current behavior: `MergeOriginDefault` calls `DefaultBranch()`, and if that fails (no GitHub remote, no config, no git symbolic-ref), returns an error that aborts the entire prompt.

Desired behavior: If `DefaultBranch()` fails, log a warning and return nil — the prompt runs on the current branch without syncing.
</context>

<requirements>
1. In `pkg/git/brancher.go`, modify `MergeOriginDefault()`:
   - When `DefaultBranch()` returns an error, log a warning with `slog.Warn("skipping merge origin default: could not determine default branch", "error", err)` and return nil
   - Keep the rest of the method unchanged — if DefaultBranch succeeds, merge as before

2. In `pkg/git/brancher_test.go`, update the `MergeOriginDefault` test:
   - Rename "returns error when not in a GitHub repository" to "skips merge when default branch cannot be determined"
   - Change assertion from `Expect(err).To(HaveOccurred())` to `Expect(err).NotTo(HaveOccurred())`
   - Update comment to explain the new behavior
</requirements>

<constraints>
- Do NOT change the `Brancher` interface
- Do NOT modify `DefaultBranch()` — it should still return errors when discovery fails
- Do NOT modify processor code — only `pkg/git/brancher.go` and `pkg/git/brancher_test.go`
- Do NOT commit — dark-factory handles git
- Merge conflicts (when DefaultBranch succeeds but git merge fails) must still return errors
</constraints>

<verification>
```bash
make precommit
```
</verification>
