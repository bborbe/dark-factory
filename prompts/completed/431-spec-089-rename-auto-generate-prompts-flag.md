---
status: completed
spec: [089-rename-auto-generate-prompts-flag]
summary: Renamed disableAutoGeneratePrompts to autoGeneratePrompts with inverted polarity across config, CLI, factory, watcher, tests, and docs. Default flipped from auto-gen ON to OFF.
container: dark-factory-exec-431-spec-089-rename-auto-generate-prompts-flag
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-25T20:00:00Z"
queued: "2026-05-25T19:55:50Z"
started: "2026-05-25T20:00:02Z"
completed: "2026-05-25T20:14:46Z"
---

<summary>
- Renames the config flag `disableAutoGeneratePrompts` to `autoGeneratePrompts` everywhere in the live tree
- Inverts the polarity: `autoGeneratePrompts: true` enables auto-generation, `false` (or unset) disables it
- Flips the default: the daemon no longer auto-generates prompts on spec approval unless an operator opts in
- Removes the old key outright with no compatibility shim — passing the old name via `--set` or YAML is now an "unknown key" error
- Updates README, configuration docs, running-mode docs, generate-prompts-for-spec command doc, and CLI help text to match the new flag and the new default
- Updates the in-repo project config (`.dark-factory.yaml`) and the two `specs/ideas/*.md` notes that still reference the old name
- Watcher tests, layered-config tests, and `--set` parse tests are renamed and their polarity flipped; structure is preserved
- `make precommit` passes; reverse-grep for the old name returns zero hits in the live tree
</summary>

<objective>
Replace the negative-phrased opt-out flag `disableAutoGeneratePrompts` with the positive-phrased opt-in flag `autoGeneratePrompts` across config, watcher, CLI, factory, tests, and docs. After this work, auto-generation is OFF by default; operators set `autoGeneratePrompts: true` to enable it.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Coding plugin guides relevant to this work (read before editing):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo `It` block patterns used in `pkg/specwatcher/watcher_test.go` and `main_internal_test.go`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — required CHANGELOG entry format.

Source files to read before making changes:
- `pkg/config/config.go` — `Config` struct (lines 85-127); `DisableAutoGeneratePrompts bool` is at line 119 with YAML tag `disableAutoGeneratePrompts,omitempty`.
- `pkg/config/loader.go` — `LayeredProjectOverrides` struct (lines 33-44), `partialConfig` struct (lines 84-131) with `DisableAutoGeneratePrompts *bool` at line 122, the overrides capture at lines 175-187, and the merge branch `mergePartialSecurity` at lines 377-384.
- `pkg/config/sources.go` — `FieldSources` struct; `DisableAutoGeneratePrompts string` at line 20.
- `pkg/globalconfig/globalconfig.go` — `GlobalConfig` struct at lines 45-54 (`DisableAutoGeneratePrompts *bool` at line 52), the partial-struct anonymous type at lines 179-188 (line 186), and the merge branch at lines 211-213.
- `pkg/specwatcher/watcher.go` — variadic constructor `NewSpecWatcher` (lines 33-51) accepting `optionalDisableAutoGenerate ...bool`; struct field `disableAutoGenerate bool` (line 60); gate inside `handleFileEvent` at lines 157-164 with the INFO log line `"spec approved — auto-generation disabled, run /dark-factory:generate-prompts-for-spec <spec-path> manually"`.
- `pkg/specwatcher/watcher_test.go` — five `It` blocks at lines 421-549 that reference `disableAutoGenerate`.
- `pkg/factory/factory.go` — `LogEffectiveConfig` slog.Info call (lines 151-152) and `CreateSpecWatcher` (lines 687-699) passing `cfg.DisableAutoGeneratePrompts` as the 5th arg.
- `main.go` — `applyGlobalOverrides` branch (lines 558-560), source-annotation default `DisableAutoGeneratePrompts: "default"` (line 581), source-annotation global/project branches (lines 598-619), `supportedSetKeys` slice (line 692), `applyOneSetOverride` switch case (lines 769-775), and the two CLI help-text blocks at lines 987 and 1009.
- `main_internal_test.go` — `It` blocks at lines 269-287 (global/project precedence), 370-401 (field-sources computation), 615-660 (`--set` parsing).
- `README.md` — lines 153 and 155 ("User-level defaults" paragraph and the spec→prompts auto/manual sentence).
- `docs/configuration.md` — `--set` table row (line 446), YAML example (line 461), default-value table (line 466), behavior bullets (lines 471-472), and `--set` examples (lines 490-491).
- `docs/running.md` — config row (line 46), YAML snippet (line 74), `--set` examples (lines 80-81).
- `commands/generate-prompts-for-spec.md` — line 9 mentions the old flag in operator-facing prose.
- `.dark-factory.yaml` — line 2: `disableAutoGeneratePrompts: true`. Behavior under the new flag is the OPPOSITE: change to `autoGeneratePrompts: false` to preserve the operator's existing intent (auto-gen OFF for this project).
- `specs/ideas/awaiting-generation-telemetry.md` — line 14 mentions the old flag.
- `specs/ideas/per-spec-disable-auto-generate.md` — lines 12 and 14 mention the old flag.
- `specs/ideas/spec-generate-cli-subcommand.md` — lines 13 and 15 mention the old flag.
- `pkg/config/roundtrip_test.go` — reflection-based parity test between `Config` and `partialConfig`. No manual edits needed; the renamed field is picked up automatically. Just confirm it still passes.

