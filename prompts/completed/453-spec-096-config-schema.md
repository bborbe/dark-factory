---
status: completed
spec: [096-healthcheck-on-daemon-startup]
summary: Added healthcheckEnabled (*bool) and healthcheckInterval (string, default 8h) to Config with validation, parsed getters, FieldSources tracking, LayeredProjectOverrides detection, partialConfig merge path, and four new keys in the effective-config startup log line
container: dark-factory-healthcheck-startup-exec-453-spec-096-config-schema
dark-factory-version: v0.180.2
created: "2026-06-16T20:10:00Z"
queued: "2026-06-16T20:22:17Z"
started: "2026-06-16T20:22:18Z"
completed: "2026-06-16T20:32:55Z"
branch: dark-factory/healthcheck-on-daemon-startup
---

<summary>
- Adds two new project-config settings: a switch to enable/disable the daemon healthcheck startup gate, and the duration its successful result is cached.
- The enable switch defaults to ON; the cache duration defaults to 8 hours — matching the existing preflight cache window.
- An unparseable cache-duration value is rejected at startup with a clear field-named error (daemon refuses to start, same as the existing preflight-interval behaviour).
- The startup "effective config" diagnostic log line now also reports both new settings together with which config layer supplied each value, so an operator can audit the gate at a glance.
- The existing preflight settings (`preflightCommand`, `preflightInterval`, `--skip-preflight`) are not touched — their behaviour stays byte-for-byte identical.
- No gate logic ships in this prompt; this is purely the config surface and the diagnostic log fields that later prompts observe.
</summary>

<objective>
Land the config surface for the daemon healthcheck startup gate: a `healthcheckEnabled` boolean (default true) and a `healthcheckInterval` duration string (default `8h`) on `config.Config`, with validation, a parsed-getter, source-detection in `FieldSources`, and four new keys in the effective-config startup log line. No gate behaviour ships here — only the config and diagnostics that later prompts depend on.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.

Read these files fully before editing:
- `/workspace/pkg/config/config.go` — the `Config` struct, `Defaults()`, `Validate()`, and the existing `PreflightInterval` field + `ParsedPreflightInterval()` + `validatePreflightInterval()`. Mirror the preflight-interval pattern exactly for `HealthcheckInterval`.
- `/workspace/pkg/config/sources.go` — the `FieldSources` struct (fields are plain `string` with values `"default"|"global"|"project"|"arg"`).
- `/workspace/pkg/factory/factory.go` — `LogEffectiveConfig(...)` (around the `slog.Info("effective config", ...)` call) emits the diagnostic line; note the existing `"preflightCommand"`/`"preflightInterval"` key/value pairs there.
- `/workspace/main.go` — `computeFieldSources(...)` resolves each `FieldSources` field; `ParseArgs(...)` returns the parsed flags. Read `LoadWithOverrides` usage and how `cfg` is built in `run(ctx)`.
- `/workspace/pkg/config/loader.go` — `LayeredProjectOverrides` struct.

Coding-plugin docs (paths are in-container — read them):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — use `errors.Errorf(ctx, ...)`, never `fmt.Errorf`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega + counterfeiter; external `_test` packages; coverage ≥80%.

Verified facts (quoted from current source — do not re-invent):
- `config.Config` already has `PreflightInterval string \`yaml:"preflightInterval"\`` and the helper:
  ```go
  func (c Config) ParsedPreflightInterval() time.Duration {
      if c.PreflightInterval == "" { return 0 }
      d, err := time.ParseDuration(c.PreflightInterval)
      if err != nil { return 0 }
      return d
  }
  ```
- `Defaults()` sets `PreflightInterval: "8h"`.
- `validatePreflightInterval` returns `errors.Errorf(ctx, "preflightInterval %q is not a valid duration: %v", ...)` and is registered in `Validate` via `validation.Name("preflightInterval", validation.HasValidationFunc(c.validatePreflightInterval))`.
- `FieldSources` is a flat struct of `string` fields; valid values are `"default"|"global"|"project"|"arg"`.
- The effective-config line is one `slog.Info("effective config", k, v, ...)` call in `LogEffectiveConfig`; it already passes `"preflightInterval", cfg.PreflightInterval`.
</context>

