---
status: committing
spec: [082-global-env-layering]
summary: Added global env var support to ~/.dark-factory/config.yaml with key-level project-wins merge, envKeyPattern validation, home-file permission warning, and env-source reporting in the effective-config log line.
container: dark-factory-exec-392-spec-082-globalconfig-merge-and-log
dark-factory-version: v0.162.0
created: "2026-05-19T00:00:00Z"
queued: "2026-05-19T16:50:54Z"
started: "2026-05-19T16:50:56Z"
branch: dark-factory/global-env-layering
---

<summary>
- Users can set env vars once in `~/.dark-factory/config.yaml`; every project picks them up automatically
- A project's `env:` block overrides individual global keys without replacing the whole inherited map
- The container launched by dark-factory sees the fully merged env (union of global + project, project wins on collision)
- Env key names in both global and project config must match `^[A-Z_][A-Z0-9_]*$`; any non-matching key fails config load with a clear error naming the offending key
- When the home config file has group or world read/write permissions, a single warning is logged at load time; the file is still loaded normally
- The startup `effective config` log line reports env keys grouped by source (`from-global`, `project-overrides`, `project-only`) and never logs env values
- Validation error messages never include env values (only the offending key name)
- Project env continues to behave exactly as before when no global env is set (zero behavioral change for existing projects)
</summary>

<objective>
Add `env:` support to the global dark-factory config (`~/.dark-factory/config.yaml`) and implement key-level merge so the container launcher receives a unified env map — global keys as base, project keys winning on collision — without changing any other field's behavior. Add env-source reporting to the `effective config` log line (keys only, no values), and add a home-file permission warning.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-logging-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-security-linting.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` for which test types to write.

Files to read in full before editing:
- `pkg/globalconfig/globalconfig.go` — `GlobalConfig` struct (lines ~38–45), `Validate()` (lines ~48–77), `fileLoader.Load()` (lines ~125–186)
- `pkg/config/config.go` — `Config.Env` field (line ~111), `validateEnv()` (lines ~554–573), `Validate()` call to validateEnv (line ~221)
- `pkg/config/loader.go` — `LayeredProjectOverrides` (lines ~33–43), `mergePartialContainer` env block (lines ~352–354)
- `pkg/factory/factory.go` — `LogEffectiveConfig` (lines ~82–139), `createStartupLogger` (lines ~141–152), `CreateRunner` env usage (line ~403), `CreateOneShotRunner` env usage (line ~556), `CreateSpecGenerator` env usage (line ~617)
- `pkg/globalconfig/globalconfig_test.go` — understand existing test structure and suite setup
- `pkg/config/config_test.go` — understand existing test structure
- `pkg/executor/executor_test.go` — find `BuildDockerCommandForTest` call sites and understand the helper
- `pkg/executor/export_test.go` — understand `BuildDockerCommandForTest` signature
</context>

<requirements>

## 1. Add `Env` to `GlobalConfig` struct (`pkg/globalconfig/globalconfig.go`)

In the `GlobalConfig` struct, add a new field after `AutoApprovePrompts`:

```go
type GlobalConfig struct {
	MaxContainers      int               `yaml:"maxContainers"`
	HideGit            *bool             `yaml:"hideGit,omitempty"`
	AutoRelease        *bool             `yaml:"autoRelease,omitempty"`
	DirtyFileThreshold *int              `yaml:"dirtyFileThreshold,omitempty"`
	Model              *string           `yaml:"model,omitempty"`
	AutoApprovePrompts *bool             `yaml:"autoApprovePrompts,omitempty"`
	Env                map[string]string `yaml:"env,omitempty"`
}
```

## 2. Add env key validation to `GlobalConfig.Validate()` (`pkg/globalconfig/globalconfig.go`)

Add a package-level regex variable for the allowed env key pattern:

```go
// envKeyPattern is the required format for environment variable key names.
const envKeyPattern = `^[A-Z_][A-Z0-9_]*$`

// envKeyRegexp validates environment variable key names.
var envKeyRegexp = regexp.MustCompile(envKeyPattern)
```

