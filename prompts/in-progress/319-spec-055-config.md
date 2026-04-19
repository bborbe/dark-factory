---
status: committing
spec: [055-preflight-baseline-check]
summary: Added PreflightCommand and PreflightInterval config fields with defaults, validation, ParsedPreflightInterval helper, LogEffectiveConfig logging, docs/configuration.md section, and tests
container: dark-factory-319-spec-055-config
dark-factory-version: v0.125.1
created: "2026-04-19T12:00:00Z"
queued: "2026-04-19T16:15:50Z"
started: "2026-04-19T16:15:53Z"
branch: dark-factory/preflight-baseline-check
---

<summary>
- Two new optional config fields appear in `.dark-factory.yaml`: `preflightCommand` and `preflightInterval`
- `preflightCommand` defaults to `make precommit`; empty string disables the preflight baseline check entirely
- `preflightInterval` defaults to `8h`; controls how long a cached green baseline result stays valid for the same git commit SHA
- Existing configs without these fields load unchanged and pick up the defaults automatically
- An invalid `preflightInterval` duration string (e.g. `"2x"`) is rejected at daemon startup with a clear error — same pattern as `maxPromptDuration`
- A helper method `ParsedPreflightInterval()` returns the parsed `time.Duration` (0 when empty or disabled)
- `docs/configuration.md` documents the new fields with a dedicated "Preflight Baseline Check" subsection
- `make precommit` passes
</summary>

<objective>
Add `preflightCommand` and `preflightInterval` to the dark-factory config model so the daemon can be configured to run a baseline validation command before each prompt. This prompt covers only the data model, defaults, validation, and documentation — no runtime logic.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before editing:
- `pkg/config/config.go` — `Config` struct (line ~82), `Defaults()` (line ~124), `Validate()` (line ~162), `validateMaxPromptDuration` (line ~261), `ParsedMaxPromptDuration()` (line ~238)
- `pkg/config/config_test.go` — test patterns for defaults, validation
- `pkg/factory/factory.go` — `LogEffectiveConfig` function (line ~65) — add new fields here too
- `docs/configuration.md` — existing sections on `maxPromptDuration` and `autoRetryLimit` (add new section after them)
</context>

<requirements>

## 1. Add new fields to the `Config` struct

In `pkg/config/config.go`, add after the `AutoRetryLimit` field (currently the last field, around line 120):

```go
PreflightCommand  string `yaml:"preflightCommand"`
PreflightInterval string `yaml:"preflightInterval"`
```

## 2. Add defaults in `Defaults()`

In the `Defaults()` function (around line 124), add after the existing entries:

```go
PreflightCommand:  "make precommit",
PreflightInterval: "8h",
```

## 3. Add `validatePreflightInterval` method

Model this exactly after `validateMaxPromptDuration` (around line 261):

```go
// validatePreflightInterval rejects unparseable duration strings for preflightInterval.
func (c Config) validatePreflightInterval(ctx context.Context) error {
	if c.PreflightInterval == "" {
		return nil
	}
	if _, err := time.ParseDuration(c.PreflightInterval); err != nil {
		return errors.Errorf(
			ctx,
			"preflightInterval %q is not a valid duration: %v",
			c.PreflightInterval,
			err,
		)
	}
	return nil
}
```

## 4. Register the validator in `Validate()`

In the `Validate()` method `validation.All{...}` block (around line 162), add after the `autoRetryLimit` entry:

```go
validation.Name(
    "preflightInterval",
    validation.HasValidationFunc(c.validatePreflightInterval),
),
```

## 5. Add `ParsedPreflightInterval()` helper

Model this exactly after `ParsedMaxPromptDuration()` (around line 238):

```go
// ParsedPreflightInterval returns the parsed duration from PreflightInterval.
// Returns 0 when PreflightInterval is empty (disables interval-based caching).
// Safe to call at any time — returns 0 on error, never panics.
func (c Config) ParsedPreflightInterval() time.Duration {
	if c.PreflightInterval == "" {
		return 0
	}
	d, err := time.ParseDuration(c.PreflightInterval)
	if err != nil {
		return 0
	}
	return d
}
```

## 6. Update `LogEffectiveConfig` in `pkg/factory/factory.go`

