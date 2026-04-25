---
status: idea
created: "2026-04-25T15:15:00Z"
---

<summary>
- Delete `processor.ProcessQueue` (the one-shot variant) and the duplicate loop in `pkg/runner/oneshot.go`; keep only `processor.Process`
- Add a `NothingToDoCallback` constructor parameter to `processor` — fired by `Process` whenever a cycle ends with no progress (no specs generated, no prompts moved to completed)
- The daemon entrypoint passes a log-only callback (`slog.Info("nothing to do, waiting for changes")`)
- The one-shot entrypoint (`dark-factory run`) passes a callback that calls `cancel()` on the processor's context — daemon shuts down cleanly, run exits 0
- Eliminates the pre-existing `dark-factory run` infinite loop on blocked-by-sequencing prompts: when no progress is made, the one-shot callback cancels the context and the binary exits with a "queue idle, exiting" log line
- Single processor implementation; behaviour difference between daemon and one-shot is one callback line
</summary>

<objective>
Unify the daemon and one-shot processor code paths. Today there are two near-duplicate flows (`Process` + ticker loop for daemon, `ProcessQueue` + manual `oneshot.Run` loop for one-shot) that have already drifted (the one-shot loop spins CPU on blocked queues; the daemon does not). Replace both with a single `Process(ctx, onIdle)` method whose only difference between modes is a constructor-injected callback.
</objective>

<context>
**Prerequisite:** This prompt depends on `336-configure-daemon-intervals.md` (or its successor) having shipped — `NewProcessor` is being modified to take `queueInterval` and `sweepInterval` constructor parameters. The `onIdle` callback is added alongside those.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` from coding plugin docs (constructor injection + small interface composition).
Read `go-testing-guide.md` from coding plugin docs.

Read these files before editing:
- `pkg/processor/processor.go` — `Process()` (line ~179), `ProcessQueue()` (line ~243), the 5s+60s tickers added by spec 058 prompt 334. After 336 ships, `NewProcessor` accepts `queueInterval` and `sweepInterval`.
- `pkg/runner/oneshot.go::Run` (line ~100+) — the duplicate loop that calls `ProcessQueue` repeatedly. The bug: blocked-by-sequencing prompts cause an infinite tight loop (~50 iterations/sec).
- `pkg/runner/daemon.go` (or wherever `dark-factory daemon` is wired) — the entrypoint that invokes `Process`. After this prompt, it passes a log-only `onIdle` callback.
- `pkg/factory/factory.go` — `NewProcessor` call site; the new callback is wired here per workflow mode.

Authoritative intent (from project owner):

> "Use the normal daemon and add a function callback that is called nothingToDo. The daemon only prints a log message in this loop, and the run version triggers the context cancel shut on the dark-factory thing."

The new callback type:

```go
// NothingToDoCallback fires when a Process cycle ends with no progress made.
// Daemon mode passes a log-only callback. One-shot mode passes one that calls cancel().
type NothingToDoCallback func(ctx context.Context, cancel context.CancelFunc)
```

"No progress" definition: at the end of a tick (queue tick OR sweep tick), if zero specs generated prompts AND zero prompts moved to `prompts/completed/` AND zero specs auto-transitioned to `verifying`, the cycle is idle.
</context>

<requirements>

## 1. Add `NothingToDoCallback` and constructor parameter

In `pkg/processor/processor.go`, after the existing types:

```go
// NothingToDoCallback fires when a Process cycle ends with no progress made.
type NothingToDoCallback func(ctx context.Context, cancel context.CancelFunc)
```

Add `onIdle NothingToDoCallback` to the `processor` struct. Add it to `NewProcessor` parameters (placed after the interval parameters added by 336). Document the parameter in the constructor comment.

If the callback is nil, treat it as a no-op (defensive default — but the factory should always pass one).

## 2. Track per-cycle progress in `Process`

Change `Process` to count progress per cycle and invoke `onIdle` when a cycle is idle.

Define a small struct used internally:

```go
type cycleResult struct {
    generatedSpecs    int
    completedPrompts  int
    transitionedSpecs int
}

