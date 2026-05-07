---
status: committing
spec: [078-auto-approve-generated-prompts]
summary: Added autoApprovePrompts boolean setting across all config layers (GlobalConfig, Config, partialConfig, LayeredProjectOverrides, FieldSources), wired --auto-approve-prompts CLI flag into runDaemonCommand and runRunCommand, and emits autoApprovePrompts/autoApprovePromptsSource in LogEffectiveConfig at daemon startup.
container: dark-factory-385-spec-078-config
dark-factory-version: v0.154.0
created: "2026-05-07T22:00:00Z"
queued: "2026-05-07T21:50:05Z"
started: "2026-05-07T21:51:28Z"
branch: dark-factory/auto-approve-generated-prompts
---

<summary>
- A new `autoApprovePrompts` boolean setting is added to the dark-factory configuration system
- The setting defaults to `false` and is resolvable from three layers: global config (`~/.dark-factory/config.yaml`), project config (`.dark-factory.yaml`), and CLI flag (`--auto-approve-prompts`)
- Precedence is CLI > project > global > default, identical to the `maxContainers` pattern
- A `*bool` sentinel is used in both GlobalConfig and partialConfig to distinguish "unset" from `false`
- The daemon startup log emits `autoApprovePrompts=<true|false>` and `autoApprovePromptsSource=<default|global|project|arg>` alongside existing effective-config fields (matches the existing `maxContainersSource` convention which uses `"arg"` for CLI-supplied values)
- When the setting is absent everywhere, behavior is identical to today — no generated prompt is auto-approved
- `FieldSources` gains `AutoApprovePrompts string` so the source label can flow from CLI/global/project through to the log line
- `LayeredProjectOverrides` gains `AutoApprovePrompts *bool` so `computeFieldSources` can detect which layer set it
- `partialConfig` gains `AutoApprovePrompts *bool` so project-config parsing can detect explicit `false` vs absent
- All existing tests continue to pass; `make precommit` passes
</summary>

<objective>
Add the `autoApprovePrompts` boolean setting to every config layer (global file, project file, CLI flag) and wire it into `LogEffectiveConfig` so the daemon reports the effective value and its source at startup. This prompt delivers the full configuration plumbing. The generator integration that acts on the value comes in prompt 2.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `docs/config-layering.md` — this is the definitive guide for the layering pattern this prompt follows.

Files to read in full before editing:
- `pkg/globalconfig/globalconfig.go` — `GlobalConfig` struct, `defaults()`, `Load` (partial struct pattern, `HideGit *bool` is the template)
- `pkg/config/config.go` — `Config` struct, `Defaults()`, `Validate()` (add field after `DirtyFileThreshold`)
- `pkg/config/loader.go` — `partialConfig`, `LayeredProjectOverrides`, `loadWithOverrides`, `mergePartialLimits` (where `MaxContainers` and `DirtyFileThreshold` are merged)
- `pkg/config/sources.go` — `FieldSources` struct (add one field)
- `main.go` — `applyGlobalOverrides`, `computeFieldSources`, `extractMaxContainers` (template for `extractAutoApprovePrompts`), `runDaemonCommand`, `runRunCommand`
- `pkg/factory/factory.go` — `LogEffectiveConfig` (add two new key-value pairs near `hideGit` / `hideGitSource`)
- `pkg/config/config_loader_test.go` — understand test patterns before adding tests
- `pkg/globalconfig/globalconfig_test.go` — understand test patterns before adding tests
- `main_internal_test.go` — understand test patterns for `applyGlobalOverrides` / `computeFieldSources`
- `parse_args_test.go` — understand `extractMaxContainers` test patterns
- `pkg/factory/factory_test.go` — understand `LogEffectiveConfig` test patterns
</context>

<requirements>

## 1. Add `AutoApprovePrompts *bool` to `GlobalConfig` in `pkg/globalconfig/globalconfig.go`

### 1a. Struct field

Add `AutoApprovePrompts *bool` to `GlobalConfig` after `DirtyFileThreshold`:

```go
type GlobalConfig struct {
    MaxContainers      int     `yaml:"maxContainers"`
    HideGit            *bool   `yaml:"hideGit,omitempty"`
    AutoRelease        *bool   `yaml:"autoRelease,omitempty"`
    DirtyFileThreshold *int    `yaml:"dirtyFileThreshold,omitempty"`
    Model              *string `yaml:"model,omitempty"`
    AutoApprovePrompts *bool   `yaml:"autoApprovePrompts,omitempty"`  // NEW
}
```

### 1b. YAML parsing in `Load`

In the `partial` struct inside `Load`, add `AutoApprovePrompts *bool \`yaml:"autoApprovePrompts"\``.

