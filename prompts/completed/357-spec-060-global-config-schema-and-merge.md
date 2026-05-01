---
status: completed
spec: [060-config-layering-phase-1]
container: dark-factory-357-spec-060-global-config-schema-and-merge
dark-factory-version: dev
created: "2026-05-01T09:00:00Z"
queued: "2026-05-01T09:19:24Z"
started: "2026-05-01T09:22:25Z"
completed: "2026-05-01T09:43:42Z"
branch: dark-factory/config-layering-phase-1
---

<summary>
- `~/.dark-factory/config.yaml` now accepts `hideGit`, `autoRelease`, `dirtyFileThreshold`, and `model` alongside the existing `maxContainers`
- When global config sets `model: claude-opus-4-7` and the project config is silent on model, all prompts for that project run with opus
- When project config explicitly sets a field (even if the value equals the global value), the project value always wins
- When neither global nor project config sets a field, the hardcoded default applies — identical to today's behavior
- An invalid global config value (e.g. `dirtyFileThreshold: -5`, `model: ""`) fails startup with a clear error naming the file and field
- An absent global config file is silently skipped — identical to today's behavior
- The `effective config` log line emitted at startup now includes per-field source annotations (`modelSource=global`, `hideGitSource=project`, `autoReleaseSource=default`, etc.) for the 4 new layered fields
- Existing `maxContainers` global-to-project precedence is unchanged
- Operators with no global config file see identical behavior to today
- Model values are validated against a format regex at every layer; values with shell metacharacters (spaces, semicolons, pipes, etc.) are rejected
</summary>

<objective>
Extend `GlobalConfig` with 4 optional pointer fields (`hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`), implement the merge precedence chain (default ← global ← project) for those fields, expose source tracking via a `FieldSources` type, and thread the source information through to the `effective config` log line. Downstream consumers (factory, processor, executor) continue to receive a single final `Config` and never know layers existed.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter, no commits).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `docs/config-layering.md` — design background and 5-layer model for this feature.

Key files to read in full before editing:
- `pkg/globalconfig/globalconfig.go` — `GlobalConfig` struct, `Validate()`, `fileLoader.Load()` with inline partial struct
- `pkg/globalconfig/globalconfig_test.go` — existing Ginkgo/Gomega test style to follow
- `pkg/config/config.go` — `Config` struct, `Defaults()`, `Validate()` method; note existing `model` validation at line ~183
- `pkg/config/loader.go` — `fileLoader.Load()` with `partialConfig`, `mergePartial`, workflow mapping; `LoadWithOverrides` must reuse all existing helpers
- `pkg/config/config_loader_test.go` — existing loader test style; add `LoadWithOverrides` precedence tests here
- `main.go` — `run()` function (~line 39), `runCommand` signature (~line 122), `runRunCommand` (~line 191), `runDaemonCommand` (~line 222), `printConfig` (~line 519)
- `pkg/factory/factory.go` — `LogEffectiveConfig` (~line 81), `createStartupLogger` (~line 124), `CreateRunner` (~line 288), `CreateOneShotRunner` (~line 429)
- `pkg/factory/factory_test.go` — `LogEffectiveConfig` DescribeTable (~line 261) and `CreateRunner`/`CreateOneShotRunner` call sites

Spec that defines this work: `specs/in-progress/060-config-layering-phase-1.md`
</context>

<requirements>

## 1. Add 4 pointer fields to `GlobalConfig` (pkg/globalconfig/globalconfig.go)

Change the `GlobalConfig` struct to:

```go
type GlobalConfig struct {
	MaxContainers      int     `yaml:"maxContainers"`
	HideGit            *bool   `yaml:"hideGit,omitempty"`
	AutoRelease        *bool   `yaml:"autoRelease,omitempty"`
	DirtyFileThreshold *int    `yaml:"dirtyFileThreshold,omitempty"`
	Model              *string `yaml:"model,omitempty"`
}
```

Pointer fields (`*bool`, `*int`, `*string`) let callers distinguish "this field was set in the global config file" from "this field was absent". `nil` means absent. Non-nil means explicitly set.

## 2. Add model regex and update `GlobalConfig.Validate()` (pkg/globalconfig/globalconfig.go)

Add a package-level regex for model validation:

```go
// ModelPattern is the regex source string. Exported so callers can include
// it in error messages.
const ModelPattern = `^[a-zA-Z0-9._:/-]{1,256}$`

// ModelRegex validates model identifiers at every config layer (global, project, CLI arg).
// Permits Anthropic IDs (claude-opus-4-7), other-provider IDs (qwen3.6:35b-a3b),
// namespaced paths (local/qwen3.6:35b-a3b), and Docker image refs (docker.io/bborbe/claude-yolo:v0.6.1).
// Blocks shell metacharacters since model flows to container args.
// EXPORTED so pkg/config and main.go reuse the SAME compiled regex — do not duplicate the pattern.
var ModelRegex = regexp.MustCompile(ModelPattern)
```

