---
status: prompted
approved: "2026-04-16T19:30:10Z"
generating: "2026-04-16T19:41:15Z"
prompted: "2026-04-16T19:46:39Z"
branch: dark-factory/runner-startup-consolidation
---

## Summary

The daemon runner (`pkg/runner/runner.go`) and the one-shot runner (`pkg/runner/oneshot.go`) duplicate the same 8-step startup sequence: config validation, executing-state reset, filename normalization, git fetch, merge-origin-default, queue scan, and two further steps in the same order. Any new startup step today has to be added to both files. This spec extracts that sequence into a single function in `pkg/runner/lifecycle.go` that both runners call.

## Problem

Both runners expose different user-facing behavior — the daemon loops, the one-shot exits after a single pass — but both must prepare the same runtime state before they can do anything useful. Today, that preparation is hand-written in both files. The ordering is identical, the error-wrapping is identical, the dependency list is identical.

Because the sequence is copy-pasted, every new startup concern (a new validation, a new reset, a new fetch target) has to be added twice. Worse, drift between the two is invisible in code review: the reviewer sees one file at a time and easily misses a missing step in the second. A sequence that must be identical is exactly the sequence that should exist once.

## Goal

After this work, adding a startup step is a one-file change. The daemon runner and one-shot runner both invoke a shared `startupSequence`, pass it a struct of dependencies, and do not know the internal order of the steps.

## Assumptions

- The startup sequence is genuinely identical today in both runners. (Verified by reading both files — same call order, same error wrapping, same deps.)
- Every startup step is idempotent, so re-ordering within the helper (for clarity) is safe — though this spec does not reorder.
- Both runners already receive identical dependency sets (prompt manager, git operations, scanner, config) via their constructors. Packaging them into a `StartupDeps` struct is bookkeeping, not new coupling.
- The one-shot runner's `ListQueued` call immediately after startup is NOT part of startup — it's the one-shot's execution phase. The shared sequence covers only the steps before prompt selection.
- No mode-specific startup step exists today. If one is discovered during implementation, it remains in the per-runner file.

## Non-Goals

- Changing what any startup step does.
- Changing the order of startup steps.
- Unifying the daemon loop and the one-shot execution — those stay separate.
- Changing `Runner` or `Oneshot` public interfaces.
- Changing the prompt lifecycle, container execution, or post-execution phases.
- Moving startup steps from `pkg/runner` into another package.

## Desired Behavior

1. `pkg/runner/lifecycle.go` exports a `startupSequence(ctx context.Context, deps StartupDeps) error` function (or equivalent shape — a receiver method on a helper type is acceptable).
2. `StartupDeps` is a struct carrying every dependency the sequence needs: config validator, prompt manager, git operations, scanner, anything currently consumed by the 8 steps.
3. `runner.go` calls `startupSequence` exactly once before entering its main loop. The 8 individual step calls that currently live in `runner.go` are removed.
4. `oneshot.go` calls `startupSequence` exactly once before beginning its single-pass execution. The 8 individual step calls that currently live in `oneshot.go` are removed.
5. Errors from `startupSequence` propagate unchanged — both runners see the same error shape and wrapping as today.
6. A unit test asserts both runners produce the same startup-call ordering by recording calls on a fake `StartupDeps`.
7. Adding a new startup step requires editing `lifecycle.go` and `StartupDeps` only.

## Constraints

- No behavioral changes. Existing runner tests pass unchanged except for setup code that builds `StartupDeps` instead of passing deps individually.
- Error wrapping uses `github.com/bborbe/errors` — no bare `return err`, no `fmt.Errorf`.
- Context propagation preserved: both runners pass their own context to `startupSequence`.
- The daemon runner retains full control over when `startupSequence` is called — re-running it on daemon restart, for instance, is still possible.
- `startupSequence` is NOT exported from `pkg/runner` unless another package needs it.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| A startup step is added to one runner but not migrated | Unit test comparing call orders fails | Move the step into `startupSequence` |
| `StartupDeps` missing a field used by one step | Compile error | Add field, wire both runners to populate it |
| Error wrapping diverges from today's shape | Integration test asserting error messages fails | Match wrapping exactly in `startupSequence` |
| Hidden mode-specific step discovered mid-refactor | Do not force it into the shared sequence | Keep it in the per-runner file with a comment explaining why |
| Step order accidentally reordered during extraction | Integration test for startup behavior catches it | Preserve exact order; add a test that records call order |
| One runner starts passing a different context than the other | Subtle cancellation-behavior drift | Both runners pass their own `ctx`; the shared function never defaults the context |

## Do-Nothing Option

Every new startup concern — a new validation, a new normalizer, a new git pre-fetch — requires editing two files and hoping the reviewer spots the mismatch if one is missed. Drift is invisible: both files compile and tests pass per-file even when the sequences have diverged. The runner package pays a documentation tax because "what does dark-factory do at startup?" has two answers.

## Acceptance Criteria

- [ ] `pkg/runner/lifecycle.go` contains a `startupSequence` function (or method) that encapsulates every startup step today duplicated across `runner.go` and `oneshot.go`.
- [ ] `runner.go` calls `startupSequence` exactly once and contains zero direct calls to the 8 startup steps.
- [ ] `oneshot.go` calls `startupSequence` exactly once and contains zero direct calls to the 8 startup steps.
- [ ] A unit test records calls on a fake `StartupDeps`, runs both runners' startup phases, and asserts both invoke the same steps in the same order.
- [ ] Adding a hypothetical new startup step (demonstrated in a test-only branch) takes exactly one edit in `lifecycle.go` plus one field on `StartupDeps`.
- [ ] All existing runner tests pass; only test setup that constructs the runner changes.
- [ ] `make precommit` passes.
- [ ] Error messages from startup failures remain byte-identical to today (verified by an existing or new error-message test).

## Verification

```
cd ~/Documents/workspaces/dark-factory
make precommit
```

End-to-end: start the daemon and run a single prompt through the one-shot. Both should produce identical startup log lines (or the same absence of startup log lines) as today.
