---
status: draft
---

## Summary

- The 22-parameter processor constructor is reduced to ~10 parameters
- Git workflow logic is extracted into a dedicated WorkflowExecutor
- Changelog bump detection is already extracted by prompt 196 (extract-determine-bump)
- Spec auto-completion logic is extracted or simplified
- No behavioral changes — pure structural refactor

## Problem

`pkg/processor/processor.go` is 1,146 lines with a struct that has 20 fields, 18 methods, and a 22-parameter constructor. It mixes five distinct responsibilities:

1. **Queue scanning** — which prompt to pick next, skip logic, ordering
2. **Git workflow setup** — branch creation, clone setup, directory switching
3. **Release orchestration** — commit, tag, push, PR creation, auto-merge
4. **Changelog analysis** — `determineBump`, `extractUnreleasedSection` (addressed by prompt 196)
5. **Spec auto-completion** — triggering `autoCompleter` after each prompt

The struct holds five separate git abstractions (`Brancher`, `Cloner`, `PRCreator`, `PRMerger`, `Releaser`) because it orchestrates the full git workflow internally.

## Goal

The finished system has:

- A `WorkflowExecutor` interface in `pkg/processor` that owns branch/clone/PR/merge lifecycle
- The processor delegates workflow setup and completion to `WorkflowExecutor`
- The processor struct has ~10-12 fields focused on queue management and prompt lifecycle
- `processor.go` is under 600 lines
- The `NewProcessor` constructor has ~10-12 parameters

## Non-Goals

- Changing any behavior — this is a pure refactor
- Splitting into multiple packages (workflow executor lives in `pkg/processor` or `pkg/workflow`)
- Redesigning the PR workflow or worktree workflow
- Extracting `determineBump` — already handled by prompt 196

## Desired Behavior

1. A `WorkflowExecutor` interface exists with methods like `Setup(ctx, branchName) error` and `Complete(ctx, promptPath, title) error`
2. Concrete implementations exist for direct workflow, PR workflow, and worktree/clone workflow
3. The processor's `processPrompt` method delegates to `WorkflowExecutor` instead of branching on `p.worktree`, `p.pr`, etc.
4. The processor struct no longer directly holds `Brancher`, `Cloner`, `PRCreator`, `PRMerger` — those are fields of the workflow executor implementations
5. The factory creates the appropriate `WorkflowExecutor` based on config and passes it to `NewProcessor`
6. All existing tests pass — test setup may change to construct workflow executors instead of individual git mocks

## Constraints

- No behavioral changes — every test must pass
- The `Processor` interface (`Process`, `ProcessQueue`) does not change
- Prompt 196 (extract-determine-bump) handles changelog extraction — do not duplicate that work
- Keep the dual-context pattern (`gitCtx`, `ctx`) for now — document it but don't change it

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---------|-------------------|----------|
| Workflow executor missing a code path | Compile error or test failure | Add missing method/case to executor |
| Factory creates wrong executor for config | Wrong git behavior | Test each workflow type in factory_test.go |
| Worktree cleanup not called on failure | Leftover directories | Ensure executor.Complete handles cleanup |

## Do-Nothing Option

The 22-parameter constructor continues to work. Cost: every new workflow variant adds more fields and branches to the already-overloaded processor. The `processPrompt` method grows with each new feature flag.

## Acceptance Criteria

- [ ] `NewProcessor` has 12 or fewer parameters
- [ ] `processor` struct has 14 or fewer fields
- [ ] `processor.go` is under 700 lines
- [ ] A `WorkflowExecutor` interface exists
- [ ] At least two concrete workflow executor implementations (direct, PR)
- [ ] `processPrompt` does not branch on `p.worktree` or `p.pr` directly
- [ ] `make precommit` passes
- [ ] No behavioral changes — all existing tests pass

## Verification

Run `make precommit` — must pass.