Add `"regexp"` to imports.

Update `Validate()` to validate the 4 new fields:

```go
func (g GlobalConfig) Validate(ctx context.Context) error {
	if g.MaxContainers < 1 {
		return errors.Errorf(ctx, "globalconfig: maxContainers must be >= 1, got %d", g.MaxContainers)
	}
	if g.DirtyFileThreshold != nil && *g.DirtyFileThreshold < 0 {
		return errors.Errorf(ctx, "globalconfig: dirtyFileThreshold must not be negative, got %d", *g.DirtyFileThreshold)
	}
	if g.Model != nil {
		if *g.Model == "" {
			return errors.Errorf(ctx, "globalconfig: model must not be empty string when set")
		}
		if !ModelRegex.MatchString(*g.Model) {
			return errors.Errorf(ctx, "globalconfig: model %q does not match required pattern %s", *g.Model, ModelPattern)
		}
	}
	return nil
}
```

## 3. Update `fileLoader.Load()` to parse the 4 new fields (pkg/globalconfig/globalconfig.go)

The existing inline partial struct only has `MaxContainers *int`. Extend it to include the 4 new fields:

```go
var partial struct {
	MaxContainers      *int    `yaml:"maxContainers"`
	HideGit            *bool   `yaml:"hideGit"`
	AutoRelease        *bool   `yaml:"autoRelease"`
	DirtyFileThreshold *int    `yaml:"dirtyFileThreshold"`
	Model              *string `yaml:"model"`
}
```

After the `yaml.Unmarshal` call, copy non-nil pointer values into `cfg`:

```go
if partial.MaxContainers != nil {
	cfg.MaxContainers = *partial.MaxContainers
}
if partial.HideGit != nil {
	cfg.HideGit = partial.HideGit
}
if partial.AutoRelease != nil {
	cfg.AutoRelease = partial.AutoRelease
}
if partial.DirtyFileThreshold != nil {
	cfg.DirtyFileThreshold = partial.DirtyFileThreshold
}
if partial.Model != nil {
	cfg.Model = partial.Model
}
```

Note: `MaxContainers` is value-typed in `GlobalConfig` so it dereferences (`*partial.MaxContainers`). The other 4 fields are pointer-typed so we copy the pointer directly (e.g. `cfg.HideGit = partial.HideGit`).

## 4. Add model regex validation to `pkg/config/config.go`

Reuse the exported `globalconfig.ModelRegex` and `globalconfig.ModelPattern` — do NOT redefine the regex here. Single source of truth.

Add `"github.com/bborbe/dark-factory/pkg/globalconfig"` to imports of `pkg/config/config.go`. Confirm this introduces no import cycle: `pkg/globalconfig` does not import `pkg/config`, so `pkg/config → pkg/globalconfig` is a one-way dependency.

Replace the existing model validation line in `Validate()`:
```go
validation.Name("model", validation.NotEmptyString(c.Model)),
```
with:
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

This tightens model validation: explicit `model: ""` was already rejected (by `NotEmptyString`), and now model values with shell metacharacters are also rejected. The default model `claude-sonnet-4-6` passes this regex. Flag in CHANGELOG as a behavior tightening.

## 5. Add `FieldSources` type in new file `pkg/config/sources.go`

Create `/workspace/pkg/config/sources.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

// FieldSources records which config layer provided each of the 4 layered user-pref fields.
// Valid values for each field are: "default", "global", "project", "arg".
// Zero value (empty string) is treated the same as "default" by callers.
type FieldSources struct {
	HideGit            string
	AutoRelease        string
	DirtyFileThreshold string
	Model              string
}
```

## 6. Add `LayeredProjectOverrides` and `LoadResult` types, and `LoadWithOverrides` function (pkg/config/loader.go)

Add the following types and function to `pkg/config/loader.go`. Insert them after the existing type declarations but before `NewLoader`.

```go
// LayeredProjectOverrides reports which of the 4 layered user-pref fields were
// explicitly set in .dark-factory.yaml. nil means the field was absent from the file
// (so the default or global value applies). Non-nil means project explicitly set it.
type LayeredProjectOverrides struct {
	HideGit            *bool
	AutoRelease        *bool
	DirtyFileThreshold *int
	Model              *string
	MaxContainers      *int // included for completeness; maxContainers uses its own precedence path
}

// LoadResult bundles the merged project config with information about which
// of the 4 layered user-pref fields the project explicitly set.
type LoadResult struct {
	Config    Config
	Overrides LayeredProjectOverrides
}

// LoadWithOverrides reads .dark-factory.yaml, merges with defaults, validates,
// and returns the merged config plus project override detection data.
// Use this when global-config layering is needed (e.g. in main.run()).
// Existing callers that use NewLoader().Load() are unaffected.
func LoadWithOverrides(ctx context.Context) (LoadResult, error) {
	return (&fileLoader{configPath: ".dark-factory.yaml"}).loadWithOverrides(ctx)
}
```