Spec 089 verification (line 118 of the spec) excludes only `specs/completed/088-disable-auto-prompt-generation.md` and the rename-spec files themselves. The `specs/ideas/*.md` files are NOT excluded — they must be updated.

Pattern to mirror: the `HideGit` field already has the exact shape required for the new `AutoGeneratePrompts` field across all five layers (Config, partialConfig, GlobalConfig, FieldSources, LayeredProjectOverrides). Read the `HideGit` occurrences as the reference template.
</context>

<requirements>

This is one mechanical rename + polarity flip. Apply ALL edits in a single pass so the tree compiles end-to-end before running tests. Splitting the rename mid-flight would leave the tree in a state where `cfg.DisableAutoGeneratePrompts` is gone but `cfg.AutoGeneratePrompts` is not yet referenced by callers — do not split.

### 1. `pkg/config/config.go` — rename Config field

Change line 119 from:

```go
DisableAutoGeneratePrompts bool                `yaml:"disableAutoGeneratePrompts,omitempty"`
```

to:

```go
AutoGeneratePrompts        bool                `yaml:"autoGeneratePrompts,omitempty"`
```

(Adjust the column-alignment whitespace so `gofmt` is satisfied; the surrounding struct is column-aligned.)

### 2. `pkg/config/loader.go` — rename in three places

a. Line 43 inside `LayeredProjectOverrides`:

```go
DisableAutoGeneratePrompts *bool     // non-nil when .dark-factory.yaml explicitly sets disableAutoGeneratePrompts
```

becomes:

```go
AutoGeneratePrompts        *bool     // non-nil when .dark-factory.yaml explicitly sets autoGeneratePrompts
```

b. Line 122 inside `partialConfig`:

```go
DisableAutoGeneratePrompts *bool                `yaml:"disableAutoGeneratePrompts"`
```

becomes:

```go
AutoGeneratePrompts        *bool                `yaml:"autoGeneratePrompts"`
```

(YAML tag has NO `,omitempty` — preserves the existing behavior so an explicit `false` at the project layer beats a `true` at the global layer.)

c. Lines 175-187 — the overrides capture. Change:

```go
DisableAutoGeneratePrompts: partial.DisableAutoGeneratePrompts,
```

to:

```go
AutoGeneratePrompts:        partial.AutoGeneratePrompts,
```

d. Lines 377-384 — the `mergePartialSecurity` branch. Change:

```go
if partial.DisableAutoGeneratePrompts != nil {
    cfg.DisableAutoGeneratePrompts = *partial.DisableAutoGeneratePrompts
}
```

to:

```go
if partial.AutoGeneratePrompts != nil {
    cfg.AutoGeneratePrompts = *partial.AutoGeneratePrompts
}
```

### 3. `pkg/config/sources.go` — rename FieldSources field

Change line 20:

```go
DisableAutoGeneratePrompts string
```

to:

```go
AutoGeneratePrompts        string
```

### 4. `pkg/globalconfig/globalconfig.go` — rename in three places

a. Line 52 inside `GlobalConfig`:

```go
DisableAutoGeneratePrompts *bool             `yaml:"disableAutoGeneratePrompts,omitempty"`
```

becomes:

```go
AutoGeneratePrompts        *bool             `yaml:"autoGeneratePrompts,omitempty"`
```

b. Line 186 inside the anonymous `partial` struct in `Load`:

