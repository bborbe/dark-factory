---
status: completed
spec: [088-disable-auto-prompt-generation]
summary: Threaded disableAutoGeneratePrompts config field through all layers (Config, partialConfig, GlobalConfig, FieldSources, LayeredProjectOverrides, applyGlobalOverrides, computeFieldSources, supportedSetKeys, applyOneSetOverride, LogEffectiveConfig) with tests for applySetOverrides/computeFieldSources/applyGlobalOverrides
container: dark-factory-exec-409-spec-088-config-field-plumbing
dark-factory-version: v0.169.0
created: "2026-05-24T00:00:00Z"
queued: "2026-05-23T22:30:51Z"
started: "2026-05-23T22:30:52Z"
completed: "2026-05-23T22:39:03Z"
branch: dark-factory/disable-auto-prompt-generation
---

<summary>
- `Config.DisableAutoGeneratePrompts bool` added to `pkg/config/config.go` (YAML tag: `disableAutoGeneratePrompts,omitempty`)
- `partialConfig.DisableAutoGeneratePrompts *bool` added to `pkg/config/loader.go` (YAML tag: `disableAutoGeneratePrompts`, no `omitempty` — needed for explicit `false` at project layer to beat global `true`)
- `GlobalConfig.DisableAutoGeneratePrompts *bool` added to `pkg/globalconfig/globalconfig.go` (YAML tag: `disableAutoGeneratePrompts,omitempty`)
- `FieldSources.DisableAutoGeneratePrompts string` added to `pkg/config/sources.go`
- `LayeredProjectOverrides.DisableAutoGeneratePrompts *bool` added to `pkg/config/loader.go`
- Merge branches added to `mergePartialContainer` in `pkg/config/loader.go` (mirrors `HideGit` pattern)
- Global-to-effective overrides added to `applyGlobalOverrides` in `main.go` (mirrors `HideGit` pattern)
- `computeFieldSources` in `main.go` updated to include `DisableAutoGeneratePrompts` with default/global/project sources
- `supportedSetKeys` in `main.go` extended with `"disableAutoGeneratePrompts"`
- `applyOneSetOverride` in `main.go` extended with `case "disableAutoGeneratePrompts"` using `parseStrictBool`
- `LogEffectiveConfig` in `pkg/factory/factory.go` extended with `disableAutoGeneratePrompts` and `disableAutoGeneratePromptsSource` attributes
- `roundtrip_test.go` auto-detects new Config field (no manual changes needed)
- `make precommit` passes
</summary>

<objective>
Thread the `disableAutoGeneratePrompts` config flag through all config layers (default, global, project, CLI) so the spec watcher can read the effective value and gate generator calls.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Key files to read before making changes:
- `pkg/config/config.go` — lines 85-126, the `Config` struct. Add `DisableAutoGeneratePrompts bool` after `AutoApprovePrompts` (line 118). YAML tag: `disableAutoGeneratePrompts,omitempty`.
- `pkg/config/loader.go` — lines 83-129, the `partialConfig` struct. Add `DisableAutoGeneratePrompts *bool` with YAML tag `disableAutoGeneratePrompts` (no `omitempty` — mirrors `HideGit` pattern at line 123). Also add to `LayeredProjectOverrides` at lines 33-43. Add merge branch in `mergePartialContainer` (lines 358-360 pattern).
- `pkg/globalconfig/globalconfig.go` — lines 45-53, the `GlobalConfig` struct. Add `DisableAutoGeneratePrompts *bool` with YAML tag `disableAutoGeneratePrompts,omitempty`. Also add to the partial struct at lines 178-186 and the merge block at lines 191-211.
- `pkg/config/sources.go` — lines 10-20, the `FieldSources` struct. Add `DisableAutoGeneratePrompts string` (mirrors `HideGit`, `AutoApprovePrompts`).
- `main.go` — `applyGlobalOverrides` (lines 538-558), `computeFieldSources` (lines 565-623), `supportedSetKeys` (lines 671-682), `applyOneSetOverride` (lines 750-801). Mirror the `HideGit`/`AutoApprovePrompts` pattern for each.
- `pkg/factory/factory.go` — `LogEffectiveConfig` (lines 92-163). Add `disableAutoGeneratePrompts` and `disableAutoGeneratePromptsSource` to the slog.Info call (mirror `autoApprovePrompts` pattern at lines 149-150).
- `pkg/config/roundtrip_test.go` — auto-detects new Config fields via reflection. No manual changes needed; the `Entry` table for bool fields includes `hideGit` (line 178) — `DisableAutoGeneratePrompts` will be picked up automatically.
</context>