`regexp` is already imported. Add the following to `GlobalConfig.Validate()`, after the existing `Model` check and before `return nil`:

```go
for k := range g.Env {
    if !envKeyRegexp.MatchString(k) {
        return errors.Errorf(
            ctx,
            "globalconfig: env key %q does not match required pattern %s",
            k,
            envKeyPattern,
        )
    }
}
```

## 3. Wire env into the globalconfig partial struct and Load() (`pkg/globalconfig/globalconfig.go`)

In `fileLoader.Load()`, the local `partial` struct (currently around line 150) must gain an `Env` field so it can be unmarshalled from YAML:

```go
var partial struct {
    MaxContainers      *int              `yaml:"maxContainers"`
    HideGit            *bool             `yaml:"hideGit"`
    AutoRelease        *bool             `yaml:"autoRelease"`
    DirtyFileThreshold *int              `yaml:"dirtyFileThreshold"`
    Model              *string           `yaml:"model"`
    AutoApprovePrompts *bool             `yaml:"autoApprovePrompts"`
    Env                map[string]string `yaml:"env,omitempty"`
}
```

After `yaml.Unmarshal(data, &partial)` succeeds, add the merge block alongside the existing field merges:

```go
if partial.Env != nil {
    cfg.Env = partial.Env
}
```

## 4. Add home-file permission warning in `fileLoader.Load()` (`pkg/globalconfig/globalconfig.go`)

Add a best-effort permission check immediately after `configPath` is computed (before the `os.ReadFile` call). The check must:
- stat the file; skip the check entirely if stat fails (file may not exist yet)
- emit one `slog.Warn` line if any of `group-read (0040)`, `group-write (0020)`, `other-read (0004)`, or `other-write (0002)` bits are set (i.e., `perm & 0066 != 0`)
- include the file path in the log line
- NOT block loading regardless of the result

```go
// Best-effort permission check — skip silently on stat failure.
if info, statErr := os.Stat(configPath); statErr == nil {
    if perm := info.Mode().Perm(); perm&0066 != 0 {
        slog.Warn(
            "global config has group or world read/write permissions; consider: chmod 600",
            "path", configPath,
        )
    }
}
```

Insert this block BEFORE the `os.ReadFile(configPath)` call. If the file doesn't exist, `os.Stat` returns a non-nil error, the `if` body is skipped, and the subsequent `os.IsNotExist` check handles the missing-file case normally.

Add `"log/slog"` to the import block if not already present.

## 5. Add env key format validation to `Config.validateEnv()` (`pkg/config/config.go`)

In `pkg/config/config.go`, add a package-level regex variable (same pattern as globalconfig):

```go
// envKeyPattern is the required format for environment variable key names.
const envKeyPattern = `^[A-Z_][A-Z0-9_]*$`

// envKeyRegexp validates environment variable key names.
var envKeyRegexp = regexp.MustCompile(envKeyPattern)
```

Add `"regexp"` to the import block.

In `validateEnv()`, add a key format check immediately after the empty-key check (before the reserved-key check):

```go
func (c Config) validateEnv(ctx context.Context) error {
    for k, v := range c.Env {
        if k == "" {
            return errors.Errorf(ctx, "env key must not be empty")
        }
        if !envKeyRegexp.MatchString(k) {
            return errors.Errorf(
                ctx,
                "env key %q does not match required pattern %s",
                k,
                envKeyPattern,
            )
        }
        for _, reserved := range reservedEnvKeys {
            if k == reserved {
                return errors.Errorf(ctx, "env key %q is reserved and cannot be overridden", k)
            }
        }
        if strings.ContainsAny(v, "\x00\n\r") {
            return errors.Errorf(ctx, "env value for %q contains invalid characters", k)
        }
    }
    return nil
}
```

## 6. Add `MergeEnv` function to `pkg/config/` (new file `pkg/config/env.go`)

Create a new file `pkg/config/env.go` with the merge function:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