```go
DisableAutoGeneratePrompts *bool             `yaml:"disableAutoGeneratePrompts"`
```

becomes:

```go
AutoGeneratePrompts        *bool             `yaml:"autoGeneratePrompts"`
```

c. Lines 211-213 — the merge branch:

```go
if partial.DisableAutoGeneratePrompts != nil {
    cfg.DisableAutoGeneratePrompts = partial.DisableAutoGeneratePrompts
}
```

becomes:

```go
if partial.AutoGeneratePrompts != nil {
    cfg.AutoGeneratePrompts = partial.AutoGeneratePrompts
}
```

### 5. `pkg/specwatcher/watcher.go` — rename AND invert polarity at the gate

This is the only file where polarity (not just identity) changes.

a. Rename the variadic parameter, the local var, the godoc, and the struct field. The constructor signature becomes:

```go
// NewSpecWatcher creates a new SpecWatcher.
// An optional bool can be passed as the 5th argument to enable auto-generation.
// When the optional arg is omitted, auto-generation defaults to false (disabled).
func NewSpecWatcher(
    inProgressDir string,
    generator generator.SpecGenerator,
    debounce time.Duration,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    optionalAutoGenerate ...bool,
) SpecWatcher {
    autoGenerate := false
    if len(optionalAutoGenerate) > 0 {
        autoGenerate = optionalAutoGenerate[0]
    }
    return &specWatcher{
        inProgressDir:         inProgressDir,
        generator:             generator,
        debounce:              debounce,
        currentDateTimeGetter: currentDateTimeGetter,
        autoGenerate:          autoGenerate,
    }
}
```

b. Rename the struct field:

```go
type specWatcher struct {
    inProgressDir         string
    generator             generator.SpecGenerator
    debounce              time.Duration
    mu                    sync.Mutex
    currentDateTimeGetter libtime.CurrentDateTimeGetter
    autoGenerate          bool
}
```

c. Invert the gate in `handleFileEvent` (lines 157-164). The current code is:

```go
if w.disableAutoGenerate {
    slog.Info(
        "spec approved — auto-generation disabled, run /dark-factory:generate-prompts-for-spec <spec-path> manually",
        "path",
        specPath,
    )
    return
}
```

Replace with:

```go
if !w.autoGenerate {
    slog.Info(
        "spec approved — auto-generation disabled, run /dark-factory:generate-prompts-for-spec <spec-path> manually",
        "path",
        specPath,
    )
    return
}
```

Keep the log message string BYTE-IDENTICAL — the substring `auto-generation disabled` MUST appear verbatim (spec 089 Constraints, line 61). Operator muscle memory and any log-grep tooling depends on it.

### 6. `pkg/specwatcher/watcher_test.go` — rename test wording and FLIP every `true`/`false` literal passed to `NewSpecWatcher`

Polarity has flipped: every test that previously passed `true` (meaning "disable") now passes `false` (meaning "do not auto-generate"), and every test that passed `false` (meaning "do not disable, i.e. auto-generate") now passes `true`.

For the five `It` blocks at lines 421-549:

- Line 421: `It("does NOT call generator when disableAutoGenerate is true on new file event", ...)` → `It("does NOT call generator when autoGenerate is false on new file event", ...)`. The `NewSpecWatcher` call at line 425-431 currently passes `true` as the 5th arg with comment `// disableAutoGenerate = true` — change to `false` with comment `// autoGenerate = false (gen disabled)`. Behavior assertion unchanged (`GenerateCallCount() Should Equal 0`).

- Line 452: `It("logs INFO message when disableAutoGenerate is true", ...)` → `It("logs INFO message when autoGenerate is false", ...)`. The `NewSpecWatcher` call at line 460-466 currently passes `true` — change to `false`. Comment update to `// autoGenerate = false (gen disabled)`. Substring assertion `auto-generation disabled` unchanged.

- Line 488 (multi-line It): `"does NOT call generator when disableAutoGenerate is true for pre-existing spec on startup"` → `"does NOT call generator when autoGenerate is false for pre-existing spec on startup"`. The `NewSpecWatcher` call at line 500-506 currently passes `true` — change to `false` with comment `// autoGenerate = false (gen disabled)`.

- Line 521: `It("calls generator when disableAutoGenerate is false (default behavior)", ...)` → `It("calls generator when autoGenerate is true", ...)`. (Note: the parenthetical "default behavior" is now wrong since the new default is OFF; drop it.) The `NewSpecWatcher` call at line 525-531 currently passes `false` — change to `true` with comment `// autoGenerate = true (gen enabled)`.

