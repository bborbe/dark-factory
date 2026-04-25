---
status: committing
container: dark-factory-337-unify-oneshot-with-callback
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T15:15:00Z"
queued: "2026-04-25T14:02:32Z"
started: "2026-04-25T14:02:34Z"
---

<summary>
- Delete `processor.ProcessQueue` (the one-shot variant) and the duplicate loop in `pkg/runner/oneshot.go`; keep only `processor.Process`
- Add a `NothingToDoCallback` constructor parameter to `processor` — fired by `Process` whenever a tick ends with no progress (no prompts moved to completed, no specs auto-transitioned)
- Daemon entrypoint passes a log-only callback (`slog.Info("nothing to do, waiting for changes")`)
- One-shot entrypoint (`dark-factory run`) passes a callback that calls `cancel()` on the processor's context — `Process` exits cleanly via the existing `<-ctx.Done()` case
- Eliminates the pre-existing `dark-factory run` infinite loop on blocked-by-sequencing prompts: when no progress is made, the one-shot callback cancels and the binary exits
- Single processor implementation; behaviour difference between daemon and one-shot is one callback line
</summary>

<objective>
Unify the daemon and one-shot processor code paths. Today there are two near-duplicate flows (`Process` + ticker loop for daemon, `ProcessQueue` + manual `oneshot.Run` loop for one-shot) that have already drifted (the one-shot loop spins CPU on blocked queues; the daemon does not). Replace both with a single `Process(ctx)` method whose only difference between modes is a constructor-injected callback.
</objective>

<context>
**Prerequisite shipped:** prompt `336-configure-daemon-intervals` already added `queueInterval` and `sweepInterval` constructor parameters to `processor.NewProcessor` and removed the package-level sweep var. This prompt adds `onIdle` as a third constructor parameter alongside those.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` from the coding plugin docs (constructor injection + small interface composition).
Read `go-testing-guide.md` from the coding plugin docs.

Read these files before editing (line numbers are post-336):

- `pkg/processor/processor.go`:
  - `Processor` interface declaration around line 50 — `ProcessQueue(ctx)` method is removed by this prompt
  - `NewProcessor` (line 64) — currently 27 parameters; add `onIdle NothingToDoCallback` as the last one
  - `Process` (line 181) — the daemon's ticker loop; modify to track per-tick progress and fire `onIdle` when a tick is idle
  - `ProcessQueue` (line 245) — the one-shot variant; **delete entirely**
  - Existing `processExistingQueued` (line 528+) and `checkPromptedSpecs` (line 800+) — currently return just `error`. They need to return progress signals so `Process` can detect idle ticks.
- `pkg/runner/oneshot.go::Run` — the duplicate loop that calls `ProcessQueue` repeatedly. **Delete the loop**; replace with a single `processor.Process(ctx)` call.
- `pkg/factory/factory.go::CreateProcessor` — single `NewProcessor` call site. Wire the `onIdle` callback here. Different callbacks for daemon vs one-shot — see step 7.

Authoritative intent (from project owner):

> "Use the normal daemon and add a function callback that is called nothingToDo. The daemon only prints a log message in this loop, and the run version triggers the context cancel shut on the dark-factory thing."

The new callback type lives in `pkg/processor/`:

```go
// NothingToDoCallback fires when a Process tick ends with no progress made.
// Daemon mode passes a log-only callback. One-shot mode passes one that calls cancel().
type NothingToDoCallback func(ctx context.Context, cancel context.CancelFunc)
```

"No progress" definition: at the end of any tick (queue tick OR sweep tick OR `<-p.ready` watcher signal), if zero prompts moved to `prompts/completed/` AND zero specs auto-transitioned to `verifying`, the tick is idle.
</context>

<requirements>

## 1. Add `NothingToDoCallback` and constructor parameter

In `pkg/processor/processor.go`, add the type near the top of the file (after the existing `Processor` interface):

```go
// NothingToDoCallback fires when a Process tick ends with no progress made.
// Daemon mode passes a log-only callback. One-shot mode passes one that calls cancel().
type NothingToDoCallback func(ctx context.Context, cancel context.CancelFunc)
```

Add `onIdle NothingToDoCallback` as the last parameter of `NewProcessor` (after `sweepInterval`):

```go
// onIdle is invoked at the end of any tick that made no progress.
// Pass a log-only callback for daemon mode, or one that calls cancel() for one-shot mode.
// Must not be nil — the factory always supplies one.
onIdle NothingToDoCallback,
```

Store it on the `processor` struct (alongside the interval fields at line 174-175).

If callers might pass nil during tests, defensive default is acceptable: in `NewProcessor`, if `onIdle == nil`, set it to a no-op closure. Document this so test authors don't rely on the silent fallback.

## 2. Track per-tick progress

Define an internal type used by the worker functions:

```go
// tickResult aggregates progress signals from a single Process tick.
type tickResult struct {
    completedPrompts  int
    transitionedSpecs int
}