Also change `fileLoader.Load()` to delegate to `loadWithOverrides()` to avoid code duplication:

```go
func (l *fileLoader) Load(ctx context.Context) (Config, error) {
	result, err := l.loadWithOverrides(ctx)
	if err != nil {
		return Config{}, err
	}
	return result.Config, nil
}
```

Add the private `loadWithOverrides()` method. It is identical to the current `Load()` body except:
- It returns `(LoadResult, error)` instead of `(Config, error)`
- After unmarshaling `partial`, it captures the 4 layered field pointers before calling `mergePartial`
- It returns both `cfg` and the captured overrides

```go
func (l *fileLoader) loadWithOverrides(ctx context.Context) (LoadResult, error) {
	// Start with defaults
	cfg := Defaults()

	// Try to read config file
	// #nosec G304 -- configPath is hardcoded, not user input
	data, err := os.ReadFile(l.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist - return defaults, no overrides
			return LoadResult{Config: cfg}, nil
		}
		return LoadResult{}, errors.Wrap(ctx, err, "read config file")
	}

	// Check file permissions
	fileInfo, statErr := os.Stat(l.configPath)
	if statErr != nil {
		slog.Warn("failed to stat config file", "path", l.configPath, "error", statErr)
	} else if fileInfo.Mode()&0004 != 0 {
		slog.Warn("config file is world-readable, consider: chmod 600", "path", l.configPath)
	}

	// Parse YAML into partial config
	var partial partialConfig
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return LoadResult{}, errors.Wrap(ctx, err, "parse config file")
	}

	// Capture the 4 layered user-pref fields before merging (pointer = explicitly set in project)
	overrides := LayeredProjectOverrides{
		HideGit:            partial.HideGit,
		AutoRelease:        partial.AutoRelease,
		DirtyFileThreshold: partial.DirtyFileThreshold,
		Model:              partial.Model,
		MaxContainers:      partial.MaxContainers,
	}

	// Merge non-nil values onto defaults
	mergePartial(&cfg, &partial)

	// Step A — workflow: pr legacy enum mapping
	if partial.Workflow != nil && *partial.Workflow == WorkflowPR {
		cfg.Workflow = WorkflowClone
		cfg.PR = true
		slog.Info(
			"'workflow: pr' is deprecated; use 'workflow: clone' with 'pr: true' instead",
			"resolved", "workflow: clone, pr: true",
		)
	} else if partial.Workflow != nil && partial.Worktree != nil {
		// Step B — new workflow value alongside legacy worktree: bool
		slog.Warn(
			"'worktree' is ignored when 'workflow' is set; remove 'worktree' from .dark-factory.yaml",
			"workflow", cfg.Workflow,
		)
	} else if partial.Workflow == nil && partial.Worktree != nil {
		// Step C — legacy worktree: bool mapping (no workflow field)
		switch {
		case !cfg.Worktree && !cfg.PR:
			cfg.Workflow = WorkflowDirect
			cfg.PR = false
			slog.Info(
				"'worktree' is deprecated in .dark-factory.yaml; use 'workflow' instead",
				"resolved_workflow", cfg.Workflow, "resolved_pr", cfg.PR,
			)
		case !cfg.Worktree && cfg.PR:
			cfg.Workflow = WorkflowBranch
			cfg.PR = true
			slog.Info(
				"'worktree' is deprecated in .dark-factory.yaml; use 'workflow' instead",
				"resolved_workflow", cfg.Workflow, "resolved_pr", cfg.PR,
			)
		case cfg.Worktree && cfg.PR:
			cfg.Workflow = WorkflowClone
			cfg.PR = true
			slog.Info(
				"'worktree' is deprecated in .dark-factory.yaml; use 'workflow' instead",
				"resolved_workflow", cfg.Workflow, "resolved_pr", cfg.PR,
			)
		case cfg.Worktree && !cfg.PR:
			cfg.Workflow = WorkflowClone
			cfg.PR = true
			slog.Info(
				"'worktree' is deprecated in .dark-factory.yaml; use 'workflow' instead",
				"resolved_workflow", cfg.Workflow, "resolved_pr", cfg.PR,
			)
			slog.Warn(
				"'worktree: true, pr: false' overrides pr to true for compatibility; set 'pr: true' explicitly to silence this warning",
			)
		}
	}
	// Step D — zero out cfg.Worktree unconditionally
	cfg.Worktree = false

	// Validate merged config
	if err := cfg.Validate(ctx); err != nil {
		return LoadResult{}, errors.Wrap(ctx, err, "validate config")
	}

	return LoadResult{Config: cfg, Overrides: overrides}, nil
}
```

**Important**: This is a near-verbatim copy of the existing `Load()` body. Copy it carefully, do NOT change the workflow mapping logic, permission check, or validation logic.