- **CRITICAL — variadic-default flip will silently break multiple existing tests.** The other tests in the file that construct `NewSpecWatcher` WITHOUT the 5th arg rely on the variadic default. Under the OLD constructor, default = `disableAutoGenerate = false` = auto-gen ON. Under the NEW constructor, default = `autoGenerate = false` = auto-gen OFF. Every 4-arg test that asserted generation HAPPENED (`GenerateCallCount() >= 1` or similar) will now fail unless `true` is added as the 5th arg.

  The file contains **10** call sites at lines 91, 120, 141, 168, 206, 240, 284, 320, 360, 395 that omit the 5th arg today. Of these, the following assert `BeNumerically(">=", 1)` (or equivalent positive generation) and MUST be updated to pass `true,` as the new 5th arg:

  - Line 79's `It` block — `NewSpecWatcher(` at line 91; assertion at line 104 (`>= 1`)
  - Line 164's `It` block — `NewSpecWatcher(` at line 168; assertion at line 188 (`>= 1`)
  - Line 196's `It` block ("should NOT call generator on Write events") — `NewSpecWatcher(` at line 206; FIRST assertion at line 220 (`>= 1`) verifies the startup scan must fire. Add `true,` as the 5th arg so the startup scan triggers; the subsequent `Equal(callsBefore)` assertion (no re-trigger on Write) is unaffected.
  - Line 236's `It` block — `NewSpecWatcher(` at line 240; assertion at line 261 (`>= 1`)
  - Line 308's `It` block — `NewSpecWatcher(` at line 320; assertion at line 340 (`>= 1`)
  - Line 378's `It` block ("should work with relative paths") — `NewSpecWatcher(` at line 395; assertion at line 416 (`>= 1`)

  Add `true,` as the 5th arg to ALL SIX of the calls above. Do NOT touch the line-352 `It` block ("should NOT call generator for spec with status `prompted`") — its `NewSpecWatcher(` at line 360 belongs to a test whose assertion at line 373 is `Equal(0)` (no generation expected, filtered upstream by status); leaving it 4-arg under the new default is correct.

  For any other 4-arg call site whose test asserts `GenerateCallCount() == 0` (no generation), leave it 4-arg — the new default already gives the assertion-correct behavior.

  Read the full file before editing to confirm the line numbers and the assertion direction at each site; the line numbers above are from the audit at the time this prompt was written and may have drifted by one or two lines.

### 7. `pkg/factory/factory.go` — rename in two places

a. `LogEffectiveConfig` slog.Info call at lines 151-152:

```go
"disableAutoGeneratePrompts", cfg.DisableAutoGeneratePrompts,
"disableAutoGeneratePromptsSource", sources.DisableAutoGeneratePrompts,
```

becomes:

```go
"autoGeneratePrompts", cfg.AutoGeneratePrompts,
"autoGeneratePromptsSource", sources.AutoGeneratePrompts,
```

b. `CreateSpecWatcher` — locate the 5th argument inside the `NewSpecWatcher(...)` call. It currently reads `cfg.DisableAutoGeneratePrompts` (around line 709; the unique identifier text is what to anchor on, not the line number). The watcher's variadic is now `optionalAutoGenerate` (polarity flipped), and the `cfg.AutoGeneratePrompts` field has matching polarity, so the line becomes:

```go
cfg.AutoGeneratePrompts,
```

No inversion required — the polarity flip at the field name and at the gate cancel out (positive flag → positive parameter → positive gate test).

### 8. `main.go` — rename across `applyGlobalOverrides`, source-annotation defaults, branches, the supported-keys slice, the `--set` switch case, and both CLI help-text blocks

a. Lines 558-560 — global-overrides branch:

```go
if global.DisableAutoGeneratePrompts != nil && proj.DisableAutoGeneratePrompts == nil {
    cfg.DisableAutoGeneratePrompts = *global.DisableAutoGeneratePrompts
}
```

becomes:

```go
if global.AutoGeneratePrompts != nil && proj.AutoGeneratePrompts == nil {
    cfg.AutoGeneratePrompts = *global.AutoGeneratePrompts
}
```

b. Line 581 — `computeFieldSources` default annotation:

```go
DisableAutoGeneratePrompts: "default",
```

becomes:

```go
AutoGeneratePrompts:        "default",
```

