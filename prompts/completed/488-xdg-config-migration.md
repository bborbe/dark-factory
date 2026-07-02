---
status: completed
summary: Migrate global config discovery from ~/.dark-factory/config.yaml to XDG-first with legacy fallback
execution_id: dark-factory-xdg-config-exec-488-xdg-config-migration
dark-factory-version: dev
created: "2026-07-02T09:30:00Z"
queued: "2026-07-02T09:02:25Z"
started: "2026-07-02T09:02:27Z"
completed: "2026-07-02T09:09:25Z"
---

<summary>
- dark-factory reads its global config from `~/.config/dark-factory/config.yaml` (XDG), with automatic fallback to the legacy `~/.dark-factory/config.yaml`
- A new `FindConfigDir(toolName string)` function returns the first existing directory in XDG-first order, defaulting to XDG when neither exists
- `Load()` and `FileExists()` both route through `FindConfigDir` instead of hardcoding `~/.dark-factory/config.yaml`
- Container lock stays at `~/.dark-factory/container.lock` — it is runtime state, not config; out of scope
- CLI help text and documentation reference the new XDG path as the primary location
- All existing tests pass; new tests cover the three XDG/legacy/neither discovery cases
</summary>

<objective>
Migrate dark-factory's global config file discovery from a hardcoded `~/.dark-factory/config.yaml` path to XDG-first (`~/.config/dark-factory/config.yaml`) with automatic legacy fallback, so the tool follows the XDG Base Directory Specification for user config files.
</objective>

<context>
Read `pkg/globalconfig/globalconfig.go` — the current `Load()` and `FileExists()` both construct `filepath.Join(home, ".dark-factory", "config.yaml")` directly.

Read `pkg/globalconfig/globalconfig_internal_test.go` — tests override `userHomeDir` to a temp dir and create `tmpDir/.dark-factory/config.yaml` for setup. These tests need to be adapted to also cover the XDG-first and legacy-fallback scenarios.

Read `pkg/globalconfig/env_test.go` — uses `setenv("HOME", tmpHome)` with `tmpHome/.dark-factory/config.yaml`. Needs the same adaptation.

Read `main.go` — the `config` subcommand help text (around lines 775 and 882) mentions `.dark-factory.yaml` (project config, not global). Keep those lines as-is; the help text we're updating is about the global config path, which appears in docs, not in main.go's help strings.

Read `docs/configuration.md` — has many references to `~/.dark-factory/config.yaml`. These need updating to mention `~/.config/dark-factory/config.yaml` as the primary path.
</context>

<requirements>

1. Add `FindConfigDir(toolName string) (string, error)` to `pkg/globalconfig/globalconfig.go`.

   Contract:
   - Takes a tool name (e.g. `"dark-factory"`).
   - Returns the absolute path to the config directory for that tool.
   - Uses `userHomeDir()` (the existing package-level variable, so tests can override) to resolve `~`.
   - Decision logic:
     a. `~/.config/<tool>/` exists (stat success) → return that path
     b. `~/.<tool>/` exists (stat success) → return legacy path
     c. Neither exists → return `~/.config/<tool>/` (XDG default, do not create it)
   - Returns error only if `userHomeDir` fails; stat failures on missing dirs are swallowed (they trigger fallthrough, not error).
   - Add GoDoc comment explaining the XDG-first semantics.

2. Update GoDoc comments and `fileLoader.Load(ctx)` in `pkg/globalconfig/globalconfig.go`:

   - Update the GoDoc comment on `GlobalConfig` (line 51) from `~/.dark-factory/config.yaml` to reference the XDG-first path with legacy fallback.
   - Update the GoDoc comment on `NewLoader` (line 120) similarly.
   - Update the GoDoc comment on `fileLoader` (line 125) similarly.
   - Replace `home, err := userHomeDir()` + `filepath.Join(home, ".dark-factory", "config.yaml")` in `Load()` with a call to `FindConfigDir("dark-factory")` + `filepath.Join(dir, "config.yaml")`.
   - Remove the now-unused `home` variable.
   - Keep all existing behavior: permission warning, read, parse, merge, validate — only the path construction changes.