## 7. Update `main.go` — switch to `LoadWithOverrides` and apply global merge

### 7a. Update `run()` to load both configs and apply global merge

The current `run()` body does:
```go
loader := config.NewLoader()
cfg, err := loader.Load(ctx)
if err != nil {
    return err
}
```

Replace these 4 lines with:

```go
loadResult, err := config.LoadWithOverrides(ctx)
if err != nil {
    return err
}
cfg := loadResult.Config

globalCfg, err := globalconfig.NewLoader().Load(ctx)
if err != nil {
    return err
}
applyGlobalOverrides(&cfg, globalCfg, loadResult.Overrides)
sources := computeFieldSources(globalCfg, loadResult.Overrides)
```

Remove the `loader := config.NewLoader()` line (no longer needed).

### 7b. Update the `runCommand` call site in `run()`

The call to `runCommand` currently passes:
```go
return runCommand(ctx, cfg, command, subcommand, args, autoApprove, skipPreflight, currentDateTimeGetter)
```

Change to:
```go
return runCommand(ctx, cfg, command, subcommand, args, autoApprove, skipPreflight, sources, currentDateTimeGetter)
```

### 7c. Update `runCommand` signature

Add `sources config.FieldSources` after `skipPreflight bool`:

```go
func runCommand(
	ctx context.Context,
	cfg config.Config,
	command, subcommand string,
	args []string,
	autoApprove bool,
	skipPreflight bool,
	sources config.FieldSources,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
```

Update the `case "run":` and `case "daemon":` lines:
```go
case "run":
    return runRunCommand(ctx, cfg, args, autoApprove, skipPreflight, sources, currentDateTimeGetter)
case "daemon":
    return runDaemonCommand(ctx, cfg, args, skipPreflight, sources, currentDateTimeGetter)
```

No other changes to `runCommand` body.

### 7d. Update `runRunCommand` signature

Add `sources config.FieldSources` after `skipPreflight bool`:

```go
func runRunCommand(
	ctx context.Context,
	cfg config.Config,
	args []string,
	autoApprove bool,
	skipPreflight bool,
	sources config.FieldSources,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
```

Update the factory call:
```go
runErr := factory.CreateOneShotRunner(ctx, cfg, version.Version, autoApprove, skipPreflight, sources, currentDateTimeGetter).
    Run(ctx)
```

### 7e. Update `runDaemonCommand` signature

Add `sources config.FieldSources` after `skipPreflight bool`:

```go
func runDaemonCommand(
	ctx context.Context,
	cfg config.Config,
	args []string,
	skipPreflight bool,
	sources config.FieldSources,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
```

Update the factory call:
```go
runErr := factory.CreateRunner(ctx, cfg, version.Version, skipPreflight, sources, currentDateTimeGetter).
    Run(ctx)
```

### 7f. Add `applyGlobalOverrides` to `main.go`

Add this function at the bottom of `main.go` (before the last closing brace is not needed — just add after `extractMaxContainers`):

```go
// applyGlobalOverrides applies global config values for the 4 layered user-pref fields
// into cfg, but only where the project config did not explicitly set the field.
// Fields the project explicitly set (non-nil pointer in overrides) are left untouched.
func applyGlobalOverrides(cfg *config.Config, global globalconfig.GlobalConfig, proj config.LayeredProjectOverrides) {
	if global.Model != nil && proj.Model == nil {
		cfg.Model = *global.Model
	}
	if global.HideGit != nil && proj.HideGit == nil {
		cfg.HideGit = *global.HideGit
	}
	if global.AutoRelease != nil && proj.AutoRelease == nil {
		cfg.AutoRelease = *global.AutoRelease
	}
	if global.DirtyFileThreshold != nil && proj.DirtyFileThreshold == nil {
		cfg.DirtyFileThreshold = *global.DirtyFileThreshold
	}
}
```

### 7g. Add `computeFieldSources` to `main.go`

```go
// computeFieldSources determines which config layer provided each of the 4 layered user-pref fields.
// Rules: global wins over default; project wins over global.
// "arg" source is not set here — it is set in run commands when CLI flags override the value.
func computeFieldSources(global globalconfig.GlobalConfig, proj config.LayeredProjectOverrides) config.FieldSources {
	s := config.FieldSources{
		HideGit:            "default",
		AutoRelease:        "default",
		DirtyFileThreshold: "default",
		Model:              "default",
	}
	if global.Model != nil {
		s.Model = "global"
	}
	if global.HideGit != nil {
		s.HideGit = "global"
	}
	if global.AutoRelease != nil {
		s.AutoRelease = "global"
	}
	if global.DirtyFileThreshold != nil {
		s.DirtyFileThreshold = "global"
	}
	// Project overrides global (project wins)
	if proj.Model != nil {
		s.Model = "project"
	}
	if proj.HideGit != nil {
		s.HideGit = "project"
	}
	if proj.AutoRelease != nil {
		s.AutoRelease = "project"
	}
	if proj.DirtyFileThreshold != nil {
		s.DirtyFileThreshold = "project"
	}
	return s
}
```