<requirements>

### 1. Add `DisableAutoGeneratePrompts` to `Config` struct in `pkg/config/config.go`

In the `Config` struct (around line 118, after `AutoApprovePrompts bool`), add:

```go
DisableAutoGeneratePrompts bool `yaml:"disableAutoGeneratePrompts,omitempty"`
```

This is a plain bool — Go zero-value `false` is the default and matches current behavior. No change to `Defaults()` needed.

### 2. Add `DisableAutoGeneratePrompts` to `partialConfig` struct in `pkg/config/loader.go`

In `partialConfig` (around line 128, after `AutoApprovePrompts *bool`), add:

```go
DisableAutoGeneratePrompts *bool `yaml:"disableAutoGeneratePrompts"`
```

The YAML tag has NO `omitempty` — this is intentional. Mirrors `HideGit` at line 123. Without it, project-level `disableAutoGeneratePrompts: false` (explicit) would not beat global `disableAutoGeneratePrompts: true`.

### 3. Add `DisableAutoGeneratePrompts` to `LayeredProjectOverrides` in `pkg/config/loader.go`

In `LayeredProjectOverrides` (around line 42, after `AutoApprovePrompts *bool`), add:

```go
DisableAutoGeneratePrompts *bool
```

### 4. Add merge branch in `mergePartialContainer` in `pkg/config/loader.go`

In `mergePartialContainer` (around line 358-360, after the `HideGit` block):

```go
if partial.DisableAutoGeneratePrompts != nil {
    cfg.DisableAutoGeneratePrompts = *partial.DisableAutoGeneratePrompts
}
```

### 5. Add `DisableAutoGeneratePrompts` to `GlobalConfig` struct in `pkg/globalconfig/globalconfig.go`

In `GlobalConfig` (around line 52, after `AutoApprovePrompts *bool`), add:

```go
DisableAutoGeneratePrompts *bool `yaml:"disableAutoGeneratePrompts,omitempty"`
```

Also add it to the partial struct inside `Load` (around line 184):

```go
AutoApprovePrompts *bool             `yaml:"autoApprovePrompts"`
DisableAutoGeneratePrompts *bool    `yaml:"disableAutoGeneratePrompts"`
```

And add the merge in the `Load` method (around lines 206-208):

```go
if partial.AutoApprovePrompts != nil {
    cfg.AutoApprovePrompts = partial.AutoApprovePrompts
}
if partial.DisableAutoGeneratePrompts != nil {
    cfg.DisableAutoGeneratePrompts = partial.DisableAutoGeneratePrompts
}
```

### 6. Add `DisableAutoGeneratePrompts` to `FieldSources` struct in `pkg/config/sources.go`

In `FieldSources` (around line 19, after `AutoApprovePrompts string`), add:

```go
DisableAutoGeneratePrompts string
```

### 7. Update `applyGlobalOverrides` in `main.go` to apply global DisableAutoGeneratePrompts

In `applyGlobalOverrides` (around line 555-557), add after the `AutoApprovePrompts` block:

```go
if global.DisableAutoGeneratePrompts != nil && proj.DisableAutoGeneratePrompts == nil {
    cfg.DisableAutoGeneratePrompts = *global.DisableAutoGeneratePrompts
}
```

### 8. Update `computeFieldSources` in `main.go` to track DisableAutoGeneratePrompts source

In `computeFieldSources` (around lines 577-578), add `"DisableAutoGeneratePrompts": "default"` to the struct literal.

Also add the global check (around line 592):

