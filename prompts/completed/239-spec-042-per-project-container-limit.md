---
status: completed
spec: [042-per-project-container-limit]
summary: Added per-project maxContainers field to .dark-factory.yaml config with validation, effectiveMaxContainers helper, updated all factory call sites, and documented in configuration.md
container: dark-factory-239-spec-042-per-project-container-limit
dark-factory-version: v0.85.0
created: "2026-04-02T09:20:00Z"
queued: "2026-04-02T09:57:22Z"
started: "2026-04-02T09:57:25Z"
completed: "2026-04-02T10:09:55Z"
---

<summary>
- Per-project `.dark-factory.yaml` supports an optional `maxContainers` field that overrides the global limit for that project's daemon
- Missing or zero `maxContainers` in the project config silently falls back to the global `~/.dark-factory/config.yaml` limit (default: 3)
- Negative `maxContainers` in the project config is rejected with a validation error at startup
- The wait-loop threshold is replaced with the effective limit (project if set, global otherwise) — the counting mechanism (docker ps, system-wide) is unchanged
- `dark-factory status` shows the effective limit in the `Containers: N/M` line, not always the global limit
- `docs/configuration.md` documents the new per-project `maxContainers` field
</summary>

<objective>
Add an optional `maxContainers` field to the per-project `.dark-factory.yaml` that overrides the global container limit for this project's daemon. The global counting mechanism is unchanged — only the threshold value changes. This lets users give high-priority projects more slots and background projects fewer.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key files to read before making changes:
- `pkg/config/config.go` — `Config` struct and `Validate()` — add `MaxContainers int` field here
- `pkg/config/loader.go` — `partialConfig` struct and `mergePartial()` — add partial field and merge logic here
- `pkg/globalconfig/globalconfig.go` — `GlobalConfig.MaxContainers` and `DefaultMaxContainers` — the global source of truth
- `pkg/factory/factory.go` — four places that pass `globalCfg.MaxContainers` to `CreateProcessor` and `status.NewChecker`; the effective limit is computed here
- `pkg/processor/processor.go` — receives `maxContainers int` in `NewProcessor`; uses it in `waitForContainerSlot` — no changes needed here
- `pkg/status/status.go` — `NewChecker` receives `maxContainers int`; uses it to populate `ContainerMax` — no changes needed here
- `docs/configuration.md` — add documentation for the new field
</context>

<requirements>
1. **Add `MaxContainers` to `pkg/config/config.go`**:
   - Add `MaxContainers int \`yaml:"maxContainers,omitempty"\`` to the `Config` struct (after `GenerateCommand`)
   - Do NOT set a default in `Defaults()` — zero means "use global"
   - Add validation to `Validate()` using the existing pattern:
     ```go
     validation.Name("maxContainers", validation.HasValidationFunc(func(ctx context.Context) error {
         if c.MaxContainers < 0 {
             return errors.Errorf(ctx, "maxContainers must not be negative, got %d", c.MaxContainers)
         }
         return nil
     })),
     ```
   - Do NOT add a `ResolvedMaxContainers` method — that logic belongs in the factory

2. **Add `MaxContainers` to `pkg/config/loader.go`**:
   - Add `MaxContainers *int \`yaml:"maxContainers,omitempty"\`` to `partialConfig`
   - Add merge logic in `mergePartial`:
     ```go
     if partial.MaxContainers != nil {
         cfg.MaxContainers = *partial.MaxContainers
     }
     ```

3. **Add tests to `pkg/config/config_test.go`** (or the existing test file, follow existing test style):
   - `maxContainers` missing → `cfg.MaxContainers == 0`, validation passes
   - `maxContainers: 0` → `cfg.MaxContainers == 0`, validation passes (zero = unset)
   - `maxContainers: 1` → `cfg.MaxContainers == 1`, validation passes
   - `maxContainers: -1` → validation returns error containing "maxContainers"
   - `maxContainers: 5` → `cfg.MaxContainers == 5`, validation passes

