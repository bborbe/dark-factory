---
status: completed
summary: Added `env` field to Config, partialConfig, mergePartial, dockerExecutor, NewDockerExecutor, buildDockerCommand, and all factory call sites, with validation rejecting empty/reserved keys and sorted -e flags in docker command
container: dark-factory-161-optional-env-vars
dark-factory-version: v0.34.0
created: "2026-03-10T12:44:22Z"
queued: "2026-03-10T12:44:22Z"
started: "2026-03-10T12:53:43Z"
completed: "2026-03-10T13:01:28Z"
---
<summary>
- Projects can pass extra environment variables to the YOLO container
- Useful for configuring Go private module settings, proxy overrides, or build flags
- Variables are set alongside the existing prompt and model variables
- Reserved internal variables cannot be overridden (validation rejects them)
- Existing projects without the setting continue to work unchanged
</summary>

<objective>
Add an `env` field to `.dark-factory.yaml` that accepts a map of key-value pairs passed as environment variables to the YOLO container. This enables project-specific settings like `GOPRIVATE` and `GONOSUMCHECK` without modifying the container image or entrypoint.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — `Config` struct (line ~39), `Defaults()` (line ~62), `Validate()` (line ~93).
Read `pkg/config/loader.go` — `partialConfig` struct (line ~53) and `mergePartial()` (line ~109). CRITICAL: any new Config field MUST also be added to partialConfig and mergePartial, otherwise the YAML value is silently dropped.
Read `pkg/executor/executor.go` — `dockerExecutor` struct (line ~29), `NewDockerExecutor()` (line ~38), `buildDockerCommand()` (line ~185). See where existing `-e` flags are added (lines ~204-205 for YOLO_PROMPT_FILE and ANTHROPIC_MODEL).
Read `pkg/executor/executor_internal_test.go` — existing tests for `buildDockerCommand`.
Read `pkg/config/config_test.go` — existing config validation and loader tests.
Read `pkg/factory/factory.go` — `CreateSpecGenerator()` (~line 184) and `CreateProcessor()` (~line 220) both call `NewDockerExecutor()`. `CreateProcessor` is called from `CreateRunner()` (~line 102) and `CreateOneShotRunner()` (~line 156).
</context>

<requirements>
1. Add `Env map[string]string \`yaml:"env,omitempty"\`` field to `Config` struct in `pkg/config/config.go`. No default value (nil map means no extra env vars).

2. Add `Env map[string]string \`yaml:"env,omitempty"\`` to `partialConfig` in `pkg/config/loader.go`. Maps are reference types — a nil map means "not set in YAML", a non-nil map (even empty) means "explicitly set". No pointer wrapper needed.

3. Add merge logic in `mergePartial()` in `pkg/config/loader.go`:
   ```go
   if partial.Env != nil {
       cfg.Env = partial.Env
   }
   ```

4. Add config validation in `Validate()` in `pkg/config/config.go`: if `Env` is non-nil, validate that no key is empty and no key matches the reserved names `YOLO_PROMPT_FILE` or `ANTHROPIC_MODEL` (these are set internally by the executor). Use the existing `validation.Name()` pattern.

5. Add `env map[string]string` field to `dockerExecutor` struct in `pkg/executor/executor.go`. Update `NewDockerExecutor()` to accept and store it.

6. Update `buildDockerCommand()` in `pkg/executor/executor.go`: after the existing `-e` flags (YOLO_PROMPT_FILE and ANTHROPIC_MODEL), iterate the env map in sorted key order and append `-e KEY=VALUE` for each entry:
   ```go
   if len(e.env) > 0 {
       keys := make([]string, 0, len(e.env))
       for k := range e.env {
           keys = append(keys, k)
       }
       sort.Strings(keys)
       for _, k := range keys {
           args = append(args, "-e", k+"="+e.env[k])
       }
   }
   ```

7. Update all callers of `NewDockerExecutor()` in `pkg/factory/factory.go` to pass `cfg.Env`. There are two call sites:
   - `CreateSpecGenerator()` (~line 186): add `cfg.Env` to the `NewDockerExecutor()` call
   - `CreateProcessor()` (~line 249): add `env map[string]string` parameter to `CreateProcessor`'s signature (~line 220) as the last parameter (after `defaultBranch`), pass it to `NewDockerExecutor()` at line ~249, and update both callers of `CreateProcessor` — in `CreateRunner()` (~line 102) and `CreateOneShotRunner()` (~line 156) — to pass `cfg.Env`

8. Add tests:
   - Config validation: env with valid keys passes; env with empty key fails; env with reserved key `YOLO_PROMPT_FILE` fails
   - Loader: config with `env:` map loads correctly; config without env leaves it nil
   - Executor: `buildDockerCommand` with env includes `-e KEY=VALUE` flags in sorted order after existing `-e` flags; without env has no extra `-e` flags
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Env var keys must be sorted for deterministic docker commands (important for testing)
- Do NOT allow overriding YOLO_PROMPT_FILE or ANTHROPIC_MODEL — these are internal
- Do NOT set any env vars when env map is nil/empty (default behavior unchanged)
- Env values are passed as-is to the container (no variable expansion needed — values like `bitbucket.seibert.tools/*` are literal)
</constraints>

<verification>
```
make precommit
```
Must pass with no errors.
</verification>

<success_criteria>
- `env` field exists in Config struct, partialConfig, and mergePartial
- Nil/empty env = no extra env vars (default, backwards compatible)
- Configured env = sorted `-e KEY=VALUE` flags in docker command
- Empty key or reserved key fails config validation with clear error
- All existing tests pass
- New tests cover configured, unconfigured, reserved-key, and empty-key paths
</success_criteria>