// MergeEnv returns a new map with globalEnv as the base and projectEnv overlaid.
// For each key present in both, the project value wins.
// Keys present only in either source are preserved unchanged.
// Returns nil when both inputs are nil or empty.
func MergeEnv(globalEnv, projectEnv map[string]string) map[string]string {
	if len(globalEnv) == 0 && len(projectEnv) == 0 {
		return nil
	}
	merged := make(map[string]string, len(globalEnv)+len(projectEnv))
	for k, v := range globalEnv {
		merged[k] = v
	}
	for k, v := range projectEnv {
		merged[k] = v // project wins on collision
	}
	return merged
}
```

## 7. Update `pkg/factory/factory.go` — merge env before executor creation

### 7a. In `CreateRunner`

Immediately after `globalCfg` is loaded (currently around line 298-301), save the original project env and compute the merged env, then update `cfg.Env`:

```go
globalCfg, err := globalconfig.NewLoader().Load(ctx)
if err != nil {
    return &errRunner{err: errors.Wrap(ctx, err, "globalconfig")}
}
// Merge global env with project env (project wins on collision).
projectEnv := cfg.Env
cfg.Env = config.MergeEnv(globalCfg.Env, projectEnv)
```

Place this block immediately after the `globalconfig.NewLoader().Load(ctx)` error check. After this point, `cfg.Env` is the merged map. The call to `CreateSpecGenerator` (which uses `cfg` internally) and `CreateProcessor` (which receives `cfg.Env`) will both naturally consume the merged env. **No changes needed to the individual call sites at lines 403 and 617.**

Update the `createStartupLogger` call (currently around line 453) to pass `projectEnv`:

```go
createStartupLogger(ctx, cfg, globalCfg, sources, projectEnv),
```

### 7b. In `CreateOneShotRunner`

Apply the identical pattern immediately after `globalCfg` is loaded (around line 472):

```go
globalCfg, err := globalconfig.NewLoader().Load(ctx)
if err != nil {
    return &errOneShotRunner{err: errors.Wrap(ctx, err, "globalconfig")}
}
// Merge global env with project env (project wins on collision).
projectEnv := cfg.Env
cfg.Env = config.MergeEnv(globalCfg.Env, projectEnv)
```

Update the `createStartupLogger` call (around line 598):

```go
createStartupLogger(ctx, cfg, globalCfg, sources, projectEnv),
```

## 8. Update `LogEffectiveConfig` and `createStartupLogger` (`pkg/factory/factory.go`)

### 8a. Update `createStartupLogger` signature

Add `projectEnv map[string]string` as the last parameter:

```go
func createStartupLogger(
    ctx context.Context,
    cfg config.Config,
    globalCfg globalconfig.GlobalConfig,
    sources config.FieldSources,
    projectEnv map[string]string,
) func() {
    present, _ := globalconfig.FileExists(ctx)
    return func() { LogEffectiveConfig(cfg, globalCfg, present, sources, projectEnv) }
}
```

### 8b. Update `LogEffectiveConfig` signature and body

Add `projectEnv map[string]string` as the last parameter. Inside the function, compute the three source groupings (using sorted, reproducible output) and add them to the `slog.Info` call. Values must never appear in the log line.

Add `"sort"` to the factory.go import block.

```go
func LogEffectiveConfig(
    cfg config.Config,
    globalCfg globalconfig.GlobalConfig,
    globalFilePresent bool,
    sources config.FieldSources,
    projectEnv map[string]string,
) {
    // ... existing maxContainers source computation unchanged ...

    // Compute env key groupings — values are never logged.
    var fromGlobal, projectOverrides, projectOnly []string
    for k := range globalCfg.Env {
        if _, overridden := projectEnv[k]; overridden {
            projectOverrides = append(projectOverrides, k)
        } else {
            fromGlobal = append(fromGlobal, k)
        }
    }
    for k := range projectEnv {
        if _, inGlobal := globalCfg.Env[k]; !inGlobal {
            projectOnly = append(projectOnly, k)
        }
    }
    sort.Strings(fromGlobal)
    sort.Strings(projectOverrides)
    sort.Strings(projectOnly)

    slog.Info("effective config",
        // ... ALL existing fields unchanged ...
        "envFromGlobal", fromGlobal,
        "envProjectOverrides", projectOverrides,
        "envProjectOnly", projectOnly,
    )
}
```

Add the three new `env*` fields at the END of the existing `slog.Info(...)` call, immediately before the closing `)`. Do not reorder or remove any existing field.

## 9. Tests

**Important — use plain `func TestX(t *testing.T)` (not Ginkgo `Describe`) for the three tests below.** The spec's verification commands use `go test -run TestEnv`, `-run TestEnvMerge`, `-run TestContainerLaunchReceivesMergedEnv`. Ginkgo `Describe("TestEnv", ...)` blocks live under the package's existing `TestSuite` Go function and would NOT match those `-run` filters, silently passing the spec's verification with "no tests to run". Use plain Go test functions with `t.Run` subtests so the names match `-run` literally. The packages can host both Ginkgo suites and plain tests side-by-side — Go's `testing` package runs all `Test*` functions.

### 9a. TestEnv in `pkg/globalconfig/globalconfig_test.go`

Add a plain Go test function (NOT a Ginkgo Describe block):

```go
func TestEnv(t *testing.T) {
    if runtime.GOOS == "windows" {
        // permission sub-case requires POSIX perms; other sub-cases also fine to run there,
        // but skipping the whole function is simplest and matches the spec's POSIX-only carve-out.
    }

    setupHome := func(t *testing.T, configYAML string, perm os.FileMode) string {
        t.Helper()
        tmpHome := t.TempDir()
        require.NoError(t, os.MkdirAll(filepath.Join(tmpHome, ".dark-factory"), 0700))
        require.NoError(t, os.WriteFile(
            filepath.Join(tmpHome, ".dark-factory", "config.yaml"),
            []byte(configYAML), perm,
        ))
        t.Setenv("HOME", tmpHome)   // os.UserHomeDir respects HOME on unix
        return tmpHome
    }

    t.Run("valid_env_key_loads", func(t *testing.T) {
        setupHome(t, "env:\n  API_KEY: value\n", 0600)
        cfg, err := NewLoader().Load(context.Background())
        require.NoError(t, err)
        require.Equal(t, "value", cfg.Env["API_KEY"])
    })

    t.Run("invalid_lowercase_key_rejected", func(t *testing.T) {
        setupHome(t, "env:\n  lowercase_key: value\n", 0600)
        _, err := NewLoader().Load(context.Background())
        require.Error(t, err)
        require.Contains(t, err.Error(), "lowercase_key")
        require.NotContains(t, err.Error(), "value")   // values never leak into errors
    })

    t.Run("invalid_leading_digit_rejected", func(t *testing.T) {
        setupHome(t, "env:\n  1BADKEY: secret\n", 0600)
        _, err := NewLoader().Load(context.Background())
        require.Error(t, err)
        require.Contains(t, err.Error(), "1BADKEY")
        require.NotContains(t, err.Error(), "secret")
    })

    t.Run("invalid_special_chars_rejected", func(t *testing.T) {
        setupHome(t, "env:\n  BAD-KEY: value\n", 0600)
        _, err := NewLoader().Load(context.Background())
        require.Error(t, err)
        require.Contains(t, err.Error(), "BAD-KEY")
    })

    t.Run("world_readable_perm_warns_but_loads", func(t *testing.T) {
        if runtime.GOOS == "windows" {
            t.Skip("POSIX perms only")
        }
        var buf bytes.Buffer
        origLogger := slog.Default()
        slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
        t.Cleanup(func() { slog.SetDefault(origLogger) })

        tmpHome := setupHome(t, "env:\n  API_KEY: value\n", 0644)
        cfg, err := NewLoader().Load(context.Background())
        require.NoError(t, err)
        require.NotEmpty(t, cfg.Env, "config must still load")

        output := buf.String()
        require.Contains(t, output, filepath.Join(tmpHome, ".dark-factory", "config.yaml"))
        require.Contains(t, output, "chmod 600")
    })
}
```

Add to the import block: `"bytes"`, `"context"`, `"log/slog"`, `"os"`, `"path/filepath"`, `"runtime"`, `"testing"`, and `"github.com/stretchr/testify/require"` (already used elsewhere in the project — grep `testify/require` to confirm). Place the test in `pkg/globalconfig/globalconfig_test.go` or a new sibling file `pkg/globalconfig/env_test.go` (either works — match whatever the existing convention is by grepping the dir).

### 9b. TestEnvMerge in `pkg/config/env_test.go` (new file)

Create `pkg/config/env_test.go` with package `config` (internal test, so `MergeEnv` is accessible without qualification). Use a plain Go test function with table-driven `t.Run` subtests:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
    "testing"

    "github.com/stretchr/testify/require"
)

func TestEnvMerge(t *testing.T) {
    cases := []struct {
        name      string
        global    map[string]string
        project   map[string]string
        expected  map[string]string
    }{
        {
            name:     "global_only",
            global:   map[string]string{"A": "1", "B": "2"},
            project:  nil,
            expected: map[string]string{"A": "1", "B": "2"},
        },
        {
            name:     "project_only",
            global:   nil,
            project:  map[string]string{"X": "10", "Y": "20"},
            expected: map[string]string{"X": "10", "Y": "20"},
        },
        {
            name:     "overlap_project_wins",
            global:   map[string]string{"SHARED": "global", "G_ONLY": "g"},
            project:  map[string]string{"SHARED": "project", "P_ONLY": "p"},
            expected: map[string]string{"SHARED": "project", "G_ONLY": "g", "P_ONLY": "p"},
        },
        {
            name:     "disjoint_union",
            global:   map[string]string{"A": "1"},
            project:  map[string]string{"B": "2"},
            expected: map[string]string{"A": "1", "B": "2"},
        },
        {
            name:     "both_nil_returns_nil",
            global:   nil,
            project:  nil,
            expected: nil,
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            got := MergeEnv(tc.global, tc.project)
            require.Equal(t, tc.expected, got)
        })
    }
}
```

