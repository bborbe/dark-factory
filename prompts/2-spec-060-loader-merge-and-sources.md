---
status: draft
spec: [060-config-layering-phase-1]
created: "2026-05-01T08:01:00Z"
---

<summary>
- The project config loader applies global config fields before project fields, so a value set in `~/.dark-factory/config.yaml` becomes the effective default for every project that does not override it.
- The four user-pref fields (`hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`) traverse default ← global ← project precedence.
- Project-level config still wins over global when a project explicitly sets a field — including when the project explicitly sets `false` or `0`.
- The startup `effective config` log line names the source of each layered field (`hideGitSource=global`, `modelSource=project`, etc.) so operators can tell which layer won by reading the log.
- Project config validation now rejects an explicit empty `model: ""` at any layer — flagged as a minor behavior tightening in the changelog.
- No project's existing `.dark-factory.yaml` requires modification (the empty-model case was never valid in practice).
</summary>

<objective>
Wire the project config loader to consume the expanded `globalconfig.GlobalConfig` from prompt 1, apply layered precedence (default ← global ← project) for the four user-pref fields, track per-field sources for diagnostic logging, and tighten validation to reject explicit empty model strings.

**Precondition:** Prompt 1 (`1-spec-060-global-config-expansion.md`) is complete. `pkg/globalconfig.GlobalConfig` has the new pointer fields and the `ModelPattern` regex.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read these guides in `~/.claude/plugins/marketplaces/coding/docs/`:
- `go-validation-framework-guide.md`
- `go-factory-pattern.md`
- `go-testing-guide.md`

Read these files before editing:
- `pkg/config/config.go` — `Config` struct, `Validate`, the model field, `validateModel` (if present)
- `pkg/config/loader.go` — `Loader` interface, `fileLoader.Load`, `partialConfig`, `mergePartial*` helpers
- `pkg/config/config_test.go` and `pkg/config/config_loader_test.go` — existing test patterns
- `pkg/factory/factory.go` — `LogEffectiveConfig` (~line 89), `EffectiveMaxContainers`, `createStartupLogger` (~line 130), and where `Loader.Load` is called
- `main.go` — where `globalconfig.NewLoader().Load(ctx)` and `config.NewLoader().Load(ctx)` are invoked (around lines 520 and earlier in `run()`)
- `mocks/config-loader.go` — counterfeiter mock for `Loader`

Spec: `specs/in-progress/060-config-layering-phase-1.md` — desired behaviors 2, 3, 4, 7.
</context>

<requirements>

## 1. Change `pkg/config.Loader` interface to accept `globalCfg`

In `pkg/config/loader.go`, update the `Loader` interface and `fileLoader`:

```go
//counterfeiter:generate -o ../../mocks/config-loader.go --fake-name Loader . Loader

// Loader loads configuration from a file with global-layer precedence applied.
type Loader interface {
    Load(ctx context.Context, globalCfg globalconfig.GlobalConfig) (Config, ResolutionSources, error)
}
```

Add the import:

```go
"github.com/bborbe/dark-factory/pkg/globalconfig"
```

This creates a new package dependency `pkg/config → pkg/globalconfig`. Confirm this introduces no cycle (`pkg/globalconfig` imports nothing from dark-factory).

## 2. Add `ResolutionSources` type

In `pkg/config/loader.go` (or a new `pkg/config/sources.go` if cleaner), add:

```go
// ResolutionSources records which layer supplied the final value for each
// layered field. Values are one of "default", "global", "project", or "arg".
// Only fields that participate in layered precedence are tracked.
type ResolutionSources struct {
    HideGit            string
    AutoRelease        string
    DirtyFileThreshold string
    Model              string
    MaxContainers      string
}
```

The `MaxContainers` source mirrors today's existing logic (the `LogEffectiveConfig` function in factory currently computes it inline — move that source determination into the loader so all five fields are determined uniformly and `LogEffectiveConfig` only formats).