4. **Update `pkg/factory/factory.go`** — compute effective limit before calling `CreateProcessor` and `status.NewChecker`:

   There are currently four places in `factory.go` that use `globalCfg.MaxContainers`:

   a. Line ~260: `CreateProcessor(... globalCfg.MaxContainers)` — inside `CreateRunner`
   b. Line ~345: `CreateProcessor(... globalCfg.MaxContainers)` — inside `CreateOneShotRunner`
   c. Line ~580: `status.NewChecker(... globalCfgForServer.MaxContainers)` — inside `CreateServer` (does NOT receive `cfg`)
   d. Line ~632: `status.NewChecker(... globalCfgForStatus.MaxContainers)` — inside `CreateStatusCommand` (receives `cfg config.Config`)
   e. Line ~815: `status.NewChecker(... globalCfgForCombined.MaxContainers)` — inside `CreateCombinedStatusCommand` (receives `cfg config.Config`)

   For each of these, replace the raw `globalCfg.MaxContainers` with a helper call or inline expression:
   ```go
   effectiveMaxContainers(cfg.MaxContainers, globalCfg.MaxContainers)
   ```

   Add a package-level helper function in `pkg/factory/factory.go` (or a new `pkg/factory/util.go`):
   ```go
   // effectiveMaxContainers returns the per-project limit when set (> 0),
   // otherwise falls back to the global limit.
   func effectiveMaxContainers(projectMax, globalMax int) int {
       if projectMax > 0 {
           return projectMax
       }
       return globalMax
   }
   ```

   For cases (d), (e): `CreateStatusCommand(cfg config.Config)` and `CreateCombinedStatusCommand(cfg config.Config)` already receive `cfg` — use `effectiveMaxContainers(cfg.MaxContainers, globalCfgForStatus.MaxContainers)` etc.

   For case (c): `CreateServer` does NOT receive `cfg`. Add `projectMaxContainers int` as a new parameter to `CreateServer`. The caller must pass `cfg.MaxContainers`. Then use `effectiveMaxContainers(projectMaxContainers, globalCfgForServer.MaxContainers)` inside. Update all call sites of `CreateServer` accordingly (search for `CreateServer(` in `main.go` and factory tests).

5. **Add a test for `effectiveMaxContainers`** in `pkg/factory/` (either in an existing `_test.go` or a new `util_test.go`):
   - project=0, global=3 → 3
   - project=5, global=3 → 5
   - project=1, global=3 → 1

6. **Update `docs/configuration.md`** — find the section about global config (or container limits) and add:

   ```markdown
   ## Per-Project Container Limit

   Override the global `maxContainers` limit for a specific project by adding `maxContainers` to `.dark-factory.yaml`:

   ```yaml
   # Priority project: allow up to 5 containers
   maxContainers: 5

   # Background project: restrict to 1 container at a time
   maxContainers: 1
   ```

   | Field | Default | Purpose |
   |-------|---------|---------|
   | `maxContainers` | (global limit) | Override the system-wide container limit for this project. Missing or 0 falls back to the global limit from `~/.dark-factory/config.yaml` (default: 3). Must be ≥ 1 if set. |

   Counting remains system-wide (all running dark-factory containers across all projects). Only the threshold is per-project. Two projects both set to 5 can together exceed any single limit — this is intentional.
   ```

   Place this section after the existing global config / container limit documentation (search for `maxContainers` in the file first to find where the global setting is documented).
</requirements>

<constraints>
- Existing global `maxContainers` in `~/.dark-factory/config.yaml` continues to work as before — `globalCfg.MaxContainers` is still the source of truth when no per-project value is set
- No changes to Docker label scheme or container counting mechanism (`waitForContainerSlot` in processor, `CountRunning` in executor)
- Validation: `maxContainers` in project config must not be negative (zero is treated as "unset" and falls back to global)
- No inter-process coordination — each daemon reads its own config independently
- Do NOT commit — dark-factory handles git
- All existing tests must pass
- New code must follow `github.com/bborbe/errors` for error wrapping (never `fmt.Errorf`)
- Use `github.com/bborbe/validation` for config validation (existing pattern in `pkg/config/config.go`)
- Coverage ≥ 80% for changed packages
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm MaxContainers field parsed from project config
grep -n "MaxContainers" pkg/config/config.go pkg/config/loader.go

# Confirm effectiveMaxContainers helper exists
grep -n "effectiveMaxContainers" pkg/factory/factory.go

# Confirm status checker receives effective limit (not always global)
grep -n "effectiveMaxContainers" pkg/factory/factory.go | wc -l
# Should show 5 occurrences (one per usage site)

# Confirm docs updated
grep -n "maxContainers" docs/configuration.md
```
</verification>