The three spec-required sub-cases (global-only, overlap, disjoint) plus project-only and both-nil are all present.

### 9c. TestContainerLaunchReceivesMergedEnv in `pkg/runner/env_test.go` (new file)

The spec's AC #4 specifically names `pkg/runner/...` (or `pkg/processor/...`). Put this test in `pkg/runner` so the spec's verification command (`go test ./pkg/runner/... -run TestContainerLaunchReceivesMergedEnv -v`) matches literally. The test must exercise the real factory wiring — not a unit-level executor seam — so that an implementer who adds the `Env` field but forgets to call `MergeEnv` in `CreateRunner` is caught.

Create `pkg/runner/env_test.go` with package `runner_test`. The test:

1. Sets up a tmp `HOME` containing `.dark-factory/config.yaml` with global env (e.g. `SHARED: global-val`, `GLOBAL_ONLY: gv`).
2. Builds a `config.Config` with project env (e.g. `SHARED: project-val`, `PROJECT_ONLY: pv`) plus the minimum required fields for `CreateRunner` to succeed (project name, default branch, prompt dirs — grep existing runner tests for the minimum viable config).
3. Calls `factory.CreateRunner(ctx, cfg)` to obtain a runner.
4. Asserts that the runner construction succeeded without error (no `errRunner` wrapper). This proves `MergeEnv` was called without panic and the merged `cfg.Env` was accepted by all downstream constructors.
5. Additionally, captures the `effective config` log line (by redirecting `slog.Default()` to a buffer BEFORE calling `CreateRunner`) and asserts that the line contains `envFromGlobal=[GLOBAL_ONLY]`, `envProjectOverrides=[SHARED]`, and `envProjectOnly=[PROJECT_ONLY]` — using `MatchRegexp` patterns so a buggy grouping that misclassified keys would fail.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
    "bytes"
    "context"
    "log/slog"
    "os"
    "path/filepath"
    "regexp"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/bborbe/dark-factory/pkg/config"
    "github.com/bborbe/dark-factory/pkg/factory"
)

