---
status: prompted
approved: "2026-04-16T19:30:07Z"
generating: "2026-04-16T19:30:07Z"
prompted: "2026-04-16T19:38:24Z"
branch: dark-factory/split-processor-workflow
---

## Summary

The processor package owns queue management, not git workflow orchestration. After this change, the orchestration of a prompt's git lifecycle — branch creation, clone setup, PR opening, merge — lives behind a small interface that the factory wires in, and the processor is free of `if p.worktree` / `if p.pr` branches. Adding a new workflow variant means writing a new implementation of that interface, not editing the processor.

## Problem

`pkg/processor/processor.go` is the entry point for executing a queued prompt, but it currently owns far more than that:

- The struct has 20 fields, many of them git-workflow-specific (`Brancher`, `Cloner`, `PRCreator`, `PRMerger`, `Releaser`, `worktree bool`, `pr bool`, ...).
- The constructor takes 22 parameters.
- `processPrompt` branches on `p.worktree` and `p.pr` to pick its git setup and completion paths, so every new workflow variant adds another boolean field + another branch.
- Queue scanning, git-workflow setup, release orchestration, and spec auto-completion all share the same struct.

This forces every caller of `NewProcessor` (just the factory today, but tests in the future) to wire all five git abstractions even when a given workflow uses only one or two of them. It also means the processor package has to understand every workflow variant — adding a fifth workflow would add a fifth code path inside `processPrompt`.

## Goal

After this work, adding a new git-workflow variant (e.g., squash-merge mode, stacked-PR mode) is a local change: author a new implementation of the workflow interface, wire it in the factory, done. The processor package does not know that new variant exists.

## Assumptions

- The three existing workflow variants (direct, PR, worktree/clone) have distinct but non-overlapping setup and completion behavior — a single interface with `Setup` and `Complete` (or similar) can express all of them.
- Existing tests cover each workflow path end-to-end; moving logic behind an interface does not create an untested gap.
- Counterfeiter can generate fakes for the new interface cleanly (standard Go interface, no generics, no channels).
- The dual-context pattern (`gitCtx` for git operations, `ctx` for everything else) survives the extraction — both contexts are available to the workflow implementation.
- No prompt currently relies on observing a workflow-specific internal state on the processor via test fakes; consumers see the `Processor` public interface only.

## Non-Goals

- Splitting `prompt.Manager` — separate spec (`split-prompt-manager.md`).
- Deduplicating factory.go — separate spec (`factory-dedup.md`).
- Sharing the runner/oneshot startup sequence — separate spec (`runner-startup-consolidation.md`).
- Moving code out of `pkg/processor` — the workflow implementations live in `pkg/processor` too.
- Changing PR-creation behavior, merge behavior, or release behavior.
- Changing the `Processor` public interface (`Process`, `ProcessQueue`).
- Changing the prompt status lifecycle or file-move semantics.

## Desired Behavior

1. A `WorkflowExecutor` interface in `pkg/processor` expresses the git-lifecycle contract the processor needs from any workflow variant. Method shape is implementation detail; at minimum it covers pre-execution setup (branch/clone/worktree) and post-execution completion (commit, push, PR, merge, cleanup).
2. Three concrete implementations exist in `pkg/processor`, one per current workflow variant (direct, PR, worktree/clone). They own the git abstractions they need; the processor does not.
3. The factory picks the implementation based on config and passes it into `NewProcessor`. The processor receives a single `WorkflowExecutor`, not five git helpers.
4. `processPrompt` delegates setup and completion to the executor. It contains no `if p.worktree` or `if p.pr` branches.
5. Adding a fourth workflow variant requires: writing a new `WorkflowExecutor` implementation, adding a factory-selection branch, writing tests. No edits to `processor.go`.
6. Tests for workflow-specific behavior move to tests of the concrete executor; processor tests fake the executor and assert delegation.

## Constraints

- No behavioral changes. Every existing test passes unchanged except for test setup that constructs the processor.
- `Processor` public interface unchanged.
- Dual-context pattern preserved — the executor accepts both contexts at its method boundaries or receives them via construction.
- All new code follows project conventions: `github.com/bborbe/errors` wrapping (no `fmt.Errorf`, no bare `return err`), Ginkgo/Gomega tests, Counterfeiter fakes with package prefix, `libtime` for time.
- The existing `Brancher`, `Cloner`, `PRCreator`, `PRMerger`, `Releaser` interfaces keep working — they become fields of concrete `WorkflowExecutor` implementations, not the processor.
- Extraction is complete in one cut-over: the processor does not carry both old and new code paths simultaneously.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| A workflow variant loses a code path during extraction (e.g., cleanup missed) | Integration test for that variant fails | Add missing call to the concrete executor |
| Factory selects wrong executor for a given config | Wrong git behavior in E2E test | Add config-to-executor selection test in `factory_test.go` |
| Processor still holds a direct git dependency after refactor | Code review rejects, or AST-level check catches it | Move the dependency into the executor |
| Counterfeiter fails to generate a fake for the new interface | Build fails | Simplify interface (no generics, no embedded interfaces with generics) |
| Worktree cleanup skipped on failure path | Leftover directories — existing integration test catches it | Executor's `Complete` method handles cleanup in a `defer` or error path |
| Test suite depends on processor exposing `p.worktree` / `p.pr` booleans | Compile error in tests | Refactor test to fake the executor instead |

## Do-Nothing Option

The processor keeps growing with every workflow variant. The 22-parameter constructor becomes 30, the 20-field struct becomes 28, and `processPrompt`'s branching complexity compounds. Each new variant takes longer to add safely and is more likely to regress the others. Test setup in factory tests remains a 37-arg call that's hostile to change.

## Acceptance Criteria

- [ ] `processor.processPrompt` contains no conditional branching on workflow type (no `if p.worktree`, `if p.pr`, etc. selecting git behavior).
- [ ] `pkg/processor/processor.go` does not import `pkg/git/brancher`, `pkg/git/cloner`, `pkg/git/prcreator`, or `pkg/git/prmerger` at package level.
- [ ] Any git interface that processor.go still references is reached only through the `WorkflowExecutor` abstraction, not held on the processor struct as a direct field.
- [ ] A `WorkflowExecutor` interface exists in `pkg/processor` with a generated counterfeiter fake under `mocks/`.
- [ ] At least three concrete `WorkflowExecutor` implementations exist covering every current workflow variant; each has its own test file.
- [ ] Adding a hypothetical fourth variant (demonstrated as a test-only fake) requires zero edits in `processor.go` and `processor_test.go`.
- [ ] Factory tests assert the correct executor type is chosen for each workflow config.
- [ ] All existing processor tests pass; only test-setup code changes (constructor args reduce).
- [ ] `make precommit` passes.
- [ ] `go vet` and golangci-lint detect no regressions.

## Verification

```
cd ~/Documents/workspaces/dark-factory
make precommit
```

End-to-end sanity: run a prompt through each of the three workflow variants against a sandbox repo and confirm identical git state to today's behavior (commit, push, PR if applicable, worktree cleanup if applicable).