In the `slog.Info("effective config", ...)` call (around line 77), add the new fields after the existing ones:

```go
"preflightCommand", cfg.PreflightCommand,
"preflightInterval", cfg.PreflightInterval,
```

## 7. Update `docs/configuration.md`

Add a new subsection **"Preflight Baseline Check"** after the existing "Prompt Timeout" (`maxPromptDuration`) subsection. Insert the following markdown:

```markdown
### Preflight Baseline Check

Run the project's baseline validation command on a clean tree before each prompt executes.
Prompts only start on a known-green baseline. When the baseline is broken, queued prompts
remain queued (no retry count increment, no status change), and the operator is notified.

```yaml
preflightCommand: "make precommit"
preflightInterval: "8h"
```

| Field | Default | Purpose |
|-------|---------|---------|
| `preflightCommand` | `make precommit` | Shell command run inside the container before each prompt. Empty string disables preflight entirely. |
| `preflightInterval` | `8h` | How long a cached green baseline result stays valid for the same git commit SHA. When the SHA advances or the interval elapses, preflight re-runs. Accepts Go duration strings: `"30m"`, `"2h"`, `"8h"`. Invalid strings are rejected at daemon startup. |

**Caching:** Preflight runs at most once per git commit SHA within `preflightInterval`. Multiple queued prompts on the same baseline SHA reuse the cached result — no wasted container time.

**On failure:** The daemon logs the command, its captured output, and the commit SHA that was checked. A notification is sent. The prompt remains queued.
```

## 8. Write tests in `pkg/config/config_test.go`

Follow the existing patterns in the file (Ginkgo/Gomega, external package `config_test`).

### Test 8a: Defaults include new fields

In the `Describe("Defaults", ...)` block, extend the existing "returns config with default values" It block to also assert:

```go
Expect(cfg.PreflightCommand).To(Equal("make precommit"))
Expect(cfg.PreflightInterval).To(Equal("8h"))
```

### Test 8b: `validatePreflightInterval` — invalid duration rejected

```go
It("rejects invalid preflightInterval", func() {
    cfg := config.Defaults()
    cfg.PreflightInterval = "2x"
    err := cfg.Validate(ctx)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("preflightInterval"))
})
```

### Test 8c: `validatePreflightInterval` — empty string is valid (disables)

```go
It("allows empty preflightInterval (disables preflight)", func() {
    cfg := config.Defaults()
    cfg.PreflightInterval = ""
    err := cfg.Validate(ctx)
    Expect(err).NotTo(HaveOccurred())
})
```

### Test 8d: `ParsedPreflightInterval` — valid duration

```go
It("ParsedPreflightInterval parses valid duration", func() {
    cfg := config.Defaults()
    cfg.PreflightInterval = "2h"
    Expect(cfg.ParsedPreflightInterval()).To(Equal(2 * time.Hour))
})
```

### Test 8e: `ParsedPreflightInterval` — empty returns 0

```go
It("ParsedPreflightInterval returns 0 for empty", func() {
    cfg := config.Defaults()
    cfg.PreflightInterval = ""
    Expect(cfg.ParsedPreflightInterval()).To(Equal(time.Duration(0)))
})
```

## 9. Write CHANGELOG entry

Add `## Unreleased` at the top of `CHANGELOG.md` if it does not already exist, then append:

```
- feat: add `preflightCommand` and `preflightInterval` config fields for baseline check before prompt execution
```

## 10. Run `make test`

```bash
cd /workspace && make test
```

Must pass before `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Existing configs without the new fields must load unchanged (YAML omitempty and zero-value defaults handle this)
- `validatePreflightInterval` must follow the exact same pattern as `validateMaxPromptDuration` — reject only non-empty invalid strings
- The per-prompt end-of-run `validationCommand` field and its behavior must not change
- Use `errors.Errorf` from `github.com/bborbe/errors` (no `fmt.Errorf`)
- External test package (`package config_test`)
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "PreflightCommand\|PreflightInterval" pkg/config/config.go` — at least 6 matches (struct fields, defaults, validate, helper)
2. `grep -n "preflightCommand\|preflightInterval" docs/configuration.md` — at least 2 matches
3. `go test ./pkg/config/...` — passes
</verification>