func (r tickResult) madeProgress() bool {
    return r.completedPrompts > 0 || r.transitionedSpecs > 0
}
```

Modify these existing worker functions to also return progress counts:

- `processExistingQueued` → `(completedPrompts int, err error)` — count is the number of prompts moved to completed/ during the call
- `processCommittingPrompts` → optional: count as `completedPrompts` if it recovers a stuck commit (otherwise return 0)
- `checkPromptedSpecs` → `(transitionedSpecs int, err error)` — count is the number of specs that moved from `prompted` → `verifying`

Use the return-value pattern (signature change) rather than a mutable field on `processor` — `Process` has goroutines and a shared mutable progress field invites races.

## 3. Wire cancellable context

At the top of `Process` (line 181):

```go
ctx, cancel := context.WithCancel(ctx)
defer cancel()
```

The existing `<-ctx.Done()` case in the `select` block already returns nil — no further changes needed there. The cancellable wrapper is what the one-shot callback's `cancel()` will trigger.

## 4. Fire `onIdle` at the end of idle ticks

Wire `onIdle` invocation into the three tick branches of `Process`'s `select` block. Sketch (the existing branches at lines 216, 226, 234 are extended):

```go
case <-p.ready:
    p.skippedPrompts = make(map[string]libtime.DateTime)
    p.processCommittingPrompts(ctx)
    completed, err := p.processExistingQueued(ctx)
    if err != nil { slog.Warn("prompt failed; queue blocked until manual retry", "error", err) }
    if (tickResult{completedPrompts: completed}).madeProgress() {
        continue
    }
    p.onIdle(ctx, cancel)

case <-ticker.C:
    p.processCommittingPrompts(ctx)
    completed, err := p.processExistingQueued(ctx)
    if err != nil { slog.Warn("prompt failed; queue blocked until manual retry", "error", err) }
    if (tickResult{completedPrompts: completed}).madeProgress() {
        continue
    }
    p.onIdle(ctx, cancel)

case <-sweepTicker.C:
    transitioned, err := p.checkPromptedSpecs(ctx)
    if err != nil { slog.Warn("periodic checkPromptedSpecs failed", "error", err) }
    if (tickResult{transitionedSpecs: transitioned}).madeProgress() {
        continue
    }
    p.onIdle(ctx, cancel)
```

The startup-scan calls (lines 184-196 in `Process`, BEFORE the loop) MUST NOT fire `onIdle` — that would cancel one-shot before any work has a chance to start. `onIdle` only fires from inside the `select` block.

**Mandatory funlen mitigation:** `Process` is already long; adding three new tick branches will exceed the funlen limit. Extract three helper methods up front:

```go
// returns true if the tick made progress
func (p *processor) runReadyTick(ctx context.Context) bool { ... }
func (p *processor) runQueueTick(ctx context.Context) bool { ... }
func (p *processor) runSweepTick(ctx context.Context) bool { ... }
```

Each helper does the work of one tick branch and returns whether progress was made. Then `Process`'s `select` body shrinks to:

```go
case <-p.ready:
    if !p.runReadyTick(ctx) { p.onIdle(ctx, cancel) }