After the existing `if partial.Model != nil` block, add:
```go
if partial.AutoApprovePrompts != nil {
    cfg.AutoApprovePrompts = partial.AutoApprovePrompts
}
```

### 1c. Validation — no change

`AutoApprovePrompts` is a boolean. All values are valid. Do NOT add any validation for it in `Validate()`.

### 1d. Defaults — no change

`defaults()` returns `GlobalConfig{MaxContainers: DefaultMaxContainers}`. The `AutoApprovePrompts` field defaults to `nil` (unset), meaning global config does not impose a value. Do NOT add it to `defaults()`.

## 2. Add `AutoApprovePrompts bool` to `Config` in `pkg/config/config.go`

In the `Config` struct, add after `DirtyFileThreshold`:

```go
AutoApprovePrompts bool `yaml:"autoApprovePrompts,omitempty"`
```

`omitempty` ensures the field is omitted from `dark-factory config` output when `false`.

Do NOT add to `Defaults()` — the zero value (`false`) is the correct default. Do NOT add any validation for this field in `Validate()`.

## 3. Update `pkg/config/loader.go`

### 3a. `partialConfig` — add field

Add after `DirtyFileThreshold *int`:

```go
AutoApprovePrompts *bool `yaml:"autoApprovePrompts"`
```

### 3b. `LayeredProjectOverrides` — add field

Add after `AutoMerge *bool`:

```go
AutoApprovePrompts *bool // non-nil when .dark-factory.yaml explicitly sets autoApprovePrompts
```

### 3c. Capture override in `loadWithOverrides`

In the `overrides := LayeredProjectOverrides{...}` block, add:

```go
AutoApprovePrompts: partial.AutoApprovePrompts,
```

### 3d. Merge in `mergePartialLimits`

In `mergePartialLimits`, add after the `AutoRetryLimit` block:

```go
if partial.AutoApprovePrompts != nil {
    cfg.AutoApprovePrompts = *partial.AutoApprovePrompts
}
```

## 4. Add `AutoApprovePrompts string` to `FieldSources` in `pkg/config/sources.go`

Add after `AutoMerge string`:

```go
AutoApprovePrompts string
```

## 5. Update `main.go`

### 5a. Update `applyGlobalOverrides`

Add a case for the new field (after the existing `DirtyFileThreshold` block):

```go
if global.AutoApprovePrompts != nil && proj.AutoApprovePrompts == nil {
    cfg.AutoApprovePrompts = *global.AutoApprovePrompts
}
```

### 5b. Update `computeFieldSources`

In `computeFieldSources`, after the `DirtyFileThreshold` blocks, add:

```go
// Global sets autoApprovePrompts
if global.AutoApprovePrompts != nil {
    s.AutoApprovePrompts = "global"
}
// Project overrides global
if proj.AutoApprovePrompts != nil {
    s.AutoApprovePrompts = "project"
}
```

Initialize `s.AutoApprovePrompts = "default"` in the initial `s := config.FieldSources{...}` block alongside the other default fields.

### 5c. Add `extractAutoApprovePrompts` helper

Add a new function immediately below `extractMaxContainers`:

```go
// extractAutoApprovePrompts removes --auto-approve-prompts from args and reports whether it was set.
// The flag is a presence flag: its appearance means true. No value argument is consumed.
func extractAutoApprovePrompts(args []string) (bool, []string) {
    for i, arg := range args {
        if arg != "--auto-approve-prompts" {
            continue
        }
        remaining := make([]string, 0, len(args)-1)
        remaining = append(remaining, args[:i]...)
        remaining = append(remaining, args[i+1:]...)
        return true, remaining
    }
    return false, args
}
```

### 5d. Apply in `runDaemonCommand`

In `runDaemonCommand`, after the `extractMaxContainers` call, add:

```go
autoApprovePrompts, remaining := extractAutoApprovePrompts(remaining)
if autoApprovePrompts {
    cfg.AutoApprovePrompts = true
    sources.AutoApprovePrompts = "arg"
}
```

Rename the `remaining` variable consistently — after extractMaxContainers already renamed args to `remaining`, chain into it here.

### 5e. Apply in `runRunCommand`

Same pattern as 5d — after the `extractMaxContainers` call in `runRunCommand`:

```go
autoApprovePrompts, remaining := extractAutoApprovePrompts(remaining)
if autoApprovePrompts {
    cfg.AutoApprovePrompts = true
    sources.AutoApprovePrompts = "arg"
}
```

## 6. Update `LogEffectiveConfig` in `pkg/factory/factory.go`

In the `slog.Info("effective config", ...)` call, add two key-value pairs immediately after `"hideGit", cfg.HideGit, "hideGitSource", sources.HideGit,`:

```go
"autoApprovePrompts", cfg.AutoApprovePrompts,
"autoApprovePromptsSource", sources.AutoApprovePrompts,
```