c. Lines 598-600 — global branch:

```go
if global.DisableAutoGeneratePrompts != nil {
    s.DisableAutoGeneratePrompts = "global"
}
```

becomes:

```go
if global.AutoGeneratePrompts != nil {
    s.AutoGeneratePrompts = "global"
}
```

d. Lines 617-619 — project branch:

```go
if proj.DisableAutoGeneratePrompts != nil {
    s.DisableAutoGeneratePrompts = "project"
}
```

becomes:

```go
if proj.AutoGeneratePrompts != nil {
    s.AutoGeneratePrompts = "project"
}
```

e. Line 692 — `supportedSetKeys` slice entry:

```go
"disableAutoGeneratePrompts",
```

becomes:

```go
"autoGeneratePrompts",
```

f. Lines 769-775 — the `--set` switch case in `applyOneSetOverride`:

```go
case "disableAutoGeneratePrompts":
    b, err := parseStrictBool(ctx, key, value)
    if err != nil {
        return err
    }
    cfg.DisableAutoGeneratePrompts = b
    sources.DisableAutoGeneratePrompts = "arg"
```

becomes:

```go
case "autoGeneratePrompts":
    b, err := parseStrictBool(ctx, key, value)
    if err != nil {
        return err
    }
    cfg.AutoGeneratePrompts = b
    sources.AutoGeneratePrompts = "arg"
```

g. Lines 987 and 1009 — both CLI help-text blocks. The existing suffix is `, autoMerge, disableAutoGeneratePrompts`. Change BOTH occurrences to `, autoMerge, autoGeneratePrompts`. Do not add any other text changes to the help blocks.

### 9. `main_internal_test.go` — rename It descriptions, field references, and map keys; do NOT flip polarity

These tests verify the layering plumbing (which layer wins) and the `--set` parsing — not behavior at the watcher gate. Polarity is irrelevant to them; just rename identifiers.

a. Lines 269-287 — the two "applies global" / "does not overwrite project" tests for the precedence: rename every `disableAutoGeneratePrompts` (in `It` descriptions) and `DisableAutoGeneratePrompts` (Go field access on `global`, `proj`, `cfg`) to `autoGeneratePrompts` / `AutoGeneratePrompts`. The assertion shape (e.g. `cfg.AutoGeneratePrompts To BeTrue / BeFalse`) stays as-is — these are layering tests, not behavior tests.

b. Lines 370-401 — the four "returns default/global/project" tests for `computeFieldSources`. Same rename pattern: `DisableAutoGeneratePrompts` → `AutoGeneratePrompts` everywhere.

c. Lines 615-660 — the three `--set` tests. Rename:
   - `It` description text: `disableAutoGeneratePrompts` → `autoGeneratePrompts` (in all three).
   - Map key in `map[string]string{"disableAutoGeneratePrompts": "true"}` → `map[string]string{"autoGeneratePrompts": "true"}` (same for `"false"` and `"yes"` variants).
   - Field references: `cfg.DisableAutoGeneratePrompts`, `sources.DisableAutoGeneratePrompts` → `cfg.AutoGeneratePrompts`, `sources.AutoGeneratePrompts`.

d. ADD ONE NEW `It` block in the same describe block as the `--set` tests (after line 660): asserts that `--set disableAutoGeneratePrompts=true` (the OLD key) now returns an unknown-key parse error. Pattern mirror the existing strict-bool rejection test at line 648. Example:

```go
It("rejects --set disableAutoGeneratePrompts=true as unknown key (legacy name removed)", func() {
    cfg := config.Defaults()
    sources := config.FieldSources{}
    err := applySetOverrides(
        ctx,
        &cfg,
        &sources,
        "run",
        map[string]string{"disableAutoGeneratePrompts": "true"},
    )
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("disableAutoGeneratePrompts"))
})
```

(Confirm the exact error string by reading what `applyOneSetOverride` returns for an unrecognised key — the assertion above checks that the error names the unknown key. Adjust the substring if the actual error wording differs.)

### 10. `README.md` — line 153 and line 155: rename AND invert prose

a. Line 153 currently reads:

```
**User-level defaults** in `~/.dark-factory/config.yaml` apply across every project that doesn't override them. Supports `model`, `hideGit`, `autoRelease`, `dirtyFileThreshold`, `maxContainers`, `disableAutoGeneratePrompts`. Precedence: default ← global ← project ← CLI arg.
```