case <-ticker.C:
    if !p.runQueueTick(ctx) { p.onIdle(ctx, cancel) }
case <-sweepTicker.C:
    if !p.runSweepTick(ctx) { p.onIdle(ctx, cancel) }
```

This keeps `Process` short and makes each tick branch independently testable.

## 5. Delete `ProcessQueue`

Remove from the `Processor` interface (line 50) and the implementation (line 245). Search for ALL callers and references:

```bash
grep -rn "ProcessQueue" pkg/ main.go
```

Expected hits:
- `pkg/runner/oneshot.go::Run` — replaced in step 6
- `pkg/processor/processor_test.go` — multiple `Describe("ProcessQueue ...")` and `It` blocks calling `p.ProcessQueue(ctx)`. **Delete these blocks entirely** — the equivalent behavior is now exercised by `Process` tests in step 8a. The "do NOT modify existing tests" rule in 8c applies to scenarios and other packages, not to these now-stale ProcessQueue tests in the same file.
- `pkg/factory/factory.go:455` — stale comment "`ProcessQueue never reads from it in one-shot mode`" referencing the removed method. Update or delete the comment.
- Any counterfeiter-generated mock methods for `ProcessQueue` in `mocks/` — regenerated by `make generate`.

Regenerate counterfeiter mocks: `cd /workspace && make generate`.

## 6. Replace `pkg/runner/oneshot.go::Run`'s manual loop

Today `Run` has a `for { ... }` loop that calls `ProcessQueue` and breaks on `len(queued) == 0 && generated == 0`. **Delete the entire loop body.** Replace with a single line:

```go
return r.processor.Process(ctx)
```

The processor's own loop now drives execution. The factory-injected `onIdle` callback handles the exit (via `cancel()` for one-shot).

If `oneShotRunner` has setup steps before the loop (logging, bookkeeping), keep those — only the loop itself is removed.

## 7. Factory: wire two callbacks

In `pkg/factory/factory.go::CreateProcessor` (the single `NewProcessor` call site), the callback selection depends on the run mode (daemon vs one-shot). Two patterns are acceptable; pick the one that matches existing factory style:

**Pattern A (mode parameter):**

```go
func CreateProcessor(cfg config.Config, mode runMode) processor.Processor {
    onIdle := selectOnIdleCallback(mode)
    return processor.NewProcessor(
        ..., // existing args
        cfg.ParsedQueueInterval(),
        cfg.ParsedSweepInterval(),
        onIdle,
    )
}