## 8. Update `pkg/factory/factory.go`

### 8a. Update `LogEffectiveConfig` signature and body

Add `sources config.FieldSources` as the last parameter (before the closing paren):

```go
func LogEffectiveConfig(
	cfg config.Config,
	globalCfg globalconfig.GlobalConfig,
	globalFilePresent bool,
	sources config.FieldSources,
) {
```

In the `slog.Info` call, add 8 new key-value pairs after `"maxContainersSource", source`:

```go
slog.Info("effective config",
    "maxContainers", effective,
    "maxContainersSource", source,
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
```

Preserve all existing key-value pairs in the same order. Only add the 4 `*Source` entries adjacent to the field they describe.

### 8b. Update `createStartupLogger` signature

Add `sources config.FieldSources` as a parameter:

```go
func createStartupLogger(
	ctx context.Context,
	cfg config.Config,
	globalCfg globalconfig.GlobalConfig,
	sources config.FieldSources,
) func() {
	present, _ := globalconfig.FileExists(ctx)
	return func() { LogEffectiveConfig(cfg, globalCfg, present, sources) }
}
```

### 8c. Update `CreateRunner` signature

Add `sources config.FieldSources` after `skipPreflight bool`:

```go
func CreateRunner(
	ctx context.Context,
	cfg config.Config,
	ver string,
	skipPreflight bool,
	sources config.FieldSources,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) runner.Runner {
```

Update the `createStartupLogger` call (near the bottom of `CreateRunner`):
```go
createStartupLogger(ctx, cfg, globalCfg, sources),
```

### 8d. Update `CreateOneShotRunner` signature

Add `sources config.FieldSources` after `skipPreflight bool`:

```go
func CreateOneShotRunner(
	ctx context.Context,
	cfg config.Config,
	ver string,
	autoApprove bool,
	skipPreflight bool,
	sources config.FieldSources,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) runner.OneShotRunner {
```

Update the `createStartupLogger` call inside `CreateOneShotRunner` in the same way.

Find it with:
```bash
grep -n "createStartupLogger" pkg/factory/factory.go
```

Both calls need the `sources` argument added.

## 9. Update `pkg/factory/factory_test.go`

### 9a. Update `LogEffectiveConfig` call sites

Search for all calls:
```bash
grep -n "factory.LogEffectiveConfig" pkg/factory/factory_test.go
```

Each call currently ends with `globalFilePresent)`. Add `config.FieldSources{}` before the closing paren:

```go
factory.LogEffectiveConfig(c, globalCfg, globalFilePresent, config.FieldSources{})
```

Add `"github.com/bborbe/dark-factory/pkg/config"` to the import block if not already present (it is already imported).

### 9b. Update `CreateRunner` call sites

Search:
```bash
grep -n "factory.CreateRunner" pkg/factory/factory_test.go
```

Add `config.FieldSources{}` before `libtime.NewCurrentDateTime()`:

```go
factory.CreateRunner(context.Background(), cfg, "v0.0.1", false, config.FieldSources{}, libtime.NewCurrentDateTime())
```

### 9c. Update `CreateOneShotRunner` call sites

Search:
```bash
grep -n "factory.CreateOneShotRunner" pkg/factory/factory_test.go
```

Add `config.FieldSources{}` before `libtime.NewCurrentDateTime()`:

```go
factory.CreateOneShotRunner(ctx, c, "v0.0.1", false, false, config.FieldSources{}, libtime.NewCurrentDateTime()).Run(ctx)
```

### 9d. Add source assertions to the `LogEffectiveConfig` DescribeTable

In the `assertRequiredFields` function, add assertions for the 4 new source keys:

```go
assertRequiredFields := func(output string) {
    // ... existing assertions ...
    Expect(output).To(ContainSubstring("modelSource="))
    Expect(output).To(ContainSubstring("hideGitSource="))
    Expect(output).To(ContainSubstring("autoReleaseSource="))
    Expect(output).To(ContainSubstring("dirtyFileThresholdSource="))
}
```

## 10. Update `main_internal_test.go`

The `runCommand` Describe block tests use `runCommand(ctx, config.Config{}, command, "", []string{}, false, true, dt)`.

Add `config.FieldSources{}` between `true` and `dt`:

```go
err := runCommand(ctx, config.Config{}, command, "", []string{}, false, true, config.FieldSources{}, dt)
```

Search for all `runCommand(` calls in `main_internal_test.go`:
```bash
grep -n "runCommand(" main_internal_test.go
```

Update each one to include `config.FieldSources{}` in the correct position.

## 11. Add tests for `GlobalConfig.Validate()` (pkg/globalconfig/globalconfig_test.go)

