---
status: draft
spec: [040-extra-mounts]
created: "2026-04-02T08:30:00Z"
---

<summary>
- Projects can configure additional Docker volume mounts injected into every YOLO container run
- Each extra mount specifies a host path (`src`), container path (`dst`), and optional `readonly` flag (defaults to true)
- Relative `src` paths are resolved relative to the project root; tilde (`~/`) is expanded to the home directory; absolute paths are used as-is
- Missing `src` paths at execution time are logged as a warning and skipped — they do not abort the run
- Empty `extraMounts` or missing field leaves existing behavior unchanged
- Config validation rejects entries with empty `src` or empty `dst`
- `docs/configuration.md` is updated with the new field and example YAML
</summary>

<objective>
Add `extraMounts` to `.dark-factory.yaml` so users can mount shared documentation, coding guides, or config directories into every YOLO container without duplicating files across repos. Mounts are read-only by default and follow the same injection pattern as the existing `netrcFile` and `gitconfigFile` mounts.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making any changes:
- `pkg/config/config.go` — `Config` struct (~line 66), `Validate()` (~line 137), `resolveFilePath()` (~line 308)
- `pkg/config/loader.go` — `partialConfig` struct (~line 53), `mergePartial()` (~line 139)
- `pkg/executor/executor.go` — `dockerExecutor` struct (~line 62), `NewDockerExecutor()` (~line 40), `buildDockerCommand()` (~line 309). Pay attention to the existing `netrcFile` and `gitconfigFile` patterns.
- `pkg/executor/executor_internal_test.go` — existing tests for `buildDockerCommand` (the `Describe("buildDockerCommand", ...)` block)
- `pkg/config/config_test.go` — existing config validation and loader tests
- `pkg/factory/factory.go` — `CreateProcessor()` (~line 417), `CreateSpecGenerator()` (~line 356), `CreateRunner()` (~line 206), `CreateOneShotRunner()` (~line 276)
- `docs/configuration.md` — existing documentation to update
</context>

<requirements>
**Step 1 — Add the ExtraMount type and Config field (`pkg/config/config.go`)**

1a. Add a new `ExtraMount` struct above the `Config` struct:
```go
// ExtraMount describes an additional volume mount to inject into the YOLO container.
type ExtraMount struct {
    Src      string `yaml:"src"`
    Dst      string `yaml:"dst"`
    Readonly *bool  `yaml:"readonly,omitempty"` // nil defaults to true
}

// IsReadonly returns true if the mount is read-only (default when Readonly is nil).
func (m ExtraMount) IsReadonly() bool {
    if m.Readonly == nil {
        return true
    }
    return *m.Readonly
}
```

1b. Add `ExtraMounts []ExtraMount \`yaml:"extraMounts,omitempty"\`` to the `Config` struct, after the `Env` field.

1c. Add a `validateExtraMounts` method and wire it into `Validate()`:
```go
// validateExtraMounts validates each extra mount entry.
func (c Config) validateExtraMounts(ctx context.Context) error {
    for i, m := range c.ExtraMounts {
        if m.Src == "" {
            return errors.Errorf(ctx, "extraMounts[%d].src must not be empty", i)
        }
        if m.Dst == "" {
            return errors.Errorf(ctx, "extraMounts[%d].dst must not be empty", i)
        }
    }
    return nil
}
```

Wire it into `Validate()` as: `validation.Name("extraMounts", validation.HasValidationFunc(c.validateExtraMounts))` — add it after the `"env"` entry.

**Step 2 — Update the config loader (`pkg/config/loader.go`)**

2a. Add `ExtraMounts []ExtraMount \`yaml:"extraMounts,omitempty"\`` to `partialConfig`. Slices use nil to mean "not set" (same pattern as the `Env map[string]string` field — no pointer wrapper needed).

2b. Add merge logic in `mergePartial()`, after the `Env` block:
```go
if partial.ExtraMounts != nil {
    cfg.ExtraMounts = partial.ExtraMounts
}
```

**Step 3 — Update the executor (`pkg/executor/executor.go`)**

3a. Add `extraMounts []config.ExtraMount` field to `dockerExecutor` struct, after the `env` field. Update `NewDockerExecutor()` to accept `extraMounts []config.ExtraMount` as a new parameter (add it after `env map[string]string`) and store it in the struct.

3b. In `buildDockerCommand()`, after the existing gitconfigFile mount block and before `args = append(args, e.containerImage)`, add:
```go
for _, m := range e.extraMounts {
    src := m.Src
    // Resolve tilde
    if strings.HasPrefix(src, "~/") {
        src = home + src[1:]
    } else if !filepath.IsAbs(src) {
        // Relative paths resolved from project root
        src = filepath.Join(projectRoot, src)
    }
    if _, err := os.Stat(src); err != nil {
        slog.Warn("extraMounts: src path does not exist, skipping", "src", src, "dst", m.Dst)
        continue
    }
    mount := src + ":" + m.Dst
    if m.IsReadonly() {
        mount += ":ro"
    }
    args = append(args, "-v", mount)
}
```

