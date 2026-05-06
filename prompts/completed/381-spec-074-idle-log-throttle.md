---
status: completed
spec: [074-bug-noisy-waiting-for-changes-log]
summary: 'Replaced unconditional idle log with a sampler-throttled closure: emits once on idle-window entry, then heartbeats at configurable idleLogInterval (default 1m); removed duplicate startup log from processor.go; added IdleLogInterval config field, loader support, validation, ParsedIdleLogInterval(); promoted github.com/bborbe/log to direct dependency; added buildIdleLogger factory function with unit tests; updated docs and CHANGELOG.'
container: dark-factory-381-spec-074-idle-log-throttle
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T09:15:00Z"
queued: "2026-05-06T09:26:47Z"
started: "2026-05-06T09:26:48Z"
completed: "2026-05-06T09:39:03Z"
branch: dark-factory/bug-noisy-waiting-for-changes-log
---

<summary>
- Dark-factory daemon currently emits `"nothing to do, waiting for changes"` on every idle tick (every 5s by default), producing ~720 lines/hour and ~17k lines/day in `.dark-factory.log` during idle periods
- After this fix, the idle line is emitted exactly once when the daemon first enters the idle state (or re-enters after processing a prompt)
- A configurable heartbeat interval (`idleLogInterval`, default `1m`) controls how often a follow-up idle line is emitted to confirm the daemon is alive
- Setting `idleLogInterval: 0` disables the heartbeat entirely — only the first-entry line fires per idle window
- A burst of 3-4 identical log lines at busy→idle transitions (from multiple wakeup paths firing simultaneously) is collapsed to 1 line by the sampler-throttled closure
- A duplicate `"waiting for changes"` emit that fires from `pkg/processor/processor.go` at startup is removed — idle logging is consolidated to a single source of truth in the factory's `onIdle` callback
- The one-shot `run` mode is unaffected — its `onIdle` callback still calls `cancel()` with no log emission
- `github.com/bborbe/log` is promoted from indirect to direct dependency, since the factory now directly imports `log.NewSampleTime`
- A `CHANGELOG.md` entry documents the change
</summary>

<objective>
Fix the noisy idle log in the dark-factory daemon. The `onIdle` callback in `pkg/factory/factory.go` currently calls `slog.Info("nothing to do, waiting for changes")` unconditionally on every idle tick, flooding `.dark-factory.log` with ~12 lines/minute during idle periods. Replace it with a sampler-throttled closure that emits immediately on the first idle tick per idle window and then only at the configured `idleLogInterval` heartbeat cadence. Remove the startup `slog.Info("waiting for changes")` from `pkg/processor/processor.go` so idle logging has exactly one source of truth.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors wrapping, Ginkgo/Gomega, no fmt.Errorf, no bare return err).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-logging-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read in full before editing:

- `pkg/config/config.go` — `Config` struct (~line 84), `Defaults()` (~line 124), `Validate()` (~line 164), and the existing `ParsedQueueInterval()` / `ParsedSweepInterval()` methods (~lines 336–362) as the template for the new `ParsedIdleLogInterval()` method
- `pkg/config/loader.go` — `partialConfig` struct (~line 53) and `mergePartial` (~line 142); use the `QueueInterval *string` / `SweepInterval *string` fields (lines 123–124) as the exact template for the new field
- `pkg/processor/processor.go` — line 191 (`slog.Info("waiting for changes")`) to remove; also read lines 56–113 to understand `NothingToDoCallback` and `NewProcessor` signature
- `pkg/factory/factory.go` — lines 283–422 covering `CreateRunner`, the `proc := CreateProcessor(...)` call, and the `onIdle` callback at lines 419–421 where the fix goes; also read the import block (lines 1–55) to confirm what is already imported
- `pkg/processor/processor_verification_test.go` — lines 490–545 covering the `newProc()` helper and the test "daemon Process logs 'waiting for changes' once after startup scan" (lines 524–545); this test must be updated after removing the processor startup log line
</context>

<requirements>

## 1. Add `IdleLogInterval` to `pkg/config/config.go`

### 1a. Add field to `Config` struct

Add `IdleLogInterval` immediately after `SweepInterval` in the `Config` struct (line ~122):

```go
QueueInterval          string              `yaml:"queueInterval"`
SweepInterval          string              `yaml:"sweepInterval"`
IdleLogInterval        string              `yaml:"idleLogInterval"`
```

### 1b. Add default to `Defaults()`

Add the following to `Defaults()`, immediately after the `SweepInterval` entry:

```go
QueueInterval:     "5s",
SweepInterval:     "60s",
IdleLogInterval:   "1m",
```

Default of `"1m"` produces at most 2 log lines in the first 90s of idle (`initial + 1 heartbeat`), satisfying the acceptance criterion.

### 1c. Add validation in `Validate()`

