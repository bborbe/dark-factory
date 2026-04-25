---
status: idea
created: "2026-04-25T13:55:00Z"
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

Add validation entries that mirror the `MaxPromptDuration` / `PreflightInterval` pattern:

- Parse as `time.Duration` via `time.ParseDuration`
- On parse error: return `errors.Errorf(ctx, "queueInterval must be a valid Go duration string, got %q", c.QueueInterval)` (and analogous for sweepInterval)
- On non-positive parsed duration: return `errors.Errorf(ctx, "queueInterval must be positive, got %s", c.QueueInterval)`

Add `validateQueueInterval` and `validateSweepInterval` methods alongside `validateMaxPromptDuration` if that's the existing structural pattern.

## 2. Wire intervals through to the processor

In `pkg/processor/processor.go`:

### 2a. Constructor

`NewProcessor(...)` currently takes `maxPromptDuration time.Duration`. Add two new positional parameters: `queueInterval time.Duration` and `sweepInterval time.Duration`. Place them adjacent to `maxPromptDuration` for grouping.

Store both on the `processor` struct (alongside `maxPromptDuration`):

```go
queueInterval  time.Duration
sweepInterval  time.Duration
```

### 2b. Use the fields in `Process()`

Replace the hard-coded literals:

```go
ticker := time.NewTicker(5 * time.Second)
// ...
sweepTicker := time.NewTicker(sweepInterval)
```

with:

```go
ticker := time.NewTicker(p.queueInterval)
// ...
sweepTicker := time.NewTicker(p.sweepInterval)
```

### 2c. Remove the package-level `var sweepInterval` and `SetSweepInterval` test helper

Delete from `processor.go`:

```go
var sweepInterval = 60 * time.Second
```

Delete from `pkg/processor/export_test.go`:

```go
func SetSweepInterval(d time.Duration) (restore func()) { ... }
```

Update any test that called `SetSweepInterval` to instead pass a small interval (e.g. `20 * time.Millisecond`) directly to `NewProcessor` via the new constructor parameter.

## 3. Wire from config to constructor

Find the call site that constructs the processor (likely in `pkg/factory/factory.go`). Currently it passes `maxPromptDuration` parsed from `cfg.MaxPromptDuration`. Add two more parsed durations:

```go
queueInterval, err := time.ParseDuration(cfg.QueueInterval)
if err != nil {
    return nil, errors.Wrapf(ctx, err, "parse queueInterval %q", cfg.QueueInterval)
}
sweepInterval, err := time.ParseDuration(cfg.SweepInterval)
if err != nil {
    return nil, errors.Wrapf(ctx, err, "parse sweepInterval %q", cfg.SweepInterval)
}
```

Pass both into `NewProcessor(...)`.

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

Append to `## Unreleased` in `CHANGELOG.md`:

```
- feat: queueInterval and sweepInterval are now configurable in .dark-factory.yaml (defaults 5s / 60s — no behavior change unless overridden); rejects invalid duration strings at startup
```

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

# No more package-level sweepInterval var
! grep -n "^var sweepInterval" pkg/processor/processor.go

# Processor uses the struct fields, not literals
grep -n "p.queueInterval\|p.sweepInterval" pkg/processor/processor.go

# Doc updated
grep -n "queueInterval\|sweepInterval" docs/configuration.md
```
</verification>