func TestContainerLaunchReceivesMergedEnv(t *testing.T) {
    // 1. tmp HOME with global env
    tmpHome := t.TempDir()
    require.NoError(t, os.MkdirAll(filepath.Join(tmpHome, ".dark-factory"), 0700))
    globalYAML := "env:\n  SHARED: global-val\n  GLOBAL_ONLY: gv\n"
    require.NoError(t, os.WriteFile(
        filepath.Join(tmpHome, ".dark-factory", "config.yaml"),
        []byte(globalYAML), 0600,
    ))
    t.Setenv("HOME", tmpHome)

    // 2. capture slog output
    var buf bytes.Buffer
    origLogger := slog.Default()
    slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
    t.Cleanup(func() { slog.SetDefault(origLogger) })

    // 3. minimal viable project config with project env
    //    Grep `pkg/runner/runner_test.go` (or similar) for the minimal config
    //    used in existing CreateRunner tests, and adapt it here. Add Env.
    cfg := minimalRunnerConfigForTest(t)   // helper defined inline below or in shared test file
    cfg.Env = map[string]string{
        "SHARED":       "project-val",
        "PROJECT_ONLY": "pv",
    }

    // 4. construct via factory — the real merge path
    _ = factory.CreateRunner(context.Background(), cfg)
    // CreateRunner emits the effective config log line lazily on Run(); if it
    // logs at construction time, the buffer already has content. Otherwise,
    // invoke whatever the existing tests use to trigger the startup logger.
    // (Grep `createStartupLogger` callers in pkg/factory.)

    output := buf.String()

    // 5. assert source-grouped reporting reaches the log
    require.Regexp(t, regexp.MustCompile(`envFromGlobal=\[[^]]*GLOBAL_ONLY`), output)
    require.Regexp(t, regexp.MustCompile(`envProjectOverrides=\[[^]]*SHARED`), output)
    require.Regexp(t, regexp.MustCompile(`envProjectOnly=\[[^]]*PROJECT_ONLY`), output)

    // 6. values never appear
    require.NotContains(t, output, "global-val")
    require.NotContains(t, output, "project-val")
    require.NotContains(t, output, "gv")
    require.NotContains(t, output, "pv")
}