Note: `buildDockerCommand` already imports `os`, `strings`, and `filepath`. The import of `config` package must be added: `"github.com/bborbe/dark-factory/pkg/config"`. Check for existing import of `config` in executor.go — the test file imports it but check if executor.go itself already imports it. If not, add the import.

**Step 4 — Update the factory (`pkg/factory/factory.go`)**

4a. Update `CreateProcessor()` signature: add `extraMounts []config.ExtraMount` parameter after `env map[string]string` (before `currentDateTimeGetter`). Pass `extraMounts` to `NewDockerExecutor()` after `env`.

4b. Update `CreateSpecGenerator()`: pass `cfg.ExtraMounts` to `NewDockerExecutor()` after `cfg.Env`.

4c. Update both `CreateProcessor` call sites:
- In `CreateRunner()` (~line 248): add `cfg.ExtraMounts` after `cfg.Env` in the `CreateProcessor(...)` call
- In `CreateOneShotRunner()` (~line 312): add `cfg.ExtraMounts` after `cfg.Env` in the `CreateProcessor(...)` call

**Step 5 — Update documentation (`docs/configuration.md`)**

In the "Private Go Modules" section, add `extraMounts` to the table and the example YAML block. Then add a new "Extra Mounts" section before or after "Private Go Modules":

```markdown
## Extra Mounts

Share documentation or config directories across repos without duplicating them:

```yaml
extraMounts:
  - src: ../docs/howto
    dst: /docs
  - src: ~/Documents/workspaces/coding/docs
    dst: /coding-docs
    readonly: true
```

| Field | Required | Default | Purpose |
|-------|----------|---------|---------|
| `src` | yes | — | Host path. Relative paths resolved from project root. `~/` expanded to home. |
| `dst` | yes | — | Container path where `src` is mounted. |
| `readonly` | no | `true` | Mount read-only (`:ro`). Set `false` for writable access. |

Missing `src` paths at execution time are logged as a warning and skipped — they do not abort the run.
```

Also add `extraMounts` to the Full Example at the bottom of `docs/configuration.md`.

**Step 6 — Write tests**

6a. In `pkg/config/config_test.go`, add tests for `validateExtraMounts`:
- Entry with empty `src` fails validation with message containing "src must not be empty"
- Entry with empty `dst` fails validation with message containing "dst must not be empty"
- Valid entry (non-empty src and dst) passes validation
- `ExtraMounts` field missing (nil slice) passes validation

6b. In `pkg/config/config_test.go` (loader section), add tests:
- Config YAML with `extraMounts:` list loads correctly and populates `ExtraMounts`
- Config YAML without `extraMounts` field leaves `ExtraMounts` nil (existing behavior unchanged)
- `readonly: false` is correctly parsed and `IsReadonly()` returns false
- Omitted `readonly` field has `IsReadonly()` returning true

6c. In `pkg/executor/executor_internal_test.go`, add tests to the `Describe("buildDockerCommand", ...)` block:
- When `extraMounts` is nil/empty: no extra `-v` flags beyond the standard ones
- When `extraMounts` has an entry with an existing `src` and `readonly` = nil (default): `-v /resolved/src:/dst:ro` is in args
- When `extraMounts` has an entry with `readonly` = false: `-v /resolved/src:/dst` (no `:ro`) is in args
- When `extraMounts` has an entry whose resolved `src` does not exist: no `-v` flag is added (skip silently)
- Relative `src` is joined with `projectRoot` (use a real temp dir for the src)
- Tilde `~/` in `src` is expanded using the `home` parameter

For executor tests, use `os.MkdirTemp` or `os.MkdirAll` to create real temporary directories as test `src` paths, since `buildDockerCommand` calls `os.Stat`.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `readonly` defaults to `true` — safety first; omitted = `:ro`
- No changes to existing mounts (netrcFile, gitconfigFile, workspace, claude, go/pkg) or their ordering
- When `extraMounts` is nil or empty, behavior is exactly unchanged
- Missing `src` is NOT fatal — log with `slog.Warn` and skip; do not return an error
- Config validation only checks `src`/`dst` are non-empty — existence is checked at runtime
- Tilde expansion in `buildDockerCommand` uses the `home` parameter already available; resolution of relative paths uses `projectRoot` already available
- `pkg/executor` does NOT currently import `pkg/config` (verified). Add `"github.com/bborbe/dark-factory/pkg/config"` to executor.go's imports so the `ExtraMount` type can be used directly — there is no import cycle.
- All new tests use Ginkgo v2 / Gomega, external test package (e.g. `package executor_test` via the internal test file already uses `package executor`)
</constraints>

<verification>
```
make precommit
```
Must pass with no errors.

Additional checks:
```bash
# Confirm ExtraMount type exists in config package
grep -n "ExtraMount" pkg/config/config.go

# Confirm extraMounts field in partialConfig
grep -n "ExtraMounts" pkg/config/loader.go

# Confirm NewDockerExecutor signature updated
grep -n "func NewDockerExecutor" pkg/executor/executor.go

# Confirm factory call sites pass ExtraMounts
grep -n "ExtraMounts" pkg/factory/factory.go

# Confirm docs updated
grep -n "extraMounts" docs/configuration.md
```
</verification>