## 3. Refactor `fileLoader.Load`

The new signature:

```go
func (l *fileLoader) Load(ctx context.Context, globalCfg globalconfig.GlobalConfig) (Config, ResolutionSources, error) {
    cfg := Defaults()
    sources := ResolutionSources{
        HideGit:            "default",
        AutoRelease:        "default",
        DirtyFileThreshold: "default",
        Model:              "default",
        MaxContainers:      "default",
    }

    // Apply global layer (only sets fields the global config explicitly set).
    applyGlobal(&cfg, &sources, globalCfg)

    // Read project file (existing logic). On not-exist, return now with cfg+sources.
    // ... existing read + permission warning ...

    // Parse partial.
    var partial partialConfig
    if err := yaml.Unmarshal(data, &partial); err != nil {
        return Config{}, ResolutionSources{}, errors.Wrap(ctx, err, "parse config file")
    }

    // Apply project layer (existing mergePartial logic, but also update sources).
    mergePartial(&cfg, &partial)
    updateSourcesFromProject(&sources, &partial)

    // Existing workflow / pr / worktree migration logic stays unchanged.
    // ... (block currently at lines 131-180) ...

    // Validate.
    if err := cfg.Validate(ctx); err != nil {
        return Config{}, ResolutionSources{}, errors.Wrap(ctx, err, "validate config")
    }

    return cfg, sources, nil
}
```

## 4. Add `applyGlobal`

In `pkg/config/loader.go`, add a private function:

```go
// applyGlobal merges fields from globalCfg onto cfg and records "global" as the
// source for each field that was explicitly set globally. Fields not set globally
// keep their default-layer value and source.
func applyGlobal(cfg *Config, sources *ResolutionSources, globalCfg globalconfig.GlobalConfig) {
    if globalCfg.HideGit != nil {
        cfg.HideGit = *globalCfg.HideGit
        sources.HideGit = "global"
    }
    if globalCfg.AutoRelease != nil {
        cfg.AutoRelease = *globalCfg.AutoRelease
        sources.AutoRelease = "global"
    }
    if globalCfg.DirtyFileThreshold != nil {
        cfg.DirtyFileThreshold = *globalCfg.DirtyFileThreshold
        sources.DirtyFileThreshold = "global"
    }
    if globalCfg.Model != nil {
        cfg.Model = *globalCfg.Model
        sources.Model = "global"
    }
    // MaxContainers source rule mirrors existing factory.LogEffectiveConfig logic:
    // global wins only when project's MaxContainers is the zero-value (0).
    // Apply global value; the project-layer step below may override.
    if globalCfg.MaxContainers > 0 {
        cfg.MaxContainers = globalCfg.MaxContainers
        sources.MaxContainers = "global"
    }
}
```

## 5. Add `updateSourcesFromProject`

```go
// updateSourcesFromProject records "project" as the source for each layered
// field that the project's partial config explicitly set.
func updateSourcesFromProject(sources *ResolutionSources, partial *partialConfig) {
    if partial.HideGit != nil {
        sources.HideGit = "project"
    }
    if partial.AutoRelease != nil {
        sources.AutoRelease = "project"
    }
    if partial.DirtyFileThreshold != nil {
        sources.DirtyFileThreshold = "project"
    }
    if partial.Model != nil {
        sources.Model = "project"
    }
    if partial.MaxContainers != nil && *partial.MaxContainers > 0 {
        sources.MaxContainers = "project"
    }
}
```

The existing `mergePartial(&cfg, &partial)` already applies the values themselves — `updateSourcesFromProject` only updates the source map.

## 6. Tighten `Config.Validate` to reject empty model

Find the `Validate` method in `pkg/config/config.go`. There may already be a `validateModel` helper or inline check via `validation.NotEmptyString(c.Model)`; if so, ensure that an explicit empty `model: ""` set via yaml fails validation today.