<requirements>
1. In `/workspace/pkg/config/config.go`, add two fields to the `Config` struct, immediately after the existing `PreflightInterval string \`yaml:"preflightInterval"\`` line:
   ```go
   HealthcheckEnabled  *bool  `yaml:"healthcheckEnabled,omitempty"`
   HealthcheckInterval string `yaml:"healthcheckInterval"`
   ```
   Use `*bool` (nil = "not set in project YAML" = default-enabled) so the effective-config source detection can distinguish a project override from the default. Do NOT use a plain `bool`.

2. In `Defaults()`, set `HealthcheckInterval: "8h"`. Do NOT set `HealthcheckEnabled` in `Defaults()` — leave it nil; nil means enabled. (A `*bool` cannot be a useful default literal here without a package-level helper; the getter in step 3 encodes "nil == enabled".)

3. Add two getter methods near `ParsedPreflightInterval`:
   ```go
   // HealthcheckEnabledValue reports whether the healthcheck startup gate is enabled.
   // nil HealthcheckEnabled means enabled (the default); only an explicit `false` disables it.
   func (c Config) HealthcheckEnabledValue() bool {
       if c.HealthcheckEnabled == nil {
           return true
       }
       return *c.HealthcheckEnabled
   }

   // ParsedHealthcheckInterval returns the parsed duration from HealthcheckInterval.
   // Returns 0 when HealthcheckInterval is empty (disables interval-based caching).
   // Safe to call at any time — returns 0 on error, never panics.
   func (c Config) ParsedHealthcheckInterval() time.Duration {
       if c.HealthcheckInterval == "" {
           return 0
       }
       d, err := time.ParseDuration(c.HealthcheckInterval)
       if err != nil {
           return 0
       }
       return d
   }
   ```

4. Add a validator mirroring `validatePreflightInterval`:
   ```go
   // validateHealthcheckInterval rejects unparseable duration strings for healthcheckInterval.
   func (c Config) validateHealthcheckInterval(ctx context.Context) error {
       if c.HealthcheckInterval == "" {
           return nil
       }
       if _, err := time.ParseDuration(c.HealthcheckInterval); err != nil {
           return errors.Errorf(
               ctx,
               "healthcheckInterval %q is not a valid duration: %v",
               c.HealthcheckInterval,
               err,
           )
       }
       return nil
   }
   ```
   Register it in `Validate(ctx)` alongside the preflight entries:
   ```go
   validation.Name(
       "healthcheckInterval",
       validation.HasValidationFunc(c.validateHealthcheckInterval),
   ),
   ```

5. In `/workspace/pkg/config/sources.go`, add two fields to `FieldSources`:
   ```go
   HealthcheckEnabled  string
   HealthcheckInterval string
   ```