3. Update `FileExists(ctx)` in `pkg/globalconfig/globalconfig.go`:

   - Replace the hardcoded `filepath.Join(home, ".dark-factory", "config.yaml")` with `FindConfigDir("dark-factory")` + `"/config.yaml"`.
   - Keep the same return semantics (false,nil for missing; false,err for home-dir error; true,nil for present).
   - Update doc comment to reference the XDG-first path.

4. Update tests in `pkg/globalconfig/globalconfig_internal_test.go`:

   - `writeConfig` helper: create config under `tmpDir/.config/dark-factory/config.yaml` (XDG path) instead of `tmpDir/.dark-factory/config.yaml`. Add a second helper or variant that creates under the legacy `tmpDir/.dark-factory/config.yaml` for fallback tests. Or better: update `writeConfig` to take an optional `legacy bool` parameter, or add a separate `writeLegacyConfig` helper.
   - Add new test cases:
     - "prefers XDG path when it exists" — write config at `~/.config/dark-factory/config.yaml`, verify `Load()` reads it (not the legacy path).
     - "falls back to legacy path when XDG does not exist" — write config at `~/.dark-factory/config.yaml` only, verify `Load()` reads it.
     - "returns XDG path when neither exists (defaults)" — verify `Load()` returns defaults (no error) when no config dir exists at either location.
   - Add corresponding `FileExists` test cases:
     - "returns true when file exists at XDG path"
     - "returns true when file exists at legacy path only"
     - "returns false when file exists at neither"
   - All existing tests must still pass after the migration.

5. Update tests in `pkg/globalconfig/env_test.go`:
   - Update `setupHome` to create config at `tmpHome/.config/dark-factory/config.yaml` (XDG path).
   - Update the permission warning test assertion to expect the new XDG path.

6. Update `docs/configuration.md`:
   - Replace all `~/.dark-factory/config.yaml` references with `~/.config/dark-factory/config.yaml` as the primary path.
   - Add a note that `~/.dark-factory/config.yaml` (legacy) is still read as a fallback if no XDG config exists.
   - Update the "setting up global config" example at line ~570 to use `mkdir -p ~/.config/dark-factory`.
   - Update the permission warning text to reference the new path.

7. Update `docs/config-layering.md`:
   - Replace `~/.dark-factory/config.yaml` with `~/.config/dark-factory/config.yaml` as the primary path.
   - Add a note about legacy fallback.

8. Update `docs/release-process.md`:
   - Update the `~/.dark-factory/config.yaml` reference in the sandbox note (line 33).

9. Update `docs/running.md`:
   - Update the `~/.dark-factory/config.yaml` reference (line 46).
   - Keep the XDG-first note consistent.
   - Do NOT touch `~/.dark-factory/healthcheck-cache/` references — that's state, not config.

10. Update `README.md`:
    - Line 154: Update `~/.dark-factory/config.yaml` to `~/.config/dark-factory/config.yaml` (XDG) with legacy fallback note.
    - Line 156: Same update for the `autoGeneratePrompts` reference.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change `pkg/containerlock/containerlock.go` — the lock is runtime state at `~/.dark-factory/container.lock`, out of scope
- Do NOT change `pkg/factory/factory.go` line 1356 (`healthcheck-cache` path) — that's state, not config
- Do NOT change `pkg/healthcheckgate/cache.go` — state, not config
- Do NOT change `pkg/lock/locker.go` — project-level lock, not global config
- Do NOT change the `config` subcommand help text in main.go lines 775/882 — those reference `.dark-factory.yaml` (project config), not the global config path
- Existing tests must still pass
- Use `userHomeDir` (existing package-level variable) for home dir resolution — do not call `os.UserHomeDir()` directly in new code
- Error messages referencing the config path should use the resolved path, not a hardcoded string
- `FindConfigDir` must be exported (capital F) for potential future use
</constraints>

<verification>
Run `make precommit` from the worktree root — must pass.
</verification>