If today's validation accepts `model: ""` (the partial loader sets it to empty string when explicitly written, and `Defaults()` provides a non-empty default that gets overridden), add or extend the model check:

```go
validation.Name("model", validation.HasValidationFunc(func(ctx context.Context) error {
    if c.Model == "" {
        return errors.Errorf(ctx, "model must not be empty")
    }
    if !globalconfig.ModelRegex.MatchString(c.Model) {
        return errors.Errorf(ctx, "model %q does not match required pattern %s", c.Model, globalconfig.ModelPattern)
    }
    return nil
})),
```

For the regex match to work cross-package, expose `modelRegex` from `pkg/globalconfig` as exported `ModelRegex` (already created in prompt 1 as unexported — promote it to exported here):

In `pkg/globalconfig/globalconfig.go`:

```go
// ModelRegex is the compiled regex for ModelPattern. Exported for use by
// pkg/config validation (same rule applies uniformly to global, project, and arg layers).
var ModelRegex = regexp.MustCompile(ModelPattern)
```

Replace the existing `modelRegex` (unexported) with `ModelRegex` and update the `Validate` method in globalconfig to use `ModelRegex.MatchString`.

## 7. Update all `Loader.Load` callers

The signature change from `Load(ctx)` to `Load(ctx, globalCfg)` ripples to:

### 7a. `main.go`

Find the project config loading site (search `config.NewLoader()`). Update the call to load global first, then pass it:

```go
globalCfg, err := globalconfig.NewLoader().Load(ctx)
if err != nil {
    return errors.Wrap(ctx, err, "load global config")
}

cfg, sources, err := config.NewLoader().Load(ctx, globalCfg)
if err != nil {
    return errors.Wrap(ctx, err, "load project config")
}
```

If the existing code already loads `globalCfg` separately, reuse the existing variable. Search for `globalconfig.NewLoader().Load(ctx)` to find the existing loading site (~line 520).

The `sources` value flows from `main.go` into the factory layer — pass it to `factory.CreateRunner` / `factory.CreateOneShotRunner` if `LogEffectiveConfig` is invoked from the factory's startup-logger closure. Alternatively, pass it directly to `LogEffectiveConfig`. Choose whichever yields the smallest diff — see step 8.

### 7b. `pkg/factory/factory.go`

`createStartupLogger` (~line 130) currently takes `cfg`, `globalCfg`, and uses `globalconfig.FileExists` to compute one source. Replace this with: take a `sources config.ResolutionSources` parameter and use those values directly. The `globalconfig.FileExists` call and the inline source-determination logic in `LogEffectiveConfig` can be deleted — the loader now does this work.

### 7c. Tests

`pkg/config/config_loader_test.go` and any test that calls `loader.Load(ctx)` must update to `loader.Load(ctx, globalconfig.GlobalConfig{})` (zero-value global = no global config). Use Ginkgo's `BeforeEach` if many tests share setup.

The counterfeiter mock at `mocks/config-loader.go` regenerates with the new signature via `make generate`.

## 8. Update `LogEffectiveConfig`

Refactor `pkg/factory/factory.go` `LogEffectiveConfig` to take `sources config.ResolutionSources` and emit per-field source attributes:

```go
func LogEffectiveConfig(cfg config.Config, sources config.ResolutionSources) {
    slog.Info("effective config",
        "maxContainers", cfg.MaxContainers,
        "maxContainersSource", sources.MaxContainers,
        "containerImage", cfg.ContainerImage,
        "model", cfg.Model,
        "modelSource", sources.Model,
        "workflow", cfg.Workflow,
        "pr", cfg.PR,
        "autoRelease", cfg.AutoRelease,
        "autoReleaseSource", sources.AutoRelease,
        "autoMerge", cfg.AutoMerge,
        "verificationGate", cfg.VerificationGate,
        "validationCommand", cfg.ValidationCommand,
        "testCommand", cfg.TestCommand,
        "debounceMs", cfg.DebounceMs,
        "hideGit", cfg.HideGit,
        "hideGitSource", sources.HideGit,
        "dirtyFileThreshold", cfg.DirtyFileThreshold,
        "dirtyFileThresholdSource", sources.DirtyFileThreshold,
        "promptsInboxDir", cfg.Prompts.InboxDir,
        "promptsInProgressDir", cfg.Prompts.InProgressDir,
        "promptsCompletedDir", cfg.Prompts.CompletedDir,
        "promptsLogDir", cfg.Prompts.LogDir,
        "preflightCommand", cfg.PreflightCommand,
        "preflightInterval", cfg.PreflightInterval,
    )
}
```

