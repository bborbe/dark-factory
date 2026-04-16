---
status: generating
approved: "2026-04-16T19:30:10Z"
generating: "2026-04-16T19:38:24Z"
branch: dark-factory/factory-dedup
---

## Summary

`pkg/factory/factory.go` constructs the same sub-systems in several places instead of going through one helper. Status-checker setup is duplicated three times, container-counter construction is duplicated five times, and two single-caller helpers were extracted only to appease the function-length linter. This spec collapses the duplicates into named helpers and removes the fake seams.

## Problem

`factory.go` wires the entire dark-factory runtime together. Over time it accumulated:

- **Three identical status-checker setups.** Each path that needs a status checker reloads `globalconfig`, constructs the same deps, and calls `NewStatusChecker`. The three sites drift independently — a fix to one does not propagate to the others.
- **Five near-identical container-counter blocks.** Each call site builds a `NewDockerContainerCounter` that differs only in which container-name filter is applied. Adding a sixth variant means copying the block again.
- **Two naive extractions.** `createDockerExecutor` and `createRunnerInstance` are called from exactly one place each; they were lifted out to bring a parent function under the `funlen` threshold, not because they form a meaningful seam. They make the file harder to read by spreading one construction sequence across three functions.

The three concerns are structurally the same: one construction site should exist per dependency, and factory.go should read top-to-bottom as wiring, not as a sequence of copy-paste blocks.

## Goal

After this work, `factory.go` has one place to change when a dependency's construction changes. Adding a new container-counter variant takes one line at the call site; adding a new consumer of the status checker takes one helper call. The file is shorter and easier to scan.

## Assumptions

- `globalconfig.Load` is deterministic and side-effect-free — calling it once and reusing the result is observationally equivalent to calling it three times.
- The five container-counter variants differ only in their name-filter argument, not in any other dependency. (Verified by reading current call sites.)
- `createDockerExecutor` and `createRunnerInstance` are each called from exactly one site today. (Verified by grep.)
- The `funlen` linter threshold for parent functions can be satisfied by inlining these helpers plus minor local restructuring; if not, `//nolint:funlen` with a justification comment is an acceptable escape hatch.
- No test fakes the helpers directly — they're internal factory plumbing.

## Non-Goals

- Changing the processor, runner, or any consumer of what factory.go builds.
- Changing `globalconfig` behavior or the `StatusChecker` interface.
- Changing what container counters count or how filters are expressed.
- Splitting `pkg/factory` into multiple packages.
- Reducing the factory's total line count as a primary goal — readability matters more than line count.

## Desired Behavior

1. Exactly one helper in `factory.go` constructs a `StatusChecker`. Every status-checker call site invokes that helper; no call site calls `globalconfig.Load` or `NewStatusChecker` directly.
2. Exactly one helper constructs container-counter dependencies. It accepts the name-filter argument and returns the ready counter (plus any co-dependencies it needs to expose). No call site constructs `NewDockerContainerCounter` directly.
3. `createDockerExecutor` and `createRunnerInstance` are inlined at their single call sites. Any resulting `funlen` violation on the caller is resolved by restructuring the caller's local scope or by adding `//nolint:funlen` with a comment explaining why the function is legitimately long (e.g., "composition root — wires N subsystems; splitting hides the order of wiring"). If, during implementation, a second caller emerges for either helper, the spec is out of date — raise it for discussion rather than silently retaining the helper.
4. Adding a new container-counter variant requires one edit at the call site — passing a new filter to the helper — and no new helper block.

## Constraints

- No behavioral changes. Every existing factory test passes unchanged.
- The factory's public API (`CreateProcessor`, `CreateRunner`, `CreateOneshot`, ...) does not change signatures or semantics.
- New code follows project conventions: `github.com/bborbe/errors` wrapping, Ginkgo/Gomega tests, `libtime`.
- The file's logical top-to-bottom wiring order (config → identity → git → prompt → workflow → processor) is preserved. Deduplication must not reorder wiring in ways that change initialization order.
- The composition-root nature of `factory.go` is acceptable — this spec does NOT target the file's overall length or the number of top-level factory functions.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Status-checker helper accidentally shares mutable state between call sites | A call site mutates state observed by another — integration test fails | Helper returns a new instance per call; test asserts independence |
| Container-counter helper's signature can't accommodate one variant's filter | Compile error | Widen the argument (e.g., `filter func(string) bool`) rather than adding a parallel helper |
| Inlining a naive extraction re-triggers `funlen` on the caller | Lint failure at `make precommit` | Add `//nolint:funlen` with justification comment OR restructure the caller's local scope |
| `globalconfig.Load` is silently called fewer times than before and a subscriber misses an update | Integration test asserts `globalconfig` load count OR test fakes the loader and asserts behavior | If load count matters, document it and load explicitly at the affected site |
| Hidden ordering dependency between the three former status-checker blocks | Runtime bug in factory startup | Test factory construction end-to-end with a fake globalconfig asserting expected call order |

## Do-Nothing Option

factory.go keeps accreting duplicate blocks. Each new dependency or counter variant adds another copy-paste. Drift between duplicates causes bugs that are hard to see in code review because the three blocks look identical at a glance. Naive extractions multiply — every new 81-line composition root grows its own `createX` / `createY` single-caller helpers.

## Acceptance Criteria

- [ ] `grep "globalconfig.Load" pkg/factory/factory.go` returns at most one match; that match is inside the status-checker helper.
- [ ] `grep "NewDockerContainerCounter" pkg/factory/factory.go` returns at most one match; that match is inside the container-counter helper.
- [ ] The status-checker helper is called from every site that previously constructed one (three sites → three calls).
- [ ] Adding a hypothetical sixth container-counter variant (demonstrated in a test or PR description) takes exactly one line at the call site.
- [ ] `createDockerExecutor` and `createRunnerInstance` are either absent or retained with `//nolint:funlen` + an explanatory comment naming which parent function would otherwise exceed the threshold and why splitting would harm readability.
- [ ] No existing factory test requires modification except where test setup calls a helper that was renamed.
- [ ] `make precommit` passes with zero new `funlen` violations.

## Verification

```
cd ~/Documents/workspaces/dark-factory
make precommit
grep -c "NewDockerContainerCounter\|globalconfig.Load" pkg/factory/factory.go
```

The grep count should be ≤ 2 (one of each, inside the helpers).