Change `disableAutoGeneratePrompts` to `autoGeneratePrompts`. No other change on line 153.

b. Line 155 currently reads:

```
**Spec→prompts can be auto or manual.** Default: the daemon auto-generates prompts when a spec is approved. Set `disableAutoGeneratePrompts: true` to defer generation and invoke `/dark-factory:generate-prompts-for-spec <spec-path>` by hand. See [docs/running.md § Two ways to generate prompts](docs/running.md#two-ways-to-generate-prompts-from-an-approved-spec) for tradeoffs.
```

Rewrite to:

```
**Spec→prompts can be auto or manual.** Default: the daemon does NOT auto-generate prompts when a spec is approved — invoke `/dark-factory:generate-prompts-for-spec <spec-path>` by hand. Set `autoGeneratePrompts: true` in `~/.dark-factory/config.yaml`, `.dark-factory.yaml`, or via `--set autoGeneratePrompts=true` to enable auto-generation. See [docs/running.md § Two ways to generate prompts](docs/running.md#two-ways-to-generate-prompts-from-an-approved-spec) for tradeoffs.
```

### 11. `docs/configuration.md` — rename and invert in five locations

a. Line 446 (table row):

```
| `disableAutoGeneratePrompts` | bool (`true` or `false`) | `--set disableAutoGeneratePrompts=true` |
```

becomes:

```
| `autoGeneratePrompts` | bool (`true` or `false`) | `--set autoGeneratePrompts=true` |
```

b. Line 461 (YAML example):

```
disableAutoGeneratePrompts: true
```

becomes:

```
autoGeneratePrompts: true
```

c. Line 466 (default-value description). Current row reads "`disableAutoGeneratePrompts` | `false` (enabled) | When `true`, the spec watcher will NOT auto-fire …". Replace with:

```
| `autoGeneratePrompts` | `false` (disabled) | When `true`, the spec watcher auto-fires the generator container when a spec is approved. When `false` (the default), the watcher logs an INFO line and does NOT start the generator; operators run `/dark-factory:generate-prompts-for-spec <spec-path>` manually to trigger generation. |
```

d. Lines 471-472 (behavior bullets). Current:

```
- `disableAutoGeneratePrompts: false` (default): Approving a spec triggers the generator container. Prompts appear in `prompts/` automatically.
- `disableAutoGeneratePrompts: true`: Approving a spec logs an INFO line and does NOT start the generator. The spec stays at `status: approved` in `specs/in-progress/`.
```

Replace with:

```
- `autoGeneratePrompts: false` (default): Approving a spec logs an INFO line and does NOT start the generator. The spec stays at `status: approved` in `specs/in-progress/`.
- `autoGeneratePrompts: true`: Approving a spec triggers the generator container. Prompts appear in `prompts/` automatically.
```

e. Lines 490-491 (`--set` examples):

```
dark-factory daemon --set disableAutoGeneratePrompts=true
dark-factory run --set disableAutoGeneratePrompts=false
```

becomes:

```
dark-factory daemon --set autoGeneratePrompts=true
dark-factory run --set autoGeneratePrompts=false
```

If a heading near these lines mentions "Disable Auto Prompt Generation", rename to "Auto Prompt Generation" so the heading reflects the new positive flag.

### 12. `docs/running.md` — rename and invert in three locations

a. Line 46 (config row in the auto-vs-manual table). Current:

```
| **Config** | `disableAutoGeneratePrompts: false` (default) | `disableAutoGeneratePrompts: true` in `.dark-factory.yaml`, `~/.dark-factory/config.yaml`, or `--set disableAutoGeneratePrompts=true` |
```

Replace with (note: the "auto" column and "manual" column swap which side names the flag, because polarity flipped):

```
| **Config** | `autoGeneratePrompts: true` in `.dark-factory.yaml`, `~/.dark-factory/config.yaml`, or `--set autoGeneratePrompts=true` | `autoGeneratePrompts: false` (default) |
```

b. Line 74 (YAML snippet):

```
disableAutoGeneratePrompts: true   # or false
```

becomes:

```
autoGeneratePrompts: true   # or false
```

c. Lines 80-81 (`--set` examples):

```
dark-factory daemon --set disableAutoGeneratePrompts=true
dark-factory run    --set disableAutoGeneratePrompts=false
```

becomes:

```
dark-factory daemon --set autoGeneratePrompts=true
dark-factory run    --set autoGeneratePrompts=false
```