`EffectiveMaxContainers` and the inline source logic (~lines 89-100) is now dead — `MaxContainers` value and `MaxContainersSource` come from the loader. Delete `EffectiveMaxContainers` and its tests if no other callers remain. Verify with:

```bash
grep -rn "EffectiveMaxContainers" --include='*.go'
```

If callers exist, leave the function but mark its usage redundant via comment.

`createStartupLogger` simplifies to a closure that holds `cfg, sources` and calls `LogEffectiveConfig`.

## 9. Update tests in `pkg/config/config_loader_test.go`

Add new test cases (Ginkgo/Gomega) that exercise layered precedence:

### 9a. Default → Global precedence

- Given `globalCfg.HideGit = ptrTo(true)`, no project file → `cfg.HideGit == true`, `sources.HideGit == "global"`
- Given `globalCfg.Model = ptrTo("claude-opus-4-7")`, no project file → `cfg.Model == "claude-opus-4-7"`, `sources.Model == "global"`

### 9b. Project wins when set

- Given `globalCfg.Model = ptrTo("claude-opus-4-7")`, project yaml `model: claude-sonnet-4-6` → `cfg.Model == "claude-sonnet-4-6"`, `sources.Model == "project"`
- Given `globalCfg.HideGit = ptrTo(true)`, project yaml `hideGit: false` → `cfg.HideGit == false`, `sources.HideGit == "project"` (explicit false wins)

### 9c. Defaults when neither set

- Given empty `globalCfg{}`, no project file → all four sources == "default"

### 9d. Validation rejects empty model uniformly

- Given project yaml `model: ""` → Load returns error mentioning "model"
- Given empty `globalCfg{Model: ptrTo("")}` (impossible in practice — globalconfig.Validate rejects this — but a regression guard) → covered by globalconfig tests

### 9e. Existing maxContainers precedence unchanged

- Given `globalCfg.MaxContainers = 5`, no project file → `cfg.MaxContainers == 5`, `sources.MaxContainers == "global"`
- Given `globalCfg.MaxContainers = 5`, project yaml `maxContainers: 7` → `cfg.MaxContainers == 7`, `sources.MaxContainers == "project"`
- Given `globalCfg.MaxContainers = 5`, project yaml `maxContainers: 0` (zero = "use whatever was inherited") → `cfg.MaxContainers == 5`, `sources.MaxContainers == "global"`

This last case mirrors today's behavior — verify it still works after the refactor.

Helper:

```go
func ptrTo[T any](v T) *T { return &v }
```

Or use a small per-type helper if generics are not idiomatic in this codebase — check existing test patterns.

## 10. Update `main.go` orchestration

Find where the existing config-loading sequence runs (around line 520). The diff is small:

Before:
```go
globalCfg, err := globalconfig.NewLoader().Load(ctx)
if err != nil { ... }
cfg, err := config.NewLoader().Load(ctx)
if err != nil { ... }
```

After:
```go
globalCfg, err := globalconfig.NewLoader().Load(ctx)
if err != nil { ... }
cfg, sources, err := config.NewLoader().Load(ctx, globalCfg)
if err != nil { ... }
```

