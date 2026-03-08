---
status: draft
---

## Summary

- Replace all `time.Now()` calls and `nowFunc func() time.Time` fields with `libtime.CurrentDateTimeGetter` from `github.com/bborbe/time`
- Inject via constructors following the bborbe ecosystem pattern
- Add `CurrentDateTimeGetter` parameter to `prompt.Load()`, `spec.Load()`, `NewManager()`, `NewWatcher()`, `NewPRMerger()`, and all cmd/server/generator constructors that call them
- Thread the getter through the factory layer
- All production code becomes deterministically testable — zero direct `time.Now()` calls

## Problem

The codebase uses `time.Now()` directly in 4 production locations and has a `nowFunc func() time.Time` field with nil-guard fallback in `PromptFile` and `SpecFile`. This violates the bborbe ecosystem pattern where time is injected via `libtime.CurrentDateTimeGetter` in constructors. The current approach makes code non-deterministic in tests and inconsistent with other bborbe projects.

## Goal

After this work, no production Go file calls `time.Now()` directly. All time access goes through `libtime.CurrentDateTimeGetter` injected via constructors. Tests use `libtime.NewCurrentDateTime()` with `SetNow()` for deterministic time.

## Non-goals

- Migrating logging from `slog` to `glog`
- Refactoring `PromptFile`/`SpecFile` beyond time injection
- Changing the `Frontmatter` timestamp format (stays RFC3339)
- Adding `CurrentDateTimeSetter` to production code — only `CurrentDateTimeGetter` in constructors, `CurrentDateTime` (getter+setter) in tests

## Desired Behavior

1. `prompt.Load(ctx, path, currentDateTimeGetter)` accepts a `CurrentDateTimeGetter` parameter and passes it to the constructed `PromptFile`
2. `spec.Load(ctx, path, currentDateTimeGetter)` accepts a `CurrentDateTimeGetter` parameter and passes it to the constructed `SpecFile`
3. `prompt.NewManager(...)` accepts `CurrentDateTimeGetter` and passes it through its `Load()` method
4. `watcher.NewWatcher(...)` accepts `CurrentDateTimeGetter` and uses it in `stampCreatedTimestamps` instead of `time.Now()`
5. `git.NewPRMerger(...)` accepts `CurrentDateTimeGetter` and uses it in `WaitAndMerge` instead of `time.Now()`
6. All cmd constructors (`NewApproveCommand`, `NewRequeueCommand`, `NewListCommand`, etc.) accept `CurrentDateTimeGetter` and pass to `prompt.Load()` calls
7. `pkg/factory/factory.go` creates `libtime.NewCurrentDateTime()` once and threads it through all constructors
8. `grep -rn "time\.Now()" pkg/ --include="*.go" | grep -v "_test.go"` returns zero results
9. All tests use `libtime.NewCurrentDateTime()` with `SetNow()` — no `func() time.Time` closures

## Constraints

- `PromptFile` and `SpecFile` struct types remain exported — only internal fields change
- `Frontmatter` struct is unchanged
- `Manager` interface is unchanged — only the constructor gets a new parameter
- `Watcher` and `PRMerger` interfaces are unchanged
- All existing tests must pass (updated to use new constructor signatures)
- `make precommit` must pass
- Do NOT use counterfeiter mock for `CurrentDateTimeGetter` — use `libtime.NewCurrentDateTime()` with `SetNow()` (per coding guidelines)
- `github.com/bborbe/time` is already an indirect dependency — promote to direct

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Caller passes nil CurrentDateTimeGetter | Panic on first `.Now()` call — fail fast | Compiler enforces non-nil via required constructor param |
| Test forgets to set time | Uses real time (NewCurrentDateTime default) | Acceptable — only tests needing fixed time must call SetNow |

## Acceptance Criteria

- [ ] `prompt.Load()` signature includes `CurrentDateTimeGetter` parameter
- [ ] `spec.Load()` signature includes `CurrentDateTimeGetter` parameter
- [ ] `prompt.NewManager()` accepts and stores `CurrentDateTimeGetter`
- [ ] `watcher.NewWatcher()` accepts `CurrentDateTimeGetter`
- [ ] `git.NewPRMerger()` accepts `CurrentDateTimeGetter`
- [ ] All cmd constructors accept `CurrentDateTimeGetter`
- [ ] Factory creates one `libtime.NewCurrentDateTime()` and passes to all
- [ ] Zero `time.Now()` calls in production code
- [ ] Zero `nowFunc` fields in production code
- [ ] All tests use `libtime.NewCurrentDateTime()` pattern
- [ ] `make precommit` passes

## Verification

```bash
# No time.Now() in production code
grep -rn "time\.Now()" pkg/ --include="*.go" | grep -v "_test.go"
# Expected: no output

# No nowFunc remaining
grep -rn "nowFunc" pkg/ --include="*.go"
# Expected: no output

# libtime imported where needed
grep -rn "bborbe/time" pkg/ --include="*.go" | grep -v "_test.go" | grep -v vendor
# Expected: prompt.go, spec.go, watcher.go, pr_merger.go, factory.go, plus cmd files

make precommit
```

## Do-Nothing Option

The current `nowFunc` approach works but violates ecosystem conventions. Every new developer or AI agent unfamiliar with the codebase will use `time.Now()` by default. The longer we wait, the more callsites accumulate. The migration is mechanical and low-risk.