If surrounding prose describes the default ("default: auto-on, set to true to disable" or similar), rewrite to match the new default ("default: off, set `autoGeneratePrompts: true` to enable auto-fire"). Read the full paragraph and rewrite for coherence.

### 13. `commands/generate-prompts-for-spec.md` — line 9 rename + invert

Current line 9:

```
**When to reach for this command** (vs. letting the daemon auto-generate): you have `disableAutoGeneratePrompts: true` and want to trigger generation for a specific approved spec, or you want to re-generate prompts for a spec whose first attempt was rejected. See [docs/running.md § Two ways to generate prompts](../docs/running.md#two-ways-to-generate-prompts-from-an-approved-spec) for the auto-vs-manual tradeoffs.
```

Replace with:

```
**When to reach for this command** (vs. letting the daemon auto-generate): you have `autoGeneratePrompts: false` (the default) and want to trigger generation for a specific approved spec, or you want to re-generate prompts for a spec whose first attempt was rejected. See [docs/running.md § Two ways to generate prompts](../docs/running.md#two-ways-to-generate-prompts-from-an-approved-spec) for the auto-vs-manual tradeoffs.
```

### 14. `.dark-factory.yaml` — update the project config to preserve current intent

Line 2 currently reads:

```yaml
disableAutoGeneratePrompts: true
```

The current intent is "auto-gen OFF for this project". Under the new flag with new polarity, "auto-gen OFF" is `autoGeneratePrompts: false`. But the new DEFAULT is already `false`, so the explicit line is redundant. Two acceptable resolutions — pick (1):

1. (Recommended.) Replace line 2 with `autoGeneratePrompts: false` so the project file remains explicit about its intent. This also serves as a worked example of the new flag in the operator's own config.
2. Delete line 2 entirely. The default takes effect.

Use option (1). Behavior is identical; explicitness wins because the file is also documentation for any operator forking this repo.

### 15. `specs/ideas/*.md` — rename references to the old flag (prose-level, no behavior implied)

The verification reverse-grep is not excluded for `specs/ideas/`. Update:

a. `specs/ideas/awaiting-generation-telemetry.md` line 14 — replace `disableAutoGeneratePrompts` with `autoGeneratePrompts` in the prose. Adjust polarity wording around it (e.g. "depends on `autoGeneratePrompts` being shipped" reads identically). Read the surrounding sentence and adjust if "shipped and in use" needs rewording (e.g., "in use with `autoGeneratePrompts: true`" if the original implied "operator opts in to the gate").

b. `specs/ideas/per-spec-disable-auto-generate.md` — lines 12 and 14. Line 12 mentions `disableAutoGenerate: true` per-spec frontmatter; line 14 mentions `Config.DisableAutoGeneratePrompts`. Update line 14 to `Config.AutoGeneratePrompts` and invert any "today" prose to match the new default. Line 12's per-spec field naming is OUT OF SCOPE for this rename (it's a separate frontmatter concept tracked under that idea file) — leave the per-spec field name alone, but rewrite the surrounding prose to read against the new global flag name. If you're unsure, leave a short HTML comment in the file: `<!-- TODO 089: re-evaluate this idea against autoGeneratePrompts (default false) -->` and move on.

c. `specs/ideas/spec-generate-cli-subcommand.md` — lines 13 and 15. Replace `disableAutoGeneratePrompts` with `autoGeneratePrompts` in the prose. Where line 13/15 describe polarity ("that flag pauses automation" → still true since the new default also pauses automation), adjust wording so the sentence reads correctly under the new default ("under the new default, automation is paused; this subcommand offers an explicit alternative").

### 16. CHANGELOG.md — add a single entry

Read `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` for format. Add an `[Unreleased]` entry (or under whichever next-version section is active in this repo) documenting:

- BREAKING: renamed `disableAutoGeneratePrompts` → `autoGeneratePrompts` everywhere; polarity inverted; default flipped from auto-gen ON to auto-gen OFF. The old key is no longer recognised — operators with `disableAutoGeneratePrompts` in `~/.dark-factory/config.yaml` or `.dark-factory.yaml` must rewrite to `autoGeneratePrompts: true` if they want the pre-rename behavior.

Mirror the wording style of entries 76-84 in the existing CHANGELOG (which document the original 088 spec).

### 17. Final cleanup pass

