---
status: created
---
<summary>
- All time-dependent code becomes deterministically testable without patching
- A single shared time source is wired through constructors from the factory layer
- The old per-struct setter pattern (`SetNowFunc`) is removed
- Callers that pass no time source fail fast at first use (no silent defaults)
</summary>

<objective>
Eliminate direct `time.Now()` calls from production code by injecting `libtime.CurrentDateTimeGetter` via constructors. This aligns with the bborbe ecosystem convention and makes all time-dependent code deterministically testable.
</objective>

<context>
Read CLAUDE.md for project conventions.

Production `time.Now()` callsites (4 total):
- `pkg/prompt/prompt.go` line ~291: `now()` method nil-guard fallback returns `time.Now()`
- `pkg/spec/spec.go` line ~63: `now()` method nil-guard fallback returns `time.Now()`
- `pkg/watcher/watcher.go` line ~189: `stampCreatedTimestamps` uses `time.Now().UTC().Format(time.RFC3339)`
- `pkg/git/pr_merger.go` line ~47,56: `WaitAndMerge` uses `time.Now()` for deadline

Current anti-pattern in `PromptFile` and `SpecFile`:
- `nowFunc func() time.Time` field with nil-guard (line ~200 in prompt.go, line ~58 in spec.go)
- `SetNowFunc()` setter on SpecFile (line ~69 in spec.go)
- `time.Now` assigned in every `Load()` constructor branch

`github.com/bborbe/time` is already an indirect dependency (v1.22.0 in go.sum). The `CurrentDateTimeGetter` interface has a single `Now() time.Time` method. In production, use `libtime.NewCurrentDateTime()`. In tests, use the same but call `SetNow()` for deterministic time.

Factory wiring: `pkg/factory/factory.go` creates all dependencies. It should create one `libtime.NewCurrentDateTime()` and pass it to constructors that need it.
</context>

<requirements>
1. Add `libtime.CurrentDateTimeGetter` parameter to `prompt.Load()` signature. Store it on `PromptFile` replacing `nowFunc`. Remove the nil-guard in `now()` — just call `p.currentDateTimeGetter.Now()`. Remove both `nowFunc: time.Now` assignments in Load's branches.

2. Add `libtime.CurrentDateTimeGetter` parameter to `spec.Load()` signature. Store it on `SpecFile` replacing `nowFunc`. Remove `SetNowFunc()` method. Remove nil-guard in `now()`. Remove all `nowFunc: time.Now` assignments in Load's branches.

3. Add `libtime.CurrentDateTimeGetter` parameter to `prompt.NewManager()` and store it. Pass it through to `prompt.Load()` calls inside the manager.

4. Add `libtime.CurrentDateTimeGetter` parameter to `watcher.NewWatcher()`. Use `getter.Now()` in `stampCreatedTimestamps` instead of `time.Now()`.

5. Add `libtime.CurrentDateTimeGetter` parameter to `git.NewPRMerger()`. Use `getter.Now()` in `WaitAndMerge` instead of `time.Now()`.

6. Update all production callsites of `prompt.Load()` and `spec.Load()` to pass the getter. Each `cmd` package constructor (e.g., `NewApproveCommand`, `NewRequeueCommand`, `NewListCommand`, `NewCombinedListCommand`, `NewPromptShowCommand`, `NewPromptVerifyCommand`, `NewSpecShowCommand`, `NewSpecApproveCommand`, `NewSpecCompleteCommand`) receives `CurrentDateTimeGetter` as a parameter and passes it to `prompt.Load()` or `spec.Load()`. Same for `pkg/server/queue_helpers.go`, `pkg/specwatcher/watcher.go`, `pkg/generator/generator.go`, and `pkg/spec/spec.go` where they call `prompt.Load()`.

7. In `pkg/factory/factory.go`: create `libtime.NewCurrentDateTime()` once at the top of `CreateRunner()` and `CreateOneShotRunner()`. Thread it through `createPromptManager()`, `CreateWatcher()`, `CreateProcessor()`, and each `Create*Command()` factory function that needs it. Each factory function receives the getter and passes it to the corresponding constructor.

8. Update all test files (search `pkg/**/*_test.go` for `prompt.Load(` and `spec.Load(`):
   - Replace `nowFunc` assignments with `libtime.NewCurrentDateTime()` + `SetNow()` where deterministic time is needed
   - Pass the getter to updated `Load()`, `NewManager()`, `NewWatcher()`, `NewPRMerger()` signatures
   - Do NOT use counterfeiter mocks for `CurrentDateTimeGetter` — use `libtime.NewCurrentDateTime()` with `SetNow()`

9. Promote `github.com/bborbe/time` from indirect to direct dependency: run `go get github.com/bborbe/time` then `go mod tidy`.
</requirements>

<implementation>
Before (prompt.go):
```go
nowFunc func() time.Time
// ...
func (pf *PromptFile) now() time.Time {
    if pf.nowFunc == nil {
        return time.Now()
    }
    return pf.nowFunc()
}
func Load(ctx context.Context, path string) (*PromptFile, error) {
    // ...
    pf := &PromptFile{Path: path, Body: content, nowFunc: time.Now}
```

After (prompt.go):
```go
currentDateTimeGetter libtime.CurrentDateTimeGetter
// ...
func (pf *PromptFile) now() time.Time {
    return pf.currentDateTimeGetter.Now()
}
func Load(ctx context.Context, path string, currentDateTimeGetter libtime.CurrentDateTimeGetter) (*PromptFile, error) {
    // ...
    pf := &PromptFile{Path: path, Body: content, currentDateTimeGetter: currentDateTimeGetter}
```

Apply the same pattern to `spec.go`, `watcher.go`, and `pr_merger.go`.
</implementation>

<constraints>
- Do NOT commit — dark-factory handles git
- `PromptFile` and `SpecFile` struct types remain exported — only internal fields change
- `Frontmatter` struct is unchanged
- `Manager`, `Watcher`, `PRMerger` interfaces are unchanged — only constructors get new parameters
- All existing tests must pass with updated signatures
- Do NOT use counterfeiter mock for `CurrentDateTimeGetter`
- Caller passing nil `CurrentDateTimeGetter` should panic on first `.Now()` call — fail fast, no nil guards
</constraints>

<verification>
```bash
# No time.Now() in production code
grep -rn "time\.Now()" pkg/ --include="*.go" | grep -v "_test.go"
# Expected: no output

# No nowFunc remaining
grep -rn "nowFunc" pkg/ --include="*.go"
# Expected: no output

# No SetNowFunc remaining
grep -rn "SetNowFunc" pkg/ --include="*.go"
# Expected: no output

# libtime imported where needed
grep -rn "bborbe/time" pkg/ --include="*.go" | grep -v "_test.go" | grep -v vendor
# Expected: prompt.go, spec.go, watcher.go, pr_merger.go, factory.go, plus cmd files

make precommit
```
Must pass with no errors.
</verification>

<success_criteria>
- Zero `time.Now()` calls in production code (grep returns nothing)
- Zero `nowFunc` fields or `SetNowFunc` methods
- All time access via `libtime.CurrentDateTimeGetter` injected through constructors
- Factory creates single `libtime.NewCurrentDateTime()` and threads it
- All tests use `libtime.NewCurrentDateTime()` with `SetNow()` for deterministic time
- `make precommit` passes
</success_criteria>
