---
spec: ["016"]
status: completed
summary: Wired auto-merge and auto-release into processor and factory
container: dark-factory-077-wire-auto-merge-into-processor
dark-factory-version: dev
created: "2026-03-05T19:39:30Z"
queued: "2026-03-05T19:39:30Z"
started: "2026-03-05T19:39:30Z"
completed: "2026-03-05T19:53:31Z"
---

# Wire auto-merge and auto-release into processor

## Prerequisites

Prompts 075 (DefaultBranch/Pull/MergeOriginDefault) and 076 (config + PRMerger) must be completed first.

## Goal

Connect PRMerger, autoMerge, and autoRelease into the processor and factory. After this, `autoMerge: true` in config will make dark-factory wait for PR merge and optionally release.

## Changes

### 1. Processor struct

Add fields:
```go
autoMerge   bool
autoRelease bool
prMerger    git.PRMerger
```

Add parameters to `NewProcessor` (after `prCreator`).

### 2. handlePRWorkflow changes

After `slog.Info("created PR", ...)` and saving PR URL to frontmatter:

If `autoMerge` enabled:
1. Call `p.prMerger.WaitAndMerge(gitCtx, prURL)`
2. On error → switch back to original branch, return error
3. On success → `p.brancher.DefaultBranch()`, `p.brancher.Switch(defaultBranch)`, `p.brancher.Pull()`
4. If `autoRelease` and `HasChangelog()` → call `p.handleDirectWorkflow(gitCtx, ctx, title)`
5. Return (already on default branch)

If `autoMerge` disabled → existing behavior (switch back to original branch).

### 3. handleWorktreeWorkflow changes

Same pattern as PR workflow. After creating PR:
1. If `autoMerge` → WaitAndMerge, chdir back to originalDir, remove worktree, switch to default branch, pull, optionally release
2. If not → existing behavior (chdir back, remove worktree)

### 4. Factory wiring

In `pkg/factory/factory.go`, pass `git.NewPRMerger()`, `cfg.AutoMerge`, `cfg.AutoRelease` to `NewProcessor`.

### 5. Docs

Update `README.md` config table with `autoMerge` and `autoRelease` fields.
Update `example/.dark-factory.yaml` if it exists.

### 6. Tests

Processor tests (`pkg/processor/processor_test.go`):
- `autoMerge: false` → `prMerger.WaitAndMerge` never called
- `autoMerge: true`, merge succeeds → WaitAndMerge called, DefaultBranch called, Switch + Pull called
- `autoMerge: true`, merge fails → Switch(originalBranch) called, error returned
- `autoMerge: true, autoRelease: true`, changelog exists → CommitAndRelease called
- `autoMerge: true, autoRelease: true`, no changelog → CommitAndRelease NOT called
- `autoMerge: true, autoRelease: false` → CommitAndRelease never called

## Constraints

- `make precommit` must pass
- Coverage ≥80% for changed packages
- Do NOT commit, tag, or push