Add a `validateIdleLogInterval` call to `Validate()` alongside the existing duration validators:

```go
validation.Name("idleLogInterval", validation.HasValidationFunc(c.validateIdleLogInterval)),
```

Implement the validator method:

```go
func (c Config) validateIdleLogInterval(ctx context.Context) error {
    if c.IdleLogInterval == "" {
        return nil
    }
    d, err := time.ParseDuration(c.IdleLogInterval)
    if err != nil {
        return errors.Errorf(
            ctx,
            "idleLogInterval %q is not a valid duration: %v",
            c.IdleLogInterval,
            err,
        )
    }
    if d < 0 {
        return errors.Errorf(ctx, "idleLogInterval must not be negative, got %s", c.IdleLogInterval)
    }
    return nil
}
```

`d == 0` is valid and means "heartbeat disabled"; `d < 0` is rejected.

### 1d. Add `ParsedIdleLogInterval()` method

Add this method adjacent to `ParsedQueueInterval()` and `ParsedSweepInterval()`, following the exact same pattern:

```go
// ParsedIdleLogInterval returns the parsed duration from IdleLogInterval.
// Returns 0 when IdleLogInterval is empty, "0", or unparseable (heartbeat disabled).
// Safe to call at any time — never panics.
func (c Config) ParsedIdleLogInterval() time.Duration {
    if c.IdleLogInterval == "" {
        return time.Minute
    }
    d, err := time.ParseDuration(c.IdleLogInterval)
    if err != nil {
        return time.Minute
    }
    return d
}
```

Note: `"0"` parses to `0` (heartbeat disabled) — this is intentional. `""` (empty, same as unset) falls back to the 1-minute default.

## 2. Update `pkg/config/loader.go`

### 2a. Add to `partialConfig`

Add the field immediately after `SweepInterval` in `partialConfig`, mirroring the existing pattern:

```go
QueueInterval          *string              `yaml:"queueInterval"`
SweepInterval          *string              `yaml:"sweepInterval"`
IdleLogInterval        *string              `yaml:"idleLogInterval"`
```

### 2b. Add to `mergePartial`

Add the merge block immediately after the `SweepInterval` merge block, following the identical pattern:

```go
if partial.QueueInterval != nil {
    cfg.QueueInterval = *partial.QueueInterval
}
if partial.SweepInterval != nil {
    cfg.SweepInterval = *partial.SweepInterval
}
if partial.IdleLogInterval != nil {
    cfg.IdleLogInterval = *partial.IdleLogInterval
}
```

## 3. Remove the duplicate log from `pkg/processor/processor.go`

Delete line 191:

```go
slog.Info("waiting for changes")
```

This is the only change to `processor.go`. The idle log is now the sole responsibility of the factory's `onIdle` callback. Do not add any replacement log line.

## 4. Replace the `onIdle` callback in `pkg/factory/factory.go`

### 4a. Add import

Add `"sync"` and `liblog "github.com/bborbe/log"` to the import block in `factory.go`. The `liblog` alias avoids collision with `log/slog` which may already be imported as `slog`:

```go
import (
    // ... existing imports ...
    "sync"
    liblog "github.com/bborbe/log"
)
```

### 4b. Build the throttled `onIdle` closure

Replace lines 419–421 in `factory.go`:

```go
// BEFORE (remove this):
func(_ context.Context, _ context.CancelFunc) {
    slog.Info("nothing to do, waiting for changes")
},
```

With the following closure (built immediately before the `proc := CreateProcessor(...)` call, then passed as the last argument):

```go
idleLogInterval := cfg.ParsedIdleLogInterval()
queueInterval   := cfg.ParsedQueueInterval()

var (
    idleMu       sync.Mutex
    lastIdleCall time.Time
    heartbeat    liblog.Sampler
)
if idleLogInterval > 0 {
    heartbeat = liblog.NewSampleTime(idleLogInterval)
}

onIdle := func(_ context.Context, _ context.CancelFunc) {
    idleMu.Lock()
    defer idleMu.Unlock()

    now := time.Now()
    // If more than 2× queueInterval has elapsed since the last idle tick,
    // at least one busy tick fired in between — treat this as a fresh idle entry.
    isNewIdleEntry := lastIdleCall.IsZero() || now.Sub(lastIdleCall) > 2*queueInterval
    lastIdleCall = now

    if isNewIdleEntry {
        slog.Info("nothing to do, waiting for changes")
        // Reset the heartbeat sampler so its interval starts from this moment,
        // not from whenever the sampler was first created.
        if idleLogInterval > 0 {
            heartbeat = liblog.NewSampleTime(idleLogInterval)
        }
        return
    }

    // Continuing idle window — only emit heartbeat if the sampler fires.
    if heartbeat != nil && heartbeat.IsSample() {
        slog.Info("nothing to do, waiting for changes", "heartbeat", true)
    }
}

proc := CreateProcessor(
    // ... all existing arguments unchanged ...
    onIdle,
)
```

**Why this works for the burst case:** When multiple wakeup paths (watcher, queueScanner, specWatcher) each invoke `onIdle` within milliseconds of each other, the first call acquires `idleMu`, sets `lastIdleCall`, emits, and resets `heartbeat`. Subsequent calls within the same millisecond see `lastIdleCall` is only microseconds old (far less than `2*queueInterval`), so `isNewIdleEntry = false`. `heartbeat.IsSample()` also returns `false` since we just reset it. Net result: exactly 1 log line per burst.

**Why `idleLogInterval: 0` works:** When the user sets `idleLogInterval: 0`, `ParsedIdleLogInterval()` returns `0`, `heartbeat` stays nil, and `heartbeat != nil && heartbeat.IsSample()` is always false. Only the first-entry line fires per idle window.

**Thread safety:** `idleMu` protects both `lastIdleCall` and `heartbeat` (which is reassigned inside the lock on reset). `liblog.NewSampleTime` is already thread-safe internally; the outer lock only guards the reassignment.

## 5. Update `pkg/processor/processor_verification_test.go`

The test "daemon Process logs 'waiting for changes' once after startup scan" (lines 524–545) asserts count = 1 for `"waiting for changes"`. This count came from `slog.Info("waiting for changes")` at processor.go:191, which we removed in requirement 3.

Update the test as follows:

- Change the test description from `"daemon Process logs 'waiting for changes' once after startup scan"` to `"daemon Process does not log 'waiting for changes' at startup (removed in favour of onIdle callback)"`
- Remove or update the `ContainSubstring` assertion to confirm the processor does NOT emit this message:

```go
Expect(logBuf.String()).NotTo(ContainSubstring("waiting for changes"))
```

- Remove the `strings.Count` assertion (it no longer applies).

The processor correctly starts up with no idle-state log; the factory's daemon-mode `onIdle` callback handles it.

## 6. Promote `github.com/bborbe/log` to direct dependency

After adding the import in requirement 4a, run:

```bash
cd /workspace && go mod tidy
```

This promotes `github.com/bborbe/log` from `// indirect` to a direct entry in `go.mod` (since the package is now directly imported by factory.go). `go.sum` already contains the checksum for v1.6.8 so no network fetch is required.

Do NOT manually edit `go.mod` or `go.sum`.

## 7. Add boundary contract test for the throttled `onIdle` closure

The closure introduces ~40 lines of new concurrent logic (mutex, `lastIdleCall` heuristic, sampler reset). `liblog.Sampler` is a library boundary — without an integration test that exercises the closure, the runtime contract is unverified.

Extract the closure-building logic into a small testable factory function in `pkg/factory/factory.go`:

```go
// buildIdleLogger returns the onIdle callback used by daemon-mode.
// Exposed for testing the burst-collapse + heartbeat behavior.
// Behavior:
//   - First call (or first call > 2*queueInterval since the last) emits unconditionally
//   - Subsequent calls within the same idle window emit only when the heartbeat sampler fires
//   - idleLogInterval == 0 disables the heartbeat; only first-entry emissions fire
func buildIdleLogger(
    idleLogInterval time.Duration,
    queueInterval   time.Duration,
    emit func(),
) func(context.Context, context.CancelFunc) {
    var (
        mu           sync.Mutex
        lastIdleCall time.Time
        heartbeat    liblog.Sampler
    )
    if idleLogInterval > 0 {
        heartbeat = liblog.NewSampleTime(idleLogInterval)
    }
    return func(_ context.Context, _ context.CancelFunc) {
        mu.Lock()
        defer mu.Unlock()
        now := time.Now()
        isNewIdleEntry := lastIdleCall.IsZero() || now.Sub(lastIdleCall) > 2*queueInterval
        lastIdleCall = now
        if isNewIdleEntry {
            emit()
            if idleLogInterval > 0 {
                heartbeat = liblog.NewSampleTime(idleLogInterval)
            }
            return
        }
        if heartbeat != nil && heartbeat.IsSample() {
            emit()
        }
    }
}
```

Adjust requirement 4b to call `buildIdleLogger(idleLogInterval, queueInterval, func() { slog.Info("nothing to do, waiting for changes") })` instead of inlining the closure body. The runtime behavior is identical; the factored form is testable.

Add `pkg/factory/idle_logger_test.go` (Ginkgo + Gomega) covering:

- **Burst collapse**: 4 invocations within the same millisecond emit exactly 1 line — `emit` callback counter == 1
- **Heartbeat fires after interval**: 1 invocation at `t=0`, 1 at `t = idleLogInterval + 10ms` (simulate by sleeping or using a short interval like `50ms` in the test) — counter == 2
- **Heartbeat suppressed within interval**: 1 invocation at `t=0`, 1 at `t = idleLogInterval / 2` — counter == 1
- **`idleLogInterval == 0` disables heartbeat**: build with `idleLogInterval = 0`, invoke 5x within `queueInterval` — counter == 1
- **`idleLogInterval == 0` first-entry still fires**: build with `idleLogInterval = 0`, invoke once — counter == 1
- **`2*queueInterval` heuristic re-arms first-entry**: invoke at `t=0`, sleep `> 2*queueInterval`, invoke again — counter == 2 (treated as fresh idle entry)

Use a short `idleLogInterval` (e.g. `50ms`) and `queueInterval` (e.g. `10ms`) in tests so `time.Sleep` is bounded.

The `emit` callback parameter (rather than asserting on `slog` output) keeps the test focused on closure mechanics, not log plumbing.

## 8. Update documentation

`idleLogInterval` is a new YAML config field — document it everywhere existing duration intervals are documented:

- **`docs/configuration.md`** (or whichever file contains the `queueInterval` / `sweepInterval` table) — add `idleLogInterval` row with default `1m`, semantics ("0 disables heartbeat; first-entry log always fires"), and a one-line operator note
- **`README.md`** — if the README has a config example block listing `queueInterval`/`sweepInterval`, add `idleLogInterval` alongside
- **Example config files** — grep for `queueInterval:` in `examples/`, `testdata/`, etc. and add `idleLogInterval` if those files document the full schema

Run `grep -rn "queueInterval\|sweepInterval" docs/ README.md examples/ testdata/ 2>/dev/null` to discover all locations; update each one consistently.

## 9. Add `CHANGELOG.md` entry

Add a bullet under `## Unreleased` in `CHANGELOG.md` (create the section if absent):

```markdown
## Unreleased

- fix: Suppress noisy `"nothing to do, waiting for changes"` idle log — emits once per idle entry, then at most once per `idleLogInterval` (default 1m) heartbeat; configurable via `idleLogInterval:` in `.dark-factory.yaml`
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change `queueInterval` semantics — the scanner still polls at the same cadence; only the log emission is throttled
- Do NOT silence the idle log entirely — operators need to confirm the daemon is alive after a long idle window
- The idle log MUST be emitted from a single source: the factory's `onIdle` callback only; `pkg/processor/processor.go:191` must be removed
- The throttling mechanism MUST use `liblog.NewSampleTime` from `github.com/bborbe/log` — no inline ad-hoc counters or `time.Since` comparisons as the sole rate-limiter
- `idleLogInterval: 0` MUST disable the heartbeat — only the first-entry line fires per idle window
- The first emission after a busy→idle transition MUST be unconditional (not subject to the sampler)
- One-shot `run` mode: the `onIdle` callback that calls `cancel()` is unchanged; this fix only affects the daemon-mode callback in `CreateRunner`
- `idleLogInterval` default is `"1m"` in `Defaults()` — do NOT leave the default at `""`
- Wrap all non-nil errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`; never `fmt.Errorf`, never bare `return err`
- Do NOT manually edit `go.mod` or `go.sum` — run `go mod tidy` instead
- Existing tests must still pass (except the processor test updated in requirement 5)
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -c "waiting for changes" pkg/processor/processor.go` — must return 0 (line 191 removed)
2. `grep -n "IdleLogInterval" pkg/config/config.go` — three occurrences: struct field, Defaults() entry, validateIdleLogInterval body
3. `grep -n "IdleLogInterval" pkg/config/loader.go` — two occurrences: partialConfig field, mergePartial block
4. `grep -n "liblog\|bborbe/log" pkg/factory/factory.go` — import alias and `liblog.NewSampleTime` call
5. `grep -n "idleLogInterval\|IdleLogInterval\|heartbeat" pkg/factory/factory.go` — closure variables and sampler usage
6. `grep -n "bborbe/log" go.mod` — entry must NOT have `// indirect` suffix after `go mod tidy`
7. `grep -n "NotTo.*ContainSubstring.*waiting for changes" pkg/processor/processor_verification_test.go` — updated assertion present
8. `ls pkg/factory/idle_logger_test.go` — new test file exists
9. `grep -n "buildIdleLogger" pkg/factory/factory.go pkg/factory/idle_logger_test.go` — factored function present in both files
10. `grep -rn "idleLogInterval" docs/ README.md` — config field documented

Runtime spot check (manual, do not run in container — for reference only):
```bash
# Empty queue, fresh log
rm -f .dark-factory.log
dark-factory daemon &
DAEMON_PID=$!
sleep 90
grep -c "waiting for changes" .dark-factory.log
# Expected: ≤ 2 (initial + 1 heartbeat after 1 min)
kill $DAEMON_PID
```
</verification>