## 7. Add / update tests

### 7a. `pkg/config/config_loader_test.go`

Add a test case that writes a `.dark-factory.yaml` with `autoApprovePrompts: true` and asserts:
- `result.Config.AutoApprovePrompts == true`
- `result.Overrides.AutoApprovePrompts != nil && *result.Overrides.AutoApprovePrompts == true`

Also add a test that `autoApprovePrompts: false` in YAML results in:
- `result.Config.AutoApprovePrompts == false`
- `result.Overrides.AutoApprovePrompts != nil` (non-nil — explicitly set)

And a test that when `autoApprovePrompts` is absent:
- `result.Config.AutoApprovePrompts == false`
- `result.Overrides.AutoApprovePrompts == nil`

### 7b. `pkg/globalconfig/globalconfig_test.go`

Add a test that writes a `~/.dark-factory/config.yaml` (in a temp dir, mock `userHomeDir`) with `autoApprovePrompts: true`:
- `cfg.AutoApprovePrompts != nil && *cfg.AutoApprovePrompts == true`

And test that omitting the field returns `cfg.AutoApprovePrompts == nil`.

### 7c. `main_internal_test.go`

Add tests for `applyGlobalOverrides` and `computeFieldSources` covering `AutoApprovePrompts`:
- Global set (`*true`), project unset → cfg gets `true`, source = `"global"`
- Global set (`*true`), project set (`*false`) → cfg gets `false` (project wins), source = `"project"`
- Neither set → cfg stays `false`, source = `"default"`

### 7d. `parse_args_test.go`

Add tests for `extractAutoApprovePrompts`:
- `["--auto-approve-prompts"]` → returns `true`, empty remaining
- `["--auto-approve-prompts", "other"]` → returns `true`, `["other"]`
- `["other", "--auto-approve-prompts"]` → returns `true`, `["other"]`
- `[]` → returns `false`, empty remaining
- `["other"]` → returns `false`, `["other"]`

### 7e. `pkg/factory/factory_test.go`

Add a test that `LogEffectiveConfig` (called with a config where `AutoApprovePrompts = true` and `sources.AutoApprovePrompts = "project"`) does not panic and (if the test captures log output) includes `autoApprovePrompts=true` and `autoApprovePromptsSource=project`. Match the existing `LogEffectiveConfig` test structure.

## 8. Add CHANGELOG entry

Add under `## Unreleased` in `CHANGELOG.md`:

```markdown
- feat: Add `autoApprovePrompts` boolean setting resolvable from global config, project config, and `--auto-approve-prompts` CLI flag; effective value and source logged at daemon startup
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Default behavior (when `autoApprovePrompts` is unset everywhere) must be identical to today's behavior — `false` is the zero value, no code path changes when the flag is absent
- Use `*bool` sentinel in `GlobalConfig` and `partialConfig` to distinguish "unset" from `false` (same pattern as `HideGit`)
- `AutoApprovePrompts bool` (not pointer) in the main `Config` struct — the resolved/merged value is always a concrete bool
- Do NOT add `AutoApprovePrompts` to `Config.Defaults()` — the zero value `false` is the correct default
- Do NOT add validation for `AutoApprovePrompts` in `Config.Validate()` or `GlobalConfig.Validate()` — all boolean values are valid
- The CLI flag `--auto-approve-prompts` is a presence flag (no value argument) — presence means `true`; absence means "do not override layer below"
- The flag only applies to `run` and `daemon` commands, same as `--max-containers`
- Wrap all errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors` — no `fmt.Errorf`, no bare `return err`
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "AutoApprovePrompts" pkg/globalconfig/globalconfig.go` — struct field + yaml partial + copy to cfg (3 occurrences)
2. `grep -n "AutoApprovePrompts" pkg/config/config.go` — one occurrence in Config struct
3. `grep -n "AutoApprovePrompts" pkg/config/loader.go` — partialConfig + LayeredProjectOverrides + capture in overrides + mergePartialLimits (4 occurrences)
4. `grep -n "AutoApprovePrompts" pkg/config/sources.go` — one occurrence
5. `grep -n "AutoApprovePrompts\|auto-approve-prompts" main.go` — applyGlobalOverrides (2 lines) + computeFieldSources (3 lines) + extractAutoApprovePrompts function + runDaemonCommand + runRunCommand (≥ 8 occurrences total)
6. `grep -n "autoApprovePrompts" pkg/factory/factory.go` — two occurrences in LogEffectiveConfig
7. `go test ./pkg/config/... ./pkg/globalconfig/... ./pkg/factory/... -count=1` — all pass
8. `go test -run TestExtractAutoApprovePrompts ./... -count=1` — passes (or equivalent test name)
</verification>
