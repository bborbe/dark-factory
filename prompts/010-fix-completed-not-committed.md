---
status: executing
---

# Fix: completed/ files not staged before git commit

## Bug

After `MoveToCompleted()` moves a prompt file from `prompts/` to `prompts/completed/`,
the `git add -A` in `CommitAndRelease()` does NOT stage the new file in `completed/`.

Result: the completed prompt appears as untracked in git and is missing from the release commit.

## Root Cause

`git add -A` stages all changes relative to the working tree. The `prompts/completed/` directory
is listed in `.gitignore` OR git considers the rename as a delete+create but the create isn't
being picked up. Investigate: check if `prompts/completed/` is gitignored.

If not gitignored: the issue is timing — `MoveToCompleted()` runs AFTER `CommitAndRelease()` in
`processPrompt()` / `processExistingQueued()`. The file is moved AFTER the commit, so it was
never staged.

Check `pkg/factory/factory.go`:

```
processPrompt() → executor.Execute() → CommitAndRelease()  ← commit happens here
                                                             ← then MoveToCompleted() runs
```

The fix: move `MoveToCompleted()` BEFORE `CommitAndRelease()`, or include the completed file
in the commit explicitly.

## Expected Behavior

The `prompts/completed/007-container-name-tracking.md` file should appear in the release commit
alongside CHANGELOG.md, not as a separate untracked file afterward.

## Implementation

1. Investigate whether `prompts/completed/` is in `.gitignore`
2. Find where `MoveToCompleted()` is called relative to `CommitAndRelease()`
3. Fix ordering so the completed file is staged before the commit
4. Add a test that verifies the completed file is present in the git repo after processing

## Also: prompts/completed/ should be committed

Similar to logs, check if `prompts/completed/` needs a `.gitignore` entry or should be committed.
Decision: completed prompts ARE useful history — keep them in git. Logs are NOT — already gitignored via `/prompts/log/`.

## Constraints

- Run `make precommit` before finishing