```go
if global.DisableAutoGeneratePrompts != nil {
    s.DisableAutoGeneratePrompts = "global"
}
```

And the project override (around line 607-608):

```go
if proj.DisableAutoGeneratePrompts != nil {
    s.DisableAutoGeneratePrompts = "project"
}
```

### 9. Add `disableAutoGeneratePrompts` to `supportedSetKeys` in `main.go`

In `supportedSetKeys` (around line 682), add `"disableAutoGeneratePrompts"` to the slice. Add it after `"autoMerge"` at the end of the list.

### 10. Add `case "disableAutoGeneratePrompts"` to `applyOneSetOverride` in `main.go`

Mirror the existing `case "hideGit":` block in `applyOneSetOverride` (around line 751). Add the new case before the `default:` case:

```go
case "disableAutoGeneratePrompts":
    b, err := parseStrictBool(ctx, key, value)
    if err != nil {
        return err
    }
    cfg.DisableAutoGeneratePrompts = b
    sources.DisableAutoGeneratePrompts = "arg"
```

Note: do NOT model after `autoApprovePrompts` — it is in `supportedSetKeys` historically but has no `case` arm in `applyOneSetOverride`. The `hideGit` case is the correct template.

### 11. Update `LogEffectiveConfig` in `pkg/factory/factory.go` to log DisableAutoGeneratePrompts

In `LogEffectiveConfig` (around lines 149-150), add after the `autoApprovePrompts` lines:

```go
"autoApprovePrompts", cfg.AutoApprovePrompts,
"autoApprovePromptsSource", sources.AutoApprovePrompts,
"disableAutoGeneratePrompts", cfg.DisableAutoGeneratePrompts,
"disableAutoGeneratePromptsSource", sources.DisableAutoGeneratePrompts,
```

### 12. Add capture of `DisableAutoGeneratePrompts` in `loadWithOverrides` in `pkg/config/loader.go`

In `loadWithOverrides` (around line 183), add to the `overrides` capture:

```go
DisableAutoGeneratePrompts: partial.DisableAutoGeneratePrompts,
```

### 13. Verify roundtrip test

Run `go test ./pkg/config/... -run Roundtrip -v` to confirm the new field is auto-detected by the parity test. No manual changes needed — the test uses reflection to detect all Config fields.

### 14. Add tests for `--set`, `computeFieldSources`, and `applyGlobalOverrides` in `main_internal_test.go`

These tests cover the rung-3 CLI + global-layering ACs from the spec. They belong with this prompt because they test code added in Requirements 7, 8, 9, 10.

Mirror the existing `hideGit` test patterns:

- `applySetOverrides` tests live around line 532 (search for `It("sets hideGit=true and marks source=arg"`).
- `computeFieldSources` tests live around line 345 (search for `Expect(s.HideGit).To(Equal("default"))`).
- `applyGlobalOverrides` tests live around line 199 (search for `Describe("applyGlobalOverrides"`, or for `applies global hideGit when project did not set it`).

Add to the `applySetOverrides` Describe block:

```go
It("sets disableAutoGeneratePrompts=true and marks source=arg", func() {
    cfg := config.Defaults()
    sources := config.FieldSources{}
    Expect(
        applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"disableAutoGeneratePrompts": "true"}),
    ).To(Succeed())
    Expect(cfg.DisableAutoGeneratePrompts).To(BeTrue())
    Expect(sources.DisableAutoGeneratePrompts).To(Equal("arg"))
})

It("sets disableAutoGeneratePrompts=false and marks source=arg", func() {
    cfg := config.Defaults()
    cfg.DisableAutoGeneratePrompts = true
    sources := config.FieldSources{}
    Expect(
        applySetOverrides(ctx, &cfg, &sources, "daemon", map[string]string{"disableAutoGeneratePrompts": "false"}),
    ).To(Succeed())
    Expect(cfg.DisableAutoGeneratePrompts).To(BeFalse())
    Expect(sources.DisableAutoGeneratePrompts).To(Equal("arg"))
})

It("rejects disableAutoGeneratePrompts=yes (strict bool)", func() {
    cfg := config.Defaults()
    sources := config.FieldSources{}
    err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"disableAutoGeneratePrompts": "yes"})
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("invalid bool"))
    Expect(err.Error()).To(ContainSubstring("true or false"))
})
```