Add these `It` blocks to the existing `Describe("GlobalConfig.Validate", ...)` block:

```go
It("returns nil when HideGit is set to true", func() {
    t := true
    cfg := globalconfig.GlobalConfig{MaxContainers: 3, HideGit: &t}
    Expect(cfg.Validate(ctx)).To(Succeed())
})

It("returns nil when HideGit is set to false", func() {
    f := false
    cfg := globalconfig.GlobalConfig{MaxContainers: 3, HideGit: &f}
    Expect(cfg.Validate(ctx)).To(Succeed())
})

It("returns nil when DirtyFileThreshold is zero", func() {
    z := 0
    cfg := globalconfig.GlobalConfig{MaxContainers: 3, DirtyFileThreshold: &z}
    Expect(cfg.Validate(ctx)).To(Succeed())
})

It("returns error when DirtyFileThreshold is negative", func() {
    n := -1
    cfg := globalconfig.GlobalConfig{MaxContainers: 3, DirtyFileThreshold: &n}
    Expect(cfg.Validate(ctx)).To(HaveOccurred())
    Expect(cfg.Validate(ctx).Error()).To(ContainSubstring("dirtyFileThreshold"))
})

It("returns nil when Model is a valid Anthropic ID", func() {
    m := "claude-opus-4-7"
    cfg := globalconfig.GlobalConfig{MaxContainers: 3, Model: &m}
    Expect(cfg.Validate(ctx)).To(Succeed())
})

It("returns nil when Model contains colon and slash (Docker image ref)", func() {
    m := "docker.io/bborbe/claude-yolo:v0.6.1"
    cfg := globalconfig.GlobalConfig{MaxContainers: 3, Model: &m}
    Expect(cfg.Validate(ctx)).To(Succeed())
})

It("returns error when Model is empty string", func() {
    empty := ""
    cfg := globalconfig.GlobalConfig{MaxContainers: 3, Model: &empty}
    Expect(cfg.Validate(ctx)).To(HaveOccurred())
    Expect(cfg.Validate(ctx).Error()).To(ContainSubstring("model"))
})

It("returns error when Model contains semicolons (shell metachar)", func() {
    bad := "claude;rm -rf /"
    cfg := globalconfig.GlobalConfig{MaxContainers: 3, Model: &bad}
    Expect(cfg.Validate(ctx)).To(HaveOccurred())
    Expect(cfg.Validate(ctx).Error()).To(ContainSubstring("model"))
})

It("returns nil when all 4 new fields are nil (not set)", func() {
    cfg := globalconfig.GlobalConfig{MaxContainers: 3}
    Expect(cfg.Validate(ctx)).To(Succeed())
})
```

## 12. Add tests for `LoadWithOverrides` precedence (pkg/config/config_loader_test.go)

Find the existing `Describe("Config", ...)` block in `config_loader_test.go`. Inside the `Describe("Load", ...)` inner block (or wherever the loader tests live), add a new `Describe("LoadWithOverrides", ...)` block:

```go
Describe("LoadWithOverrides", func() {
    var tmpDir string

    BeforeEach(func() {
        var err error
        tmpDir, err = os.MkdirTemp("", "load-with-overrides-test-*")
        Expect(err).NotTo(HaveOccurred())
        origDir, err := os.Getwd()
        Expect(err).NotTo(HaveOccurred())
        DeferCleanup(func() {
            Expect(os.Chdir(origDir)).To(Succeed())
            Expect(os.RemoveAll(tmpDir)).To(Succeed())
        })
        Expect(os.Chdir(tmpDir)).To(Succeed())
    })

    It("returns LoadResult with defaults when config file is absent", func() {
        result, err := config.LoadWithOverrides(ctx)
        Expect(err).NotTo(HaveOccurred())
        Expect(result.Config.Model).To(Equal("claude-sonnet-4-6"))
        Expect(result.Overrides.Model).To(BeNil())
        Expect(result.Overrides.HideGit).To(BeNil())
        Expect(result.Overrides.AutoRelease).To(BeNil())
        Expect(result.Overrides.DirtyFileThreshold).To(BeNil())
    })

    It("detects model explicitly set in project config", func() {
        err := os.WriteFile(".dark-factory.yaml", []byte("model: claude-opus-4-7\n"), 0600)
        Expect(err).NotTo(HaveOccurred())
        result, err := config.LoadWithOverrides(ctx)
        Expect(err).NotTo(HaveOccurred())
        Expect(result.Config.Model).To(Equal("claude-opus-4-7"))
        Expect(result.Overrides.Model).NotTo(BeNil())
        Expect(*result.Overrides.Model).To(Equal("claude-opus-4-7"))
    })

    It("reports nil override when model is not in project config (uses default)", func() {
        err := os.WriteFile(".dark-factory.yaml", []byte("workflow: direct\n"), 0600)
        Expect(err).NotTo(HaveOccurred())
        result, err := config.LoadWithOverrides(ctx)
        Expect(err).NotTo(HaveOccurred())
        Expect(result.Overrides.Model).To(BeNil())
    })

    It("detects hideGit explicitly set to false", func() {
        err := os.WriteFile(".dark-factory.yaml", []byte("hideGit: false\n"), 0600)
        Expect(err).NotTo(HaveOccurred())
        result, err := config.LoadWithOverrides(ctx)
        Expect(err).NotTo(HaveOccurred())
        Expect(result.Overrides.HideGit).NotTo(BeNil())
        Expect(*result.Overrides.HideGit).To(BeFalse())
    })

    It("detects autoRelease explicitly set to true", func() {
        err := os.WriteFile(".dark-factory.yaml", []byte("autoRelease: true\n"), 0600)
        Expect(err).NotTo(HaveOccurred())
        result, err := config.LoadWithOverrides(ctx)
        Expect(err).NotTo(HaveOccurred())
        Expect(result.Overrides.AutoRelease).NotTo(BeNil())
        Expect(*result.Overrides.AutoRelease).To(BeTrue())
    })

    It("detects dirtyFileThreshold explicitly set to zero", func() {
        err := os.WriteFile(".dark-factory.yaml", []byte("dirtyFileThreshold: 0\n"), 0600)
        Expect(err).NotTo(HaveOccurred())
        result, err := config.LoadWithOverrides(ctx)
        Expect(err).NotTo(HaveOccurred())
        Expect(result.Overrides.DirtyFileThreshold).NotTo(BeNil())
        Expect(*result.Overrides.DirtyFileThreshold).To(Equal(0))
    })
})
```

