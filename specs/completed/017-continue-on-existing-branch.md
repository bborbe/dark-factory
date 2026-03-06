---
status: completed
---

# Continue on Existing Branch/PR

## Problem

Every prompt in `workflow: pr` creates a new branch and a new PR. There is no way to run a follow-up prompt on the same branch — for iterative feature work or automated review fixes. This forces humans to manage branches manually between related prompts.

## Goal

A prompt can declare an existing branch (and optionally an existing PR URL) in its frontmatter. The processor runs the prompt on that branch instead of creating a new one. The PR updates automatically via push.

## Non-goals

- No automatic creation of the declared branch if it doesn't exist
- No merge conflict resolution
- No change to `workflow: direct` behavior

## Desired Behavior

1. If a prompt has a `branch` field set, the processor checks out that branch instead of creating a new one.
2. After YOLO executes, changes are committed and pushed to the existing remote branch.
3. If the declared branch does not exist, the prompt fails with a clear error.
4. If `pr-url` is already set in frontmatter, it is preserved — not overwritten.
5. A prompt with no `branch` field behaves identically to today.

## Constraints

- `branch` field only affects `workflow: pr` and `workflow: worktree`
- Branch must already exist at origin before the prompt runs
- Existing tests must still pass

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Declared branch does not exist | Prompt marked `failed`, clear error logged | Fix `branch` field or create branch manually |
| Push rejected (non-fast-forward) | Prompt marked `failed` | Rebase or force-push manually |

## Acceptance Criteria

- [ ] Prompt with `branch` set runs on that branch, not a new one
- [ ] Push goes to existing remote branch; PR auto-updates
- [ ] Missing branch → `failed` status with clear error
- [ ] Prompt without `branch` field is unaffected
- [ ] `pr-url` is not overwritten on subsequent executions

## Verification

```
make precommit
```

## Do-Nothing Option

Humans manage follow-up work manually by checking out the branch themselves and writing a new prompt. Works but breaks unattended operation for multi-pass workflows.