Add to the `computeFieldSources` Describe block:

```go
It("returns default for disableAutoGeneratePrompts when neither global nor project set it", func() {
    global := globalconfig.GlobalConfig{}
    proj := config.LayeredProjectOverrides{}
    s := computeFieldSources(global, proj)
    Expect(s.DisableAutoGeneratePrompts).To(Equal("default"))
})

It("returns global for disableAutoGeneratePrompts when global sets it and project does not", func() {
    t := true
    global := globalconfig.GlobalConfig{DisableAutoGeneratePrompts: &t}
    proj := config.LayeredProjectOverrides{}
    s := computeFieldSources(global, proj)
    Expect(s.DisableAutoGeneratePrompts).To(Equal("global"))
})

It("returns project for disableAutoGeneratePrompts when project explicitly sets false", func() {
    t := true
    global := globalconfig.GlobalConfig{DisableAutoGeneratePrompts: &t}
    f := false
    proj := config.LayeredProjectOverrides{DisableAutoGeneratePrompts: &f}
    s := computeFieldSources(global, proj)
    Expect(s.DisableAutoGeneratePrompts).To(Equal("project"))
})

It("returns project for disableAutoGeneratePrompts when project explicitly sets true", func() {
    global := globalconfig.GlobalConfig{}
    t := true
    proj := config.LayeredProjectOverrides{DisableAutoGeneratePrompts: &t}
    s := computeFieldSources(global, proj)
    Expect(s.DisableAutoGeneratePrompts).To(Equal("project"))
})
```

Add to the `applyGlobalOverrides` Describe block:

```go
It("applies global disableAutoGeneratePrompts when project did not set it", func() {
    cfg := config.Defaults()
    t := true
    global := globalconfig.GlobalConfig{DisableAutoGeneratePrompts: &t}
    proj := config.LayeredProjectOverrides{}
    applyGlobalOverrides(&cfg, global, proj)
    Expect(cfg.DisableAutoGeneratePrompts).To(BeTrue())
})

It("does not overwrite project disableAutoGeneratePrompts=false with global true", func() {
    cfg := config.Defaults()
    t := true
    global := globalconfig.GlobalConfig{DisableAutoGeneratePrompts: &t}
    f := false
    proj := config.LayeredProjectOverrides{DisableAutoGeneratePrompts: &f}
    applyGlobalOverrides(&cfg, global, proj)
    Expect(cfg.DisableAutoGeneratePrompts).To(BeFalse())
})
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must continue to pass without modification
- Follow the exact `HideGit` pattern for every addition — same struct position, same merge helper, same yaml tag style
- The `*bool` in `partialConfig` must NOT have `omitempty` (mirrors `HideGit` at `loader.go:123`)
- No new interfaces or exported symbols beyond the existing patterns
</constraints>

<verification>
```bash
# Core field presence
grep -nE 'DisableAutoGeneratePrompts\s+bool' pkg/config/config.go
grep -nE 'DisableAutoGeneratePrompts\s+\*bool' pkg/config/loader.go
grep -nE 'DisableAutoGeneratePrompts\s+\*bool' pkg/globalconfig/globalconfig.go
grep -n 'DisableAutoGeneratePrompts string' pkg/config/sources.go
grep -n '"disableAutoGeneratePrompts"' main.go
grep -n 'DisableAutoGeneratePrompts' pkg/factory/factory.go

# roundtrip auto-detection
go test ./pkg/config/... -run Roundtrip -v

# Config and globalconfig loading
go test ./pkg/config/... -v
go test ./pkg/globalconfig/... -v

# main internal tests
go test . -run "ApplyGlobal|SetOverrides|computeFieldSources" -v
grep -n 'disableAutoGeneratePrompts' main_internal_test.go

# Final validation
make precommit
```
</verification>