---
status: draft
---

## Summary

Three intertwined cleanups in one coordinated refactor:

- `processor` struct split — git workflow extracted to `WorkflowExecutor`, constructor drops from 22 → ~10 params, file from 1,146 → < 700 lines
- `factory.go` deduplicated — `createStatusChecker` helper (3 duplicate `globalconfig.Load` calls collapse), counter construction folded into `createContainerDeps` (5 duplicate `NewDockerContainerCounter` calls collapse), naive extractions inlined (`createDockerExecutor`, `createRunnerInstance`)
- `runner` / `oneshot` — 8-method startup forwarding layer replaced by shared `startupSequence` free function in `pkg/runner/lifecycle.go`

No behavioral changes. Tests unchanged except for constructor wiring.

## Problem

Three architectural smells that compound:

1. **God Processor** — `pkg/processor/processor.go` is 1,146 lines, struct has 20 fields, constructor takes 22 params. Mixes queue scanning, git workflow setup, release orchestration, and spec auto-completion.

2. **Bloated Factory** — `CreateProcessor` in `pkg/factory/factory.go` takes 37 parameters because it wires every dep the god processor consumes. Factory also contains:
   - 3 separate `globalconfig.Load` calls for status checker deps
   - 5 separate `NewDockerContainerCounter` constructions that differ only by container filter
   - `createDockerExecutor` / `createRunnerInstance` — single-caller wrappers extracted to satisfy `funlen`, not to create a real seam

3. **Runner/Oneshot duplication** — `pkg/runner/runner.go` and `pkg/runner/oneshot.go` duplicate an 8-method startup forwarding layer. Both call `cfg.Validate → promptMgr.ResetExecuting → promptMgr.NormalizeFilenames → git.Fetch → git.MergeOriginDefault → scanner.Scan → …` in the same order.

The factory bloat is downstream of (1) — a smaller processor needs fewer deps. The naive extractions inside factory.go are symptoms of its size. The runner duplication (3) is independent but shares the same "copy-paste instead of shared helper" failure mode.

## Goal

After this refactor:

- `pkg/processor/processor.go` is under 700 lines
- `NewProcessor` has ≤ 12 params; struct has ≤ 14 fields
- `CreateProcessor` has ≤ 16 params (natural shrinkage once processor is split)
- `factory.go` has one `createStatusChecker(ctx, cfg, clock) (StatusChecker, error)` helper
- `factory.go` has one `createContainerDeps(...)` that produces the counter instead of constructing it 5 separate times
- `createDockerExecutor` and `createRunnerInstance` inlined at their single call sites (or retained with `//nolint:funlen` + justification)
- `pkg/runner/lifecycle.go` exports `startupSequence(ctx, deps StartupDeps) error`; `runner.go` and `oneshot.go` call it instead of forwarding 8 methods

## Non-Goals

- Changing prompt lifecycle (queue → executing → completed)
- Splitting `prompt.Manager` — that's `split-prompt-manager.md`
- Moving packages; nothing leaves `pkg/processor`, `pkg/factory`, or `pkg/runner`
- Changing worktree, PR, or auto-merge semantics
- Extracting `determineBump` — already handled by prompt 196

## Desired Behavior

1. `WorkflowExecutor` interface in `pkg/processor` owns branch/clone/PR/merge lifecycle with methods like `Setup(ctx, branchName) error` and `Complete(ctx, promptPath, title) error`
2. Concrete workflow executor implementations: direct, PR, worktree/clone — selected by factory based on config
3. `processor.processPrompt` delegates to `WorkflowExecutor` instead of branching on `p.worktree` / `p.pr`
4. `createStatusChecker` centralizes `globalconfig.Load` + `NewStatusChecker` construction
5. `createContainerDeps` accepts a container-filter argument and returns ready-to-use deps including the counter
6. `startupSequence` is called from both runner variants; neither file knows about internal ordering of the 8 startup steps

## Constraints

- No behavioral changes — all existing tests pass
- The `Processor`, `Runner`, and `Oneshot` public interfaces do not change
- Keep the dual-context pattern (`gitCtx`, `ctx`) — document it but don't change it
- Counterfeiter mocks regenerate cleanly

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---------|-------------------|----------|
| Workflow executor missing a code path | Compile error or test failure | Add missing method/case |
| Factory creates wrong executor for config | Wrong git behavior | Test each workflow type in `factory_test.go` |
| Startup sequence ordering bug | Integration test failure | Order is a single function — easy to audit |
| Worktree cleanup not called on failure | Leftover directories | `Executor.Complete` handles cleanup |

## Do-Nothing Option

All three smells continue to grow: every new workflow variant adds fields/branches to processor, every new counter variant adds another factory block, every new startup step gets copy-pasted into both runner files.

## Acceptance Criteria

- [ ] `NewProcessor` ≤ 12 params; `processor` struct ≤ 14 fields
- [ ] `processor.go` < 700 lines
- [ ] `CreateProcessor` ≤ 16 params
- [ ] `WorkflowExecutor` interface + ≥ 2 concrete implementations exist
- [ ] `createStatusChecker` helper exists; called in all 3 former sites
- [ ] `createContainerDeps` constructs the counter; no direct `NewDockerContainerCounter` calls outside it
- [ ] `createDockerExecutor` and `createRunnerInstance` inlined or explicitly `//nolint:funlen`'d with justification
- [ ] `startupSequence` function exists in `pkg/runner/lifecycle.go`
- [ ] `runner.go` and `oneshot.go` each call `startupSequence` exactly once; no 8-method forwarding remains
- [ ] `make precommit` passes
- [ ] No behavioral changes — all existing tests pass

## Verification

Run `make precommit` — must pass.