Also add a `Describe` block for the model regex validation in the existing `Config.Validate` tests:

```go
Describe("model validation", func() {
    It("rejects model with semicolon", func() {
        cfg := config.Defaults()
        cfg.Model = "claude;rm -rf /"
        Expect(cfg.Validate(ctx)).To(HaveOccurred())
        Expect(cfg.Validate(ctx).Error()).To(ContainSubstring("model"))
    })

    It("accepts claude-sonnet-4-6 (default)", func() {
        cfg := config.Defaults()
        Expect(cfg.Validate(ctx)).To(Succeed())
    })

    It("accepts Docker image ref format", func() {
        cfg := config.Defaults()
        cfg.Model = "docker.io/bborbe/claude-yolo:v0.6.1"
        Expect(cfg.Validate(ctx)).To(Succeed())
    })

    It("rejects empty model string", func() {
        cfg := config.Defaults()
        cfg.Model = ""
        Expect(cfg.Validate(ctx)).To(HaveOccurred())
        Expect(cfg.Validate(ctx).Error()).To(ContainSubstring("model"))
    })
})
```

Determine the best location for these tests by reading `pkg/config/config_test.go` to see where existing validate tests are placed.

## 13. Add tests for `applyGlobalOverrides` and `computeFieldSources` (main_internal_test.go)

Add a new `Describe` block in `main_internal_test.go` to test the two new helper functions:

```go
var _ = Describe("applyGlobalOverrides", func() {
    var ctx context.Context
    BeforeEach(func() { ctx = context.Background() })
    _ = ctx // suppress unused warning if needed

    It("applies global model when project did not set it", func() {
        cfg := config.Defaults()
        global := globalconfig.GlobalConfig{MaxContainers: 3}
        m := "claude-opus-4-7"
        global.Model = &m
        proj := config.LayeredProjectOverrides{}
        applyGlobalOverrides(&cfg, global, proj)
        Expect(cfg.Model).To(Equal("claude-opus-4-7"))
    })

    It("does not overwrite project model with global model", func() {
        cfg := config.Defaults()
        cfg.Model = "claude-sonnet-4-6"
        global := globalconfig.GlobalConfig{MaxContainers: 3}
        gm := "claude-opus-4-7"
        global.Model = &gm
        pm := "claude-sonnet-4-6"
        proj := config.LayeredProjectOverrides{Model: &pm}
        applyGlobalOverrides(&cfg, global, proj)
        Expect(cfg.Model).To(Equal("claude-sonnet-4-6"))
    })

    It("applies global hideGit when project did not set it", func() {
        cfg := config.Defaults()
        global := globalconfig.GlobalConfig{MaxContainers: 3}
        t := true
        global.HideGit = &t
        proj := config.LayeredProjectOverrides{}
        applyGlobalOverrides(&cfg, global, proj)
        Expect(cfg.HideGit).To(BeTrue())
    })

    It("does not overwrite project hideGit=false with global hideGit=true", func() {
        cfg := config.Defaults()
        cfg.HideGit = false
        global := globalconfig.GlobalConfig{MaxContainers: 3}
        t := true
        global.HideGit = &t
        f := false
        proj := config.LayeredProjectOverrides{HideGit: &f}
        applyGlobalOverrides(&cfg, global, proj)
        Expect(cfg.HideGit).To(BeFalse())
    })
})

var _ = Describe("computeFieldSources", func() {
    It("returns default for all fields when global and project both absent", func() {
        global := globalconfig.GlobalConfig{MaxContainers: 3}
        proj := config.LayeredProjectOverrides{}
        s := computeFieldSources(global, proj)
        Expect(s.Model).To(Equal("default"))
        Expect(s.HideGit).To(Equal("default"))
        Expect(s.AutoRelease).To(Equal("default"))
        Expect(s.DirtyFileThreshold).To(Equal("default"))
    })

    It("returns global when global sets model and project does not", func() {
        global := globalconfig.GlobalConfig{MaxContainers: 3}
        m := "claude-opus-4-7"
        global.Model = &m
        proj := config.LayeredProjectOverrides{}
        s := computeFieldSources(global, proj)
        Expect(s.Model).To(Equal("global"))
    })

    It("returns project when project sets model (even if global also set)", func() {
        global := globalconfig.GlobalConfig{MaxContainers: 3}
        gm := "claude-opus-4-7"
        global.Model = &gm
        pm := "claude-haiku-4-5"
        proj := config.LayeredProjectOverrides{Model: &pm}
        s := computeFieldSources(global, proj)
        Expect(s.Model).To(Equal("project"))
    })

    It("returns project for hideGit when project explicitly sets false", func() {
        global := globalconfig.GlobalConfig{MaxContainers: 3}
        f := false
        proj := config.LayeredProjectOverrides{HideGit: &f}
        s := computeFieldSources(global, proj)
        Expect(s.HideGit).To(Equal("project"))
    })
})
```