func selectOnIdleCallback(mode runMode) processor.NothingToDoCallback {
    switch mode {
    case modeDaemon:
        return func(_ context.Context, _ context.CancelFunc) {
            slog.Info("nothing to do, waiting for changes")
        }
    case modeOneShot:
        return func(_ context.Context, cancel context.CancelFunc) {
            slog.Info("queue idle, exiting one-shot mode")
            cancel()
        }
    }
    panic("unknown mode")
}
```

The shared `selectOnIdleCallback` keeps the callback logic in one place. If the existing factory has a different idiom for run-mode dispatch, follow that — but do not duplicate the `NewProcessor` call to two near-identical factory functions.

## 8. Tests

### 8a. Processor unit tests (`pkg/processor/processor_test.go`)

Add Ginkgo `It` blocks (do NOT modify existing tests):

1. **`onIdle` fires when a queue tick has no progress**
   - Setup: empty queue, no approved specs, queueInterval = 20ms, sweepInterval = 1h
   - Pass a callback that records invocations
   - Run `Process` with a context that auto-cancels after 100ms
   - Assert callback invoked at least once

2. **`onIdle` does NOT fire when a tick makes progress**
   - Setup: one approved prompt that completes successfully (mock the executor)
   - Run `Process`
   - Assert callback NOT invoked during the tick that completed the prompt
   - Assert callback IS invoked on the next idle tick

3. **Cancel via callback exits Process cleanly**
   - Pass a callback that calls `cancel()` immediately on first invocation
   - Run `Process` with empty queue
   - Assert `Process` returns nil within `2*queueInterval`
   - Assert callback invoked exactly once

4. **`onIdle` fires after sweep tick when no specs transition**
   - Setup: empty queue, one prompted spec with all linked prompts already completed
   - Run `Process`; assert sweep transitions the spec on the first sweep tick AND callback does NOT fire that tick
   - Assert callback fires on the SECOND sweep tick (now idle)

5. **`onIdle` fires when a `<-p.ready` watcher tick has no progress**
   - Setup: empty queue
   - Send a value on `p.ready` (the test should be able to write to the channel via test helper or the processor's exposed channel)
   - Assert the resulting tick fires `onIdle` since no prompts were processed

### 8b. Runner / one-shot tests

Update or create `pkg/runner/oneshot_test.go`:

1. **Empty queue → exits within 1 second**
2. **Blocked queue (gap in numbering, e.g. prompt 002 with no 001) → exits within 1 second** (regression test for the pre-existing infinite-loop bug)
3. **Active queue with one runnable prompt → drains it then exits**

Use a mock executor; do not actually launch Docker.

### 8c. Existing tests must pass unchanged

`scenarios/006-spec-lifecycle.md` and `scenarios/011-reject-spec-cascade.md` are `active`. Both must continue to pass against the binary built from this change. Do not modify the scenario files.

## 9. CHANGELOG entry

Append to `## Unreleased` in `CHANGELOG.md`:

```
- refactor: unify daemon and one-shot processor loops via a NothingToDoCallback constructor parameter; one-shot's manual loop in pkg/runner/oneshot.go is replaced by processor.Process with a cancel-on-idle callback (eliminates the pre-existing infinite-loop on blocked-by-sequencing queues); daemon mode logs "nothing to do, waiting for changes" on idle ticks; processor.ProcessQueue is removed
```

## 10. Verification

```bash
cd /workspace && make precommit
```

Must exit 0.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- 336-configure-daemon-intervals already shipped — `queueInterval`, `sweepInterval`, struct-field-injected. Do not duplicate that work
- `Process` is the SINGLE processor entrypoint after this lands; `ProcessQueue` is deleted from both the `Processor` interface and the implementation
- Daemon behaviour MUST remain backwards-compatible: existing scenarios (006, 011) continue to pass without modification
- One-shot binary `dark-factory run` MUST exit 0 within ~queueInterval when the queue is empty or stuck (no infinite loop)
- The `onIdle` callback may fire repeatedly in daemon mode — this is expected (every idle tick logs)
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new error construction
- External test packages (`package processor_test`, `package runner_test`)
- Coverage ≥80% for changed packages
- `onIdle` is a constructor parameter, not a package-level mutable var with a test setter. See `go-composition.md` for the constructor-injection patterns this codebase uses
- The funlen budget on `Process` is tight — extract per-tick handling into helper methods if needed
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
cd /workspace

# ProcessQueue is gone from interface AND implementation
! grep -rn "ProcessQueue" pkg/ main.go

# NothingToDoCallback exists
grep -n "NothingToDoCallback" pkg/processor/processor.go

# Process still exists
grep -n "^func (p \*processor) Process" pkg/processor/processor.go

# One-shot has no inline loop
! grep -n "for {" pkg/runner/oneshot.go

# Smoke test for one-shot exit on empty queue is best done OUTSIDE the
# container (sandbox path is a host artifact, not mounted). It belongs in
# the operator's manual verification — see scenario 006 / 011 for the
# scenario-level coverage. The unit-test side of this is covered by 8b above.
```
</verification>
