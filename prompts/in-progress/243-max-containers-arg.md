---
status: approved
created: "2026-04-03T00:00:00Z"
queued: "2026-04-03T10:46:35Z"
---

<summary>
- Users can override the container limit via `--max-containers N` on `run` and `daemon` commands
- The CLI argument takes highest priority in the resolution chain: arg > project config > global config > default
- When set, the arg value is used everywhere the effective limit is needed (processing, status display)
- All existing resolution logic continues to work when the flag is not provided
</summary>

<objective>
Add `--max-containers N` flag to `dark-factory run` and `dark-factory daemon` so users can temporarily override the container limit without editing config files. The priority chain becomes: CLI arg (highest) → project `.dark-factory.yaml` → global `~/.dark-factory/config.yaml` → default (3).
</objective>

<context>
Read CLAUDE.md for project conventions.

Key files:
- `main.go` — `ParseArgs()` (~line 243) extracts flags from args. Currently extracts `-debug` and `--auto-approve`. The `run` and `daemon` commands dispatch at lines 85-88.
- `pkg/factory/factory.go` — `EffectiveMaxContainers(projectMax, globalMax int)` at line 44 resolves project vs global limit. Called at 5 sites (~lines 270, 356, 606, 658, 841). `CreateRunner()` (~line 206) and `CreateOneShotRunner()` (~line 276) are the entry points for `daemon` and `run`.
- `pkg/globalconfig/globalconfig.go` — `GlobalConfig.MaxContainers` at line 27, `DefaultMaxContainers` at line 18.

Current resolution: `EffectiveMaxContainers(cfg.MaxContainers, globalCfg.MaxContainers)` — if project > 0 use project, else use global. The arg override needs to take precedence over both.
</context>

<requirements>
1. **Extract `--max-containers N` in `ParseArgs`**:

   In the flag extraction loop (~line 247), detect `--max-containers` and consume the next arg as the value. Return it alongside the existing return values.

   Since changing `ParseArgs` return signature affects callers, instead parse `--max-containers` from `args` in `main.go` after `ParseArgs` returns but before dispatching to `run`/`daemon`. Add a helper function:

   ```go
   // extractMaxContainers removes --max-containers N from args and returns the value (0 = not set).
   func extractMaxContainers(args []string) (int, []string, error)
   ```

   This keeps `ParseArgs` signature unchanged. The helper:
   - Finds `--max-containers` in args, takes the next element as the integer value
   - Returns the parsed int and the remaining args with both elements removed
   - Returns error if value is missing, not an integer, or < 1

2. **Pass arg value through to factory functions**:

   In `main.go` `run()`, call `extractMaxContainers(args)` before the `run` and `daemon` cases. If a value was provided, set it on the config:

   ```go
   case "run", "daemon":
       maxContainers, remainingArgs, err := extractMaxContainers(args)
       if err != nil {
           return err
       }
       if maxContainers > 0 {
           cfg.MaxContainers = maxContainers
       }
   ```

   Since `cfg.MaxContainers` feeds into `EffectiveMaxContainers(cfg.MaxContainers, globalCfg.MaxContainers)`, and project max takes priority when > 0, setting `cfg.MaxContainers` from the arg achieves the correct override. No changes needed to `EffectiveMaxContainers` or any factory functions.

3. **Handle the `status` and `config` commands too**:

   Also extract `--max-containers` for `status` so the display reflects the override. For `config`, no change needed (it shows stored config, not effective runtime config).

4. **Add tests**:

   Test `extractMaxContainers`:
   - `["--max-containers", "5"]` → returns 5, empty remaining args
   - `["--max-containers", "5", "other"]` → returns 5, `["other"]`
   - `["other", "--max-containers", "5"]` → returns 5, `["other"]`
   - `[]` → returns 0, empty args (not set)
   - `["--max-containers"]` → error (missing value)
   - `["--max-containers", "abc"]` → error (not a number)
   - `["--max-containers", "0"]` → error (must be >= 1)
   - `["--max-containers", "-1"]` → error (must be >= 1)

5. **Update help text**:

   In `printHelp()` (~line 176), update the `run` and `daemon` lines:
   ```
   run [--max-containers N] [--auto-approve]    Process all queued prompts and exit
   daemon [--max-containers N]                   Watch for queued prompts and execute them
   ```

6. **Update `docs/configuration.md`**:

   Add a note in the "Per-Project Container Limit" section about the CLI override:
   ```
   ### CLI Override

   Override the limit for a single run without editing config:

   ```bash
   dark-factory run --max-containers 5
   dark-factory daemon --max-containers 1
   ```

   Priority: CLI arg > project config > global config > default (3).
   ```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- All existing tests must pass
- `make precommit` must pass
- Keep `ParseArgs` signature unchanged
- Use `github.com/bborbe/errors` for error wrapping (never `fmt.Errorf`)
- Use `strconv.Atoi` for parsing (stdlib, already imported elsewhere)
- The arg must be validated: must be >= 1, must be an integer
</constraints>

<verification>
Run `make precommit` — must pass.

Manual checks:
```bash
# Override works
dark-factory run --max-containers 1    # runs with limit 1
dark-factory status --max-containers 10  # shows Containers: N/10

# Invalid values rejected
dark-factory run --max-containers abc   # error
dark-factory run --max-containers 0     # error
dark-factory run --max-containers       # error (missing value)

# Without flag, existing behavior unchanged
dark-factory status   # shows project/global limit as before
```
</verification>