Add necessary imports to `main_internal_test.go` if not already present:
```go
"github.com/bborbe/dark-factory/pkg/config"
"github.com/bborbe/dark-factory/pkg/globalconfig"
```

## 14. Write CHANGELOG entry

Add or extend `## Unreleased` at the top of `CHANGELOG.md`:

```
- feat: extend global config (~/.dark-factory/config.yaml) with hideGit, autoRelease, dirtyFileThreshold, model fields; implement default←global←project merge precedence for these 4 fields
- feat: effective config log line now shows per-field source annotations (modelSource=global, hideGitSource=project, etc.) for the 4 new layered fields
- fix: tighten model validation: explicit model="" rejected at every config layer; model values with shell metacharacters rejected (BREAKING: rare — no known valid model names contain these chars)
```

## 15. Run `make test`

```bash
cd /workspace && make test
```

All tests must pass before running `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- The `config.Loader` interface (`Load(ctx) (Config, error)`) must remain unchanged — the new `LoadWithOverrides` is a separate package-level function, not an interface method
- `maxContainers` precedence (factory loads global internally, uses `EffectiveMaxContainers`) must remain unchanged — do NOT change how maxContainers is handled
- All 4 new `GlobalConfig` fields must use pointer types (`*bool`, `*int`, `*string`) — they must be `nil` when absent from the global config file
- The `loadWithOverrides()` method must be an exact copy of the existing `Load()` body with only two additions: (1) capture overrides before `mergePartial`, (2) return `LoadResult` instead of `Config`. Do NOT change the workflow mapping logic, permission check, or validation call.
- `LogEffectiveConfig` must preserve all existing key-value pairs in the same positions — only add the 4 `*Source` entries
- The `//nolint:funlen` annotation on `CreateRunner` and `CreateOneShotRunner` in `factory.go` must be preserved
- Use `errors.Errorf(ctx, ...)` and `errors.Wrap(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- All tests use Ginkgo/Gomega in external `_test` packages
- model regex `^[a-zA-Z0-9._:/-]{1,256}$` must be defined independently in both `pkg/globalconfig` and `pkg/config` (no cross-package import to share it); duplication is intentional
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "HideGit\|AutoRelease\|DirtyFileThreshold\|Model " pkg/globalconfig/globalconfig.go` — should find the 4 new pointer fields in GlobalConfig struct, inline partial struct, and Validate()
2. `grep -n "LoadWithOverrides\|LayeredProjectOverrides\|LoadResult" pkg/config/loader.go` — should find the new types and function
3. `grep -n "FieldSources" pkg/config/sources.go` — should find the struct definition
4. `grep -n "applyGlobalOverrides\|computeFieldSources" main.go` — should find the two helper function definitions
5. `grep -n "sources config.FieldSources" pkg/factory/factory.go` — should find parameters in CreateRunner, CreateOneShotRunner, createStartupLogger, and LogEffectiveConfig
6. `grep -n "modelSource\|hideGitSource\|autoReleaseSource\|dirtyFileThresholdSource" pkg/factory/factory.go` — should find 4 new slog key-value pairs in LogEffectiveConfig
7. `go build ./...` — must compile with no errors
8. `go test ./pkg/globalconfig/... ./pkg/config/... ./pkg/factory/...` — all tests pass
</verification>