func (c cycleResult) madeProgress() bool {
    return c.generatedSpecs > 0 || c.completedPrompts > 0 || c.transitionedSpecs > 0
}
```

Update the existing `processExistingQueued`, `generateFromApprovedSpecs` (if it lives in processor — otherwise expose enough information), and `checkPromptedSpecs` to return their respective counts. Aggregate them per tick.

In the `select` block of `Process`, after each `ticker.C` and `sweepTicker.C` case, after the existing work runs, build a `cycleResult` and:

```go
if !cycle.madeProgress() {
    if p.onIdle != nil {
        p.onIdle(ctx, cancel)
    }
}
```

`cancel` is captured at the top of `Process` via `ctx, cancel := context.WithCancel(ctx); defer cancel()`.

## 3. Wire the cancel context

At the top of `Process`:

```go
ctx, cancel := context.WithCancel(ctx)
defer cancel()
```

This is what the one-shot callback's `cancel()` will trigger. The existing `<-ctx.Done()` case in the `select` block already handles graceful shutdown — no changes needed there beyond passing the cancellable context.

## 4. Delete `ProcessQueue`

`processor.ProcessQueue` and any test fakes for it can be removed. Search the codebase for callers:

```bash
grep -rn "ProcessQueue" pkg/ main.go
```

Update each caller (most importantly `pkg/runner/oneshot.go::Run`) to instead call `Process(ctx, oneShotOnIdle)`.

## 5. Replace `pkg/runner/oneshot.go::Run`'s manual loop

Today it has a `for { ... }` loop calling `ProcessQueue` and breaking on `len(queued) == 0 && generated == 0`. Delete the loop entirely. Replace with:

```go
func (r *oneShotRunner) Run(ctx context.Context) error {
    onIdle := func(ctx context.Context, cancel context.CancelFunc) {
        slog.Info("queue idle, exiting one-shot mode")
        cancel()
    }
    return r.processor.Process(ctx, onIdle)
}
```

The processor now drives the loop. When idle, the callback cancels the context and `Process` returns nil via `<-ctx.Done()`.

## 6. Wire daemon entrypoint with log-only callback

In `pkg/runner/daemon.go` (or wherever `Process` is called for daemon mode), pass a log-only callback:

```go
onIdle := func(_ context.Context, _ context.CancelFunc) {
    slog.Info("nothing to do, waiting for changes")
}
return r.processor.Process(ctx, onIdle)
```

Note: the daemon's existing `slog.Info("waiting for changes")` (line ~182 of processor.go) can be removed since the callback now logs that on every idle cycle. Or keep it as the startup log and let the callback fire on subsequent idles — pick whichever reads cleanest.

## 7. Factory wiring

In `pkg/factory/factory.go`, update the `NewProcessor` call site to pass the appropriate callback. If the factory has separate code paths for daemon vs one-shot (likely yes), wire each:

- Daemon path → log-only callback
- One-shot path → cancel callback

If both paths share a single `NewProcessor` call, add a parameter to the factory function (e.g., `mode runMode`) and switch on it.

## 8. Tests

### 8a. Processor unit tests

Add Ginkgo `It` blocks in the existing `processor_test.go`:

1. **`onIdle` fires when a cycle has no progress**
   - Setup: empty queue, no approved specs
   - Inject a callback that records invocations + uses a small `queueInterval` (e.g. 20ms)
   - Run `Process` with a context that auto-cancels after 100ms
   - Assert callback invoked at least once

2. **`onIdle` does NOT fire when progress is made**
   - Setup: one approved prompt that completes successfully (mock the executor)
   - Run `Process`
   - Assert callback NOT invoked during the cycle that completed the prompt
   - Assert callback IS invoked on the next idle cycle

3. **Cancel via callback exits Process cleanly**
   - Inject a callback that calls `cancel()` immediately on first invocation
   - Run `Process` with empty queue
   - Assert `Process` returns nil within ~queueInterval + small slack
   - Assert callback invoked exactly once

### 8b. One-shot integration test (or scenario update)

Update `pkg/runner/oneshot_test.go` (or create one) to assert:

1. **Empty queue + no specs** → `Run` returns nil within 1 second
2. **Blocked queue (gap in numbering)** → `Run` returns nil within 1 second with a log line indicating idle exit (regression test for the infinite-loop bug)
3. **Active queue (one runnable prompt)** → `Run` drains and exits

Use a fake processor or a real one with a mock executor.

### 8c. Update scenario 006

`scenarios/006-spec-lifecycle.md` already exercises the daemon loop end-to-end. After this prompt, it should still pass — confirm the auto-transition happens via the same callback path.

## 9. CHANGELOG entry

Append to `## Unreleased` in `CHANGELOG.md`:

```
- refactor: unify daemon and one-shot processor loops via a NothingToDoCallback constructor parameter; one-shot's manual loop in pkg/runner/oneshot.go is replaced by processor.Process with a cancel-on-idle callback (eliminates the pre-existing infinite-loop on blocked-by-sequencing queues); daemon mode logs "nothing to do, waiting for changes" on idle cycles; ProcessQueue is removed
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
- This prompt depends on the interval-injection prompt (336 or successor) having shipped
- `Process` is the SINGLE processor entrypoint after this lands; `ProcessQueue` is deleted
- Daemon behaviour MUST remain backwards-compatible: existing scenarios (001, 010, 011, 006) continue to pass without modification
- One-shot binary `dark-factory run` MUST exit 0 when the queue is empty or stuck (no infinite loop)
- The `onIdle` callback must be idempotent — daemons fire it on every idle cycle
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new error construction
- External test packages (`package processor_test`, `package runner_test`)
- Coverage ≥80% for changed packages
- See `go-composition.md` "Anti-Pattern: Test-Only Package-Level Mutable State" — the new `onIdle` callback is a constructor parameter, not a package var with a test setter
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
cd /workspace

# ProcessQueue is gone
! grep -rn "ProcessQueue" pkg/ main.go

# NothingToDoCallback exists
grep -n "NothingToDoCallback" pkg/processor/processor.go

# Process accepts the callback
grep -n "func (p \*processor) Process" pkg/processor/processor.go

# One-shot uses Process now (not its own loop)
! grep -n "for {" pkg/runner/oneshot.go

# Smoke: empty queue exits one-shot in <2s
TMPDIR=$(mktemp -d) && cp -r ~/Documents/workspaces/dark-factory-sandbox "$TMPDIR/sandbox" && cd "$TMPDIR/sandbox" && \
  printf 'workflow: direct\npreflightCommand: ""\n' > .dark-factory.yaml && \
  git init --bare "$TMPDIR/remote.git" >/dev/null && git remote set-url origin "$TMPDIR/remote.git" && \
  timeout 5s /tmp/new-dark-factory run > /tmp/oneshot.log 2>&1 && \
  echo "one-shot exited 0 within 5s"
rm -rf "$TMPDIR"
```
</verification>