6. **Thread the new fields through `partialConfig` + `mergePartialTimings` + `LayeredProjectOverrides`** — this is mandatory. `pkg/config/roundtrip_test.go` contains a reflection-based parity test (lines ~50-74) that fails CI when a `Config` field has no matching `partialConfig` field, AND every YAML-loaded scalar on `Config` is routed via this path. Skipping it = YAML values for these new fields are silently dropped (operator's `healthcheckInterval: "4h"` ignored) AND `make precommit` fails the parity test. Mirror exactly what `HideGit` / `AutoRelease` do (already-shipped layered bools).

   a. In `/workspace/pkg/config/loader.go` `partialConfig` struct (around line 127), add:
      ```go
      HealthcheckEnabled  *bool   `yaml:"healthcheckEnabled"`
      HealthcheckInterval *string `yaml:"healthcheckInterval"`
      ```
   b. In `mergePartialTimings` (or alongside it — pattern-match the file structure), copy non-nil partial values to `Config`:
      ```go
      if p.HealthcheckEnabled != nil { c.HealthcheckEnabled = p.HealthcheckEnabled }
      if p.HealthcheckInterval != nil { c.HealthcheckInterval = *p.HealthcheckInterval }
      ```
   c. In `LayeredProjectOverrides` struct (loader.go:33), add the two `*bool`/`*string` fields so `computeFieldSources` can detect the project layer.
   d. In `/workspace/main.go` `computeFieldSources(...)`, mirror the existing `HideGit`/`AutoRelease` detection pattern:
      ```go
      if proj.HealthcheckEnabled != nil { s.HealthcheckEnabled = "project" }
      if proj.HealthcheckInterval != nil { s.HealthcheckInterval = "project" }
      ```
      And initialise both to `"default"` in the returned `config.FieldSources{...}` literal so the keys are never empty.
   e. Add a `roundtrip_test.go` case verifying `healthcheckEnabled: false\nhealthcheckInterval: "3h"` survives `Load` end-to-end — guards against regression of the partial-drop bug.

   Note: the global config layer is intentionally NOT wired in this spec; the `"global"` source value remains reserved for a future spec. `arg` is set in prompt 3 by `--skip-healthcheck`. Source values emitted: `default` | `project` | `arg`.

7. In `/workspace/pkg/factory/factory.go` `LogEffectiveConfig(...)`, add four key/value pairs to the `slog.Info("effective config", ...)` call, immediately after the existing `"preflightInterval", cfg.PreflightInterval,` pair:
   ```go
   "healthcheckEnabled", cfg.HealthcheckEnabledValue(),
   "healthcheckEnabledSource", sources.HealthcheckEnabled,
   "healthcheckInterval", cfg.HealthcheckInterval,
   "healthcheckIntervalSource", sources.HealthcheckInterval,
   ```
   The `sources` parameter is already in scope in `LogEffectiveConfig` (it takes `sources config.FieldSources`). Falling back to `"default"` when a source string is empty is already the contract of `FieldSources` (zero value treated as default) — but step 6 guarantees these are non-empty, so emit them as-is.

8. Tests — add/extend tests so all new code paths are covered (≥80% for `pkg/config`):
   - `pkg/config` (external `package config_test`): table-test `HealthcheckEnabledValue()` for nil→true, `&true`→true, `&false`→false; `ParsedHealthcheckInterval()` for "" →0, "8h"→8h, "garbage"→0; `validateHealthcheckInterval` via `Config.Validate(ctx)` for valid ("8h") accepted and invalid ("nope") rejected with a message containing `healthcheckInterval`. Assert `Defaults().HealthcheckInterval == "8h"` and `Defaults().HealthcheckEnabledValue() == true`.
   - This is the boundary test required by the spec: `healthcheckInterval` crosses the config-validation boundary — assert that `Config{HealthcheckInterval: "nope"}.Validate(ctx)` returns a non-nil error mentioning the field, and that `Config{HealthcheckInterval: "8h"}` (with the other required fields populated from `Defaults()`) passes that specific validator.

9. Do NOT modify `preflightCommand`, `preflightInterval`, `ParsedPreflightInterval`, `validatePreflightInterval`, or `--skip-preflight` in any way. Existing preflight tests must pass unmodified.
</requirements>

<constraints>
- Copied from spec: Do NOT modify `preflightCommand`, `preflightInterval`, or `--skip-preflight` — they keep current semantics exactly. Existing preflight tests must continue to pass.
- Copied from spec: Do NOT expose the healthcheck as a user-configurable command string. The config shape is `healthcheckEnabled` (bool) + `healthcheckInterval` (duration) only — no `healthcheckCommand`.
- Copied from spec: `healthcheckInterval` unparseable → daemon exits non-zero at config-load with a parse-error message naming the field. (This is the `validateHealthcheckInterval` requirement.)
- Use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never `context.Background()`.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run in `/workspace`:
```bash
make precommit
```
- `make precommit` must exit 0 (covers tests, lint, vet, coverage; the project is `-mod=mod` and has no `vendor/` — never use `-mod=vendor`).
- A `Config{HealthcheckInterval:"nope"}.Validate(ctx)` test must fail validation with a message containing `healthcheckInterval`.
- Confirm `grep -n 'healthcheckEnabled\|healthcheckIntervalSource' pkg/factory/factory.go` shows the four new effective-config keys.
- `roundtrip_test.go` parity check must pass (it does iff partialConfig has the new fields — see step 6a).
</verification>