// minimalRunnerConfigForTest builds the smallest config.Config that CreateRunner accepts.
// Implementer: grep existing runner tests for the recipe.
func minimalRunnerConfigForTest(t *testing.T) config.Config {
    t.Helper()
    // grep `factory.CreateRunner(` in pkg/runner and copy a passing test's setup
    return config.Config{ /* fill from existing test pattern */ }
}
```

**Implementer note**: if `CreateRunner` does not naturally emit `effective config` at construction time, locate where it IS emitted (`createStartupLogger` returns a closure — see Requirement 8a) and either (a) invoke the closure synchronously in the test, or (b) restructure the test to call `factory.LogEffectiveConfig` directly with the same cfg/globalCfg/projectEnv values. The point is to verify the log line for THIS feature; the exact invocation path is implementer's choice as long as the merge actually drives the log content.

### 9d. LogEffectiveConfig env-source test in `pkg/factory/factory_test.go`

Check whether `pkg/factory/factory_test.go` exists; if not, create it with the appropriate `package factory_test` header and a `TestFactory` suite bootstrap (check `pkg/factory/factory_suite_test.go` first for the suite name).

Add a `Describe("LogEffectiveConfig env reporting", ...)` block with one `It`. Use Gomega's `MatchRegexp` to enforce that each key appears specifically under its expected source group, not just somewhere in the output:

```go
It("reports env keys by source group and never logs values", func() {
    var buf bytes.Buffer
    handler := slog.NewTextHandler(&buf, nil)
    orig := slog.Default()
    slog.SetDefault(slog.New(handler))
    defer slog.SetDefault(orig)

    cfg := config.Config{
        Env: map[string]string{
            "GLOBAL_ONLY":  "gv",
            "SHARED":       "project-wins",
            "PROJECT_ONLY": "pv",
        },
    }
    globalCfg := globalconfig.GlobalConfig{
        MaxContainers: 3,
        Env: map[string]string{
            "GLOBAL_ONLY": "gv",
            "SHARED":      "global-val",
        },
    }
    projectEnv := map[string]string{
        "SHARED":       "project-wins",
        "PROJECT_ONLY": "pv",
    }

    LogEffectiveConfig(cfg, globalCfg, false, config.FieldSources{}, projectEnv)

    output := buf.String()
    // Each key appears specifically in its expected source group (regex anchors group name)
    Expect(output).To(MatchRegexp(`envFromGlobal=\[[^]]*GLOBAL_ONLY`))
    Expect(output).To(MatchRegexp(`envProjectOverrides=\[[^]]*SHARED`))
    Expect(output).To(MatchRegexp(`envProjectOnly=\[[^]]*PROJECT_ONLY`))
    // Values must NOT appear anywhere
    Expect(output).NotTo(ContainSubstring("gv"))
    Expect(output).NotTo(ContainSubstring("project-wins"))
    Expect(output).NotTo(ContainSubstring("global-val"))
    Expect(output).NotTo(ContainSubstring("pv"))
})
```

Import `bytes`, `log/slog`, `github.com/bborbe/dark-factory/pkg/config`, `github.com/bborbe/dark-factory/pkg/globalconfig`, and the Gomega matchers (`. "github.com/onsi/gomega"`) in the test file.

## 10. Add CHANGELOG entry

Under `## Unreleased` in `CHANGELOG.md`, add:

```
- feat: Support global env vars in ~/.dark-factory/config.yaml; project env overrides per-key (key-level merge, project wins). Env keys must match ^[A-Z_][A-Z0-9_]*$. Effective-config log line reports env keys by source layer.
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- The global config schema gains EXACTLY one new top-level field: `env` (string-to-string map). No other global schema changes.
- The project config schema (`.dark-factory.yaml`) is unchanged; only validation tightens (key format check added to `validateEnv`).
- Merge precedence: project keys win over global keys on collision. This is enforced by the second `for` loop in `MergeEnv` overwriting global values.
- The same downstream code path that today consumes `cfg.Env` must consume the merged map after this change. Achieve this by updating `cfg.Env = config.MergeEnv(globalCfg.Env, projectEnv)` before the existing call sites — do NOT introduce a second env variable passed separately.
- Home file permission check: best-effort only; if `os.Stat` fails for any reason, skip the check entirely. The file is always loaded regardless.
- Env values must NEVER appear in `slog` output from `LogEffectiveConfig`, validation error messages, or the permission warning. Only keys may appear.
- Wrap all non-nil errors with `errors.Wrap`/`errors.Wrapf`/`errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- The `envKeyRegexp` in globalconfig.go and in config.go are separate package-level vars — do NOT cross-import between them to share the regex. Copy the pattern.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- All currently passing tests must continue to pass.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -n '^\tEnv ' pkg/globalconfig/globalconfig.go` — returns at least one line (the struct field).
2. `go test ./pkg/globalconfig/... -run TestEnv -v` — exits 0; output includes both a passing positive case and a passing negative case.
3. `go test ./pkg/config/... -run TestEnvMerge -v` — exits 0; output includes at least three sub-cases (global-only, overlap, disjoint).
4. `go test ./pkg/runner/... -run TestContainerLaunchReceivesMergedEnv -v` — exits 0.
5. `grep -n 'envFromGlobal\|envProjectOverrides\|envProjectOnly' pkg/factory/factory.go` — returns three lines (the slog.Info field keys).
6. `grep -n 'MergeEnv' pkg/factory/factory.go` — returns at least two lines (CreateRunner + CreateOneShotRunner).
7. `grep -n 'projectEnv' pkg/factory/factory.go` — returns at least four lines (two saves + two createStartupLogger calls).
8. `grep -n 'chmod 600' pkg/globalconfig/globalconfig.go` — returns one line (the permission warning message).
</verification>
