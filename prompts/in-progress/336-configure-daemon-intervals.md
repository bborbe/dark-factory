---
status: executing
container: dark-factory-336-configure-daemon-intervals
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T13:55:00Z"
queued: "2026-04-25T13:05:02Z"
started: "2026-04-25T13:06:20Z"
---

<summary>
- Two daemon timing intervals are currently not user-tunable: the 5-second queue ticker in `pkg/processor/processor.go::Process()` and the 60-second auto-complete sweep ticker added by `fix-stuck-prompted-specs.md`
- Add `queueInterval` and `sweepInterval` fields to `.dark-factory.yaml` (Go duration strings, e.g. `"5s"`, `"60s"`) with defaults matching today's behavior
- Defaults preserve current behavior — projects that don't set these fields see no change
- Validation rejects invalid duration strings (parse error) and non-positive values at daemon startup, matching the existing pattern used by `preflightInterval` and `maxPromptDuration`
- Update `docs/configuration.md` to document both new fields
- The package-level `var sweepInterval` from `fix-stuck-prompted-specs.md` is replaced by a struct field on `processor` populated from config; the test override pattern (`SetSweepInterval`) is removed in favor of constructor injection
</summary>

<objective>
Make the daemon's queue-poll interval and auto-complete sweep interval configurable via `.dark-factory.yaml`, with sensible defaults and validation consistent with existing duration fields.
</objective>

<context>
**Prerequisite:** This prompt depends on `fix-stuck-prompted-specs.md` having been applied first (introduces the sweep ticker and `var sweepInterval`).

Read `CLAUDE.md` for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`.
Read `go-validation-framework-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Read these files before editing:
- `pkg/config/config.go` — config struct, defaults block (line ~151), validation block (line ~179, ~239). Look at `MaxPromptDuration` (line 121) and `PreflightInterval` (line 124) as the exact pattern to mirror.
- `pkg/processor/processor.go` — `Process()` ticker and sweepTicker construction (line 185 area), constructor `NewProcessor(...)` (line ~75).
- `docs/configuration.md` — Preflight Baseline Check section is the closest sibling for the new doc entries.

Existing conventions for duration config fields (mirror these):

```go
// In config.go struct:
MaxPromptDuration string `yaml:"maxPromptDuration"`
PreflightInterval string `yaml:"preflightInterval"`

// In defaults:
PreflightInterval: "8h",

// In validation: parsed as time.Duration, rejected if invalid or non-positive
```
</context>

<requirements>

## 1. Add config fields

In `pkg/config/config.go`, add two new fields next to `PreflightInterval`:

```go
QueueInterval string `yaml:"queueInterval"`
SweepInterval string `yaml:"sweepInterval"`
```

Add defaults in the same block as `PreflightInterval: "8h"`:

```go
QueueInterval:     "5s",
SweepInterval:     "60s",
```

Add `validateQueueInterval` and `validateSweepInterval` methods alongside `validateMaxPromptDuration` (config.go:287) and `validatePreflightInterval` (config.go:303). Mirror those exactly:

- Empty string → `return nil` (treats empty as "use default" — matches existing pattern)
- Parse via `time.ParseDuration`; on error → `errors.Errorf(ctx, "queueInterval %q is not a valid duration: %v", c.QueueInterval, err)` (analogous for sweep)
- ADDITIONAL check (not in existing siblings): non-positive parsed duration → `errors.Errorf(ctx, "queueInterval must be positive, got %s", c.QueueInterval)`. Reason: `time.NewTicker(d)` panics on `d <= 0`, so unlike `maxPromptDuration` (where 0 disables timeout), here zero/negative is invalid.

Register both new validators in the `validation.All{}` slice in `Validate(ctx)` (config.go:171–247) — without registration, the methods are dead code. Add right after the `preflightInterval` entry.

Add `ParsedQueueInterval()` and `ParsedSweepInterval()` methods on `Config` mirroring `ParsedMaxPromptDuration()` (config.go:267) and `ParsedPreflightInterval()` (config.go:253):

```go
// ParsedQueueInterval returns the parsed duration from QueueInterval.
// Returns 5 * time.Second when QueueInterval is empty or unparseable (preserves default behaviour).
// Safe to call at any time — never panics.
func (c Config) ParsedQueueInterval() time.Duration {
    if c.QueueInterval == "" {
        return 5 * time.Second
    }
    d, err := time.ParseDuration(c.QueueInterval)
    if err != nil {
        return 5 * time.Second
    }
    return d
}
```

Analogous for `ParsedSweepInterval()` with default `60 * time.Second`. (Defaults preserve current behavior even if validation is bypassed somehow — defense in depth.)

## 2. Wire intervals through to the processor

In `pkg/processor/processor.go`:

### 2a. Constructor

`NewProcessor(...)` currently takes `maxPromptDuration time.Duration` (line ~105). Add two new positional parameters: `queueInterval time.Duration` and `sweepInterval time.Duration`. Place them adjacent to `maxPromptDuration` for grouping — three identically-typed `time.Duration` parameters in a row is a known footgun, so update the GoDoc on `NewProcessor` to document the ordering explicitly.

Store both on the `processor` struct (alongside `maxPromptDuration`):

```go
queueInterval  time.Duration
sweepInterval  time.Duration
```

Update the struct initialization block in `NewProcessor` (line ~108) to populate both new fields.

### 2b. Use the fields in `Process()`

Replace the hard-coded literals at lines 199 and 205:

```go
// before (line ~199)
ticker := time.NewTicker(5 * time.Second)
// after
ticker := time.NewTicker(p.queueInterval)

// before (line ~205)
sweepTicker := time.NewTicker(getSweepInterval())
// after
sweepTicker := time.NewTicker(p.sweepInterval)
```