After all edits, run the reverse-grep listed in `<verification>` below. Any hit OTHER than `specs/completed/088-disable-auto-prompt-generation.md`, `specs/in-progress/089-rename-auto-generate-prompts-flag.md`, `prompts/completed/409-411-spec-088-*.md` (historical prompt archives — see exclusion note), or this very prompt file is a defect — fix it.

Note on the `prompts/completed/` exclusion: the spec's reverse-grep only excludes the spec files themselves, not the completed prompts. Completed prompt files document historical work and naturally reference the old name. The reverse-grep command in `<verification>` excludes `prompts/completed/` to avoid false positives — this matches the practical intent of the spec (rename the LIVE tree, not the historical record).

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT add a back-compat shim for the old key. The old key MUST return the same unknown-key error any other unknown `--set` key returns.
- Do NOT add a deprecation warning for the old key.
- The INFO log line in the watcher MUST contain the verbatim substring `auto-generation disabled` (preserves spec 088's wording and operator muscle memory).
- The watcher constructor stays variadic (`...bool`). Do NOT change to a required parameter — Go has no default arguments and the variadic pattern is what callers (including the factory and tests) rely on.
- Apply ALL edits in one pass. Do NOT push to git mid-way through a half-renamed tree.
- After all renames, `make precommit` MUST pass on the first try; the parity test in `pkg/config/roundtrip_test.go` is the canary for `Config`/`partialConfig` drift.
- Existing layering precedence, merge logic, and the effective-config log format are NOT changed beyond the field/source rename.
- The reverse-grep at the end MUST return zero lines (excluding the documented exceptions).
</constraints>

<verification>

```bash
# 1. Build + test
make precommit
go test ./pkg/config/... -v
go test ./pkg/specwatcher/... -v
go test ./pkg/globalconfig/... -v
go test ./pkg/config/... -run Roundtrip -v
go test . -run "ApplyGlobal|AppliesProject|SetOverrides|AutoGeneratePrompts" -v

# 2. Forward-grep: new name is present where the spec requires it
grep -nE 'AutoGeneratePrompts\s+bool' pkg/config/config.go            # ≥1
grep -nE 'AutoGeneratePrompts\s+\*bool' pkg/config/loader.go          # ≥1
grep -nE 'AutoGeneratePrompts\s+string' pkg/config/sources.go         # ≥1
grep -nE 'AutoGeneratePrompts\s+\*bool' pkg/globalconfig/globalconfig.go  # ≥1
grep -nE 'autoGenerate' pkg/specwatcher/watcher.go                    # ≥3 (field, var, gate)
grep -nE 'autoGeneratePrompts(Source)?' pkg/factory/factory.go        # ≥2
grep -nE '"autoGeneratePrompts"' main.go                              # ≥1 (allowed-keys + switch case)
grep -n 'autoGeneratePrompts' README.md                               # ≥2
grep -nE 'autoGeneratePrompts' docs/configuration.md                  # ≥4
grep -nE 'autoGeneratePrompts' docs/running.md                        # ≥3
grep -n 'autoGeneratePrompts' commands/generate-prompts-for-spec.md   # ≥1

# 3. CRITICAL: reverse-grep — old name must be GONE from the live tree.
# Exclusions:
#   - specs/completed/088-disable-auto-prompt-generation.md (historical spec)
#   - specs/in-progress/089-rename-auto-generate-prompts-flag.md (this spec)
#   - prompts/completed/*spec-088* (historical prompt archives)
#   - prompts/*rename-auto-generate-prompts-flag.md (this prompt and its lifecycle copies)
#   - CHANGELOG.md (allowed: the new entry documents the breaking change by name)
git grep -i 'disableautogenerateprompts' \
  -- ':!specs/completed/088-disable-auto-prompt-generation.md' \
     ':!specs/in-progress/*rename-auto-generate-prompts-flag*' \
     ':!specs/completed/*rename-auto-generate-prompts-flag*' \
     ':!prompts/completed/*spec-088*' \
     ':!prompts/*rename-auto-generate-prompts-flag*' \
     ':!CHANGELOG.md'
# Expected output: zero lines.

# 4. Operator-config audit (informational only — surfaces the post-rename action)
grep -i 'autogenerateprompts' ~/.dark-factory/config.yaml 2>/dev/null || echo 'no global flag set'
grep -i 'autogenerateprompts' .dark-factory.yaml 2>/dev/null || echo 'no project flag'
```

The reverse-grep at step 3 returning zero lines is the gating success condition. Any hit there means a rename was missed.
</verification>