Thread `sources` through to wherever `LogEffectiveConfig` is invoked (typically via `factory.CreateRunner` / `CreateOneShotRunner`'s startup logger closure). The factory functions need a new `sources config.ResolutionSources` parameter.

Update both `runRunCommand` and `runDaemonCommand` to pass `sources` to the factory.

Update `pkg/factory/factory_test.go` — existing call sites for `CreateRunner` and `CreateOneShotRunner` get a new arg `config.ResolutionSources{}` (zero-value sources are fine for existing tests; they assert behavior not log output).

## 11. CHANGELOG

Append to `## Unreleased` in `CHANGELOG.md`:

```
- feat: layered config precedence (default ← global ← project) for hideGit, autoRelease, dirtyFileThreshold, model — values set in ~/.dark-factory/config.yaml apply to projects that don't override them
- log: effective-config startup line now includes per-field source (e.g. `hideGitSource=global`, `modelSource=project`)
- breaking-minor: explicit empty `model: ""` in any yaml layer now fails validation (previously silently overrode the default to empty string); no project's existing config is affected unless it intentionally set the empty string
```

## 12. Run validation

```bash
cd /workspace
make generate
make precommit
```

Both must exit 0.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT touch `go.mod` / `go.sum` / `vendor/`.
- Do NOT add CLI flags in this prompt — that is the next prompt's scope.
- The `Loader` interface signature change is intentional and a breaking change to the loader API. Counterfeiter mocks regenerate via `make generate` — do not hand-edit `mocks/config-loader.go`.
- The merge function chain is reused; do NOT introduce a separate merge for each new field. Adding a 5th field later must be a one-line struct addition + one-line merge call.
- Validation runs once on the merged Config (existing behavior).
- Per-layer rejection (invalid yaml in global) still surfaces — global config is loaded first and validated by `globalconfig.Validate` before being passed to the project loader.
- Existing `maxContainers` precedence rule is unchanged: project value wins when `> 0`; otherwise global; otherwise default. Verify with the existing tests.
- `EffectiveMaxContainers` may be deleted if no callers remain. Do NOT silently leave it dead — either delete or document.
- Use `errors.Errorf` / `errors.Wrap` from `github.com/bborbe/errors` — never `fmt.Errorf`.
- The model regex `^[a-zA-Z0-9._:/-]{1,256}$` is shared via `globalconfig.ModelPattern` / `globalconfig.ModelRegex` — do NOT duplicate the regex string in `pkg/config`.
- `main.go` may have multiple sites that load configs (run, daemon, status, list, etc.). Find them all with `grep -n "config.NewLoader" main.go` and update each.
- Tests use Ginkgo/Gomega in external `_test` packages, plus the standard `testing` package for table-driven cases — follow existing patterns.
- The `//nolint:funlen` annotations on existing functions must be preserved.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
# Loader signature changed
grep -n "Load(ctx context.Context, globalCfg globalconfig.GlobalConfig)" pkg/config/loader.go

# ResolutionSources defined
grep -n "type ResolutionSources" pkg/config/

# applyGlobal exists and references all 4 new fields
grep -n "applyGlobal\|globalCfg.HideGit\|globalCfg.AutoRelease\|globalCfg.DirtyFileThreshold\|globalCfg.Model" pkg/config/loader.go

# updateSourcesFromProject exists
grep -n "updateSourcesFromProject" pkg/config/loader.go

# All Loader.Load callers updated
grep -rn "Load(ctx)" pkg/ main.go --include='*.go' | grep -v "_test\|globalconfig\|prompt\|spec" || echo "no stale 1-arg Load calls"

# LogEffectiveConfig now uses sources
grep -n "hideGitSource\|modelSource\|autoReleaseSource\|dirtyFileThresholdSource" pkg/factory/factory.go

# Model regex shared
grep -n "globalconfig.ModelRegex\|globalconfig.ModelPattern" pkg/config/

# Empty model rejected
grep -n "model must not be empty" pkg/config/

# Mock regenerated
git diff --stat mocks/config-loader.go
```

```bash
make generate
make precommit
```
</verification>