### 2c. Remove the entire package-var test-seam

Delete ALL of the following from `pkg/processor/processor.go` (lines 36–48):

```go
// sweepIntervalMu protects sweepInterval from concurrent read/write (test overrides).
var sweepIntervalMu sync.Mutex

// sweepInterval controls the auto-complete sweep cadence. Variable (not const)
// so tests can override via SetSweepInterval (export_test.go).
var sweepInterval = 60 * time.Second

// getSweepInterval returns the current sweep interval under the mutex.
func getSweepInterval() time.Duration {
    sweepIntervalMu.Lock()
    defer sweepIntervalMu.Unlock()
    return sweepInterval
}
```

Also remove the now-unused `"sync"` import from processor.go IF nothing else in the file still uses it (search before deleting).

Delete from `pkg/processor/export_test.go`:

```go
func SetSweepInterval(d time.Duration) (restore func()) { ... }
```

Update the single test call site at `pkg/processor/processor_test.go:7271` (only one call to `SetSweepInterval` exists) to instead pass `20 * time.Millisecond` directly to `NewProcessor` via the new `sweepInterval` constructor parameter.

## 3. Wire from config to constructor

Use the `Parsed*()` method convention — DO NOT call `time.ParseDuration` inline in factory.go. The factory currently calls `cfg.ParsedMaxPromptDuration()` at 5 call sites (`pkg/factory/factory.go:353, 374, 485, 516, 530`). Mirror that pattern:

```go
processor.NewProcessor(
    // ... existing args ...
    cfg.ParsedQueueInterval(),
    cfg.ParsedSweepInterval(),
    // ... rest ...
)
```

Update all 5 `processor.NewProcessor(...)` call sites in factory.go to pass the two new parsed values adjacent to `cfg.ParsedMaxPromptDuration()`.

Validation already rejects bad values at startup via `Validate(ctx)`, so `Parsed*()` returning a default on parse error is safe — the daemon never starts with a broken config.

## 4. Update `docs/configuration.md`

Add a new subsection under the existing daemon configuration section (near `Preflight Baseline Check`):

```markdown
### Queue and Sweep Intervals

```yaml
queueInterval: "5s"
sweepInterval: "60s"
```

| Field | Default | Purpose |
|-------|---------|---------|
| `queueInterval` | `5s` | How often the daemon polls for queued prompts and re-checks committing prompts. Lower values give faster response to fsnotify-missed events at the cost of more frequent file scans. |
| `sweepInterval` | `60s` | How often the daemon scans `specs/in-progress/` for prompted specs whose linked prompts have all completed and transitions them to `verifying`. Self-healing safety net for the per-prompt auto-complete path; lower values give faster recovery from missed transitions. |

Both accept Go duration strings (`"5s"`, `"60s"`, `"5m"`, `"1h"`). Invalid strings or non-positive durations are rejected at daemon startup.
```

Place this block immediately above or below the `Preflight Baseline Check` block — wherever it reads more naturally given the surrounding structure.

## 5. Update tests

### 5a. Config tests

Add cases in `pkg/config/config_test.go` (do NOT modify existing tests):
- Default `QueueInterval` is `"5s"`, default `SweepInterval` is `"60s"`
- Invalid duration string for either field → validation error
- Non-positive duration (`"0s"`, `"-1s"`) for either field → validation error

### 5b. Processor sweep test

The existing self-healing test added by `fix-stuck-prompted-specs.md` used `SetSweepInterval`. Update it to construct the processor with `sweepInterval = 20 * time.Millisecond` directly via `NewProcessor`. Remove all `SetSweepInterval` usage.

## 6. CHANGELOG entry

Skip — `CHANGELOG.md` uses dated version headers (`## v0.135.x`); the dark-factory release flow creates the next version section automatically. Do not add an `## Unreleased` block; it is not the project convention.

## 7. Run verification

```bash
cd /workspace && make precommit
```

Must exit 0.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Defaults MUST preserve today's behavior: `queueInterval: "5s"`, `sweepInterval: "60s"`. Projects that don't set these fields see exactly today's daemon timing
- Use Go duration strings (`time.ParseDuration` format) for both fields — match the convention set by `maxPromptDuration` and `preflightInterval`
- Validation must reject: parse error, zero duration, negative duration. Same defensive shape as existing duration fields
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new error construction
- External test packages where applicable
- Coverage ≥80% for changed packages
- The package-level `var sweepInterval` MUST be removed — leaving it dead would invite drift
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
cd /workspace

# Both fields exist in config
grep -n "QueueInterval\|SweepInterval" pkg/config/config.go

# Defaults are set
grep -n '"5s"\|"60s"' pkg/config/config.go

# Parsed*() methods exist
grep -n "ParsedQueueInterval\|ParsedSweepInterval" pkg/config/config.go

# Validators registered in validation.All
grep -n "queueInterval\|sweepInterval" pkg/config/config.go | grep -i "validation.Name"

# Package-level sweep state fully removed
! grep -n "^var sweepInterval\|^var sweepIntervalMu\|^func getSweepInterval" pkg/processor/processor.go

# SetSweepInterval helper removed
! grep -n "SetSweepInterval" pkg/processor/

# Processor uses the struct fields, not literals
grep -n "p.queueInterval\|p.sweepInterval" pkg/processor/processor.go

# Factory uses Parsed*() methods (not inline ParseDuration)
grep -n "ParsedQueueInterval\|ParsedSweepInterval" pkg/factory/factory.go

# Doc updated
grep -n "queueInterval\|sweepInterval" docs/configuration.md
```
</verification>
