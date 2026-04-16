---
status: approved
spec: [050-factory-dedup]
created: "2026-04-16T19:50:00Z"
queued: "2026-04-16T21:01:33Z"
---

<summary>
- A single private helper constructs `status.Checker` — all three former build sites (CreateServer, CreateStatusCommand, CreateCombinedStatusCommand) call it instead of calling `globalconfig.NewLoader().Load` and `status.NewChecker` directly
- A single private helper constructs `executor.ContainerCounter` — all five former build sites call it instead of calling `executor.NewDockerContainerCounter(subproc.NewRunner())` directly
- `globalconfig.Load` is called at most once per helper invocation; no top-level factory function calls it directly for the purpose of building a status checker
- Adding a sixth container-counter call site requires one line at the call site — no new helper block
- No behavioral changes — all existing factory tests pass unchanged
- `make precommit` passes with no new lint violations
</summary>

<objective>
Eliminate duplicated construction blocks in `pkg/factory/factory.go`. Currently `globalconfig.Load` + `status.NewChecker` is copy-pasted three times, and `executor.NewDockerContainerCounter(subproc.NewRunner())` appears five times. Introduce one private helper per type so each dependency has a single construction site.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — zero-logic factory rule, Create* prefix.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/` — inject interfaces, no package-function calls from business logic.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `github.com/bborbe/errors` wrapping, never `fmt.Errorf`.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, counterfeiter mocks, ≥80% coverage for changed code.

Read `pkg/factory/factory.go` in full before making any changes. Pay attention to:

**Three status-checker construction sites (each calls `globalconfig.NewLoader().Load` + `status.NewChecker`):**
- `CreateServer` (~line 714): loads globalconfig, warns on error, calls `status.NewChecker` with `dirtyFileThreshold = 0`
- `CreateStatusCommand` (~line 760): loads globalconfig, warns on error, calls `status.NewChecker` with `cfg.DirtyFileThreshold`
- `CreateCombinedStatusCommand` (~line 946): loads globalconfig, warns on error, calls `status.NewChecker` with `cfg.DirtyFileThreshold`

**Five container-counter construction sites (all identical):**
- `CreateRunner` (~line 314): `executor.NewDockerContainerCounter(subproc.NewRunner())`
- `CreateOneShotRunner` (~line 430): `executor.NewDockerContainerCounter(subproc.NewRunner())`
- `CreateServer` (~line 729): `executor.NewDockerContainerCounter(subproc.NewRunner())`
- `CreateStatusCommand` (~line 785): `executor.NewDockerContainerCounter(subproc.NewRunner())`
- `CreateCombinedStatusCommand` (~line 971): `executor.NewDockerContainerCounter(subproc.NewRunner())`

Read `pkg/factory/factory_test.go` to understand which public functions are tested.
Read `pkg/status/checker.go` to verify the `status.NewChecker` signature.
</context>

<requirements>
1. Add a private helper `createContainerCounter` to `pkg/factory/factory.go`:

   ```go
   // createContainerCounter returns a ContainerCounter backed by docker ps.
   func createContainerCounter() executor.ContainerCounter {
       return executor.NewDockerContainerCounter(subproc.NewRunner())
   }
   ```

2. Replace every occurrence of `executor.NewDockerContainerCounter(subproc.NewRunner())` in `pkg/factory/factory.go` with a call to `createContainerCounter()`. There are five occurrences:
   - `CreateRunner` (argument to `CreateProcessor`)
   - `CreateOneShotRunner` (argument to `CreateProcessor` inside the call)
   - `CreateServer` (argument to `status.NewChecker`)
   - `CreateStatusCommand` (argument to `status.NewChecker`)
   - `CreateCombinedStatusCommand` (argument to `status.NewChecker`)

3. Add a private helper `createStatusChecker` to `pkg/factory/factory.go`. The three call sites vary in `inProgressDir`/`completedDir`/`logDir` (some from params, some from `cfg.Prompts.*`), `serverPort`, `promptManager`, `projectMax` (the project-level max containers before applying global), `dirtyFileThreshold`, and `currentDateTimeGetter`. The helper must load globalconfig internally (so no call site needs to call `globalconfig.Load` for this purpose). Use this signature:

   ```go
   // createStatusChecker loads global config and constructs a status.Checker.
   // projectMax is the project-level MaxContainers value (may be 0); effective max is resolved against global config.
   func createStatusChecker(
       ctx context.Context,
       inProgressDir, completedDir, logDir string,
       serverPort int,
       promptManager prompt.Manager,
       projectMax int,
       dirtyFileThreshold int,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) status.Checker {
       projectDir, _ := os.Getwd()
       globalCfg, err := globalconfig.NewLoader().Load(ctx)
       if err != nil {
           slog.Warn("globalconfig load failed for status checker, using default", "error", err)
           globalCfg = globalconfig.GlobalConfig{MaxContainers: globalconfig.DefaultMaxContainers}
       }
       return status.NewChecker(
           projectDir,
           inProgressDir,
           completedDir,
           logDir,
           lock.FilePath("."),
           serverPort,
           promptManager,
           createContainerCounter(),
           EffectiveMaxContainers(projectMax, globalCfg.MaxContainers),
           dirtyFileThreshold,
           currentDateTimeGetter,
           subproc.NewRunner(),
       )
   }
   ```

   Verify that `status.NewChecker` has exactly this signature before writing; adjust if it differs.

4. Replace all three inline status-checker construction blocks with calls to `createStatusChecker`:

   **In `CreateServer`** (~line 714–734): remove the local `globalCfgForServer` variable and the `status.NewChecker(...)` block; replace with:
   ```go
   statusChecker := createStatusChecker(
       ctx,
       inProgressDir, completedDir, logDir,
       port,
       promptManager,
       projectMaxContainers,
       0, // CreateServer has no dirty-file threshold — server is status-only
       currentDateTimeGetter,
   )
   ```

   **In `CreateStatusCommand`** (~line 760–793): remove `globalCfgForStatus` variable and `status.NewChecker(...)` block; replace with:
   ```go
   statusChecker := createStatusChecker(
       ctx,
       cfg.Prompts.InProgressDir,
       cfg.Prompts.CompletedDir,
       cfg.Prompts.LogDir,
       cfg.ServerPort,
       promptManager,
       cfg.MaxContainers,
       cfg.DirtyFileThreshold,
       currentDateTimeGetter,
   )
   ```

   **In `CreateCombinedStatusCommand`** (~line 963–977): remove `globalCfgForCombined` variable and `status.NewChecker(...)` block; replace with:
   ```go
   statusChecker := createStatusChecker(
       ctx,
       cfg.Prompts.InProgressDir,
       cfg.Prompts.CompletedDir,
       cfg.Prompts.LogDir,
       cfg.ServerPort,
       promptManager,
       cfg.MaxContainers,
       cfg.DirtyFileThreshold,
       currentDateTimeGetter,
   )
   ```

5. Run `make test` after completing the changes to verify nothing broke before proceeding to the next step.

6. Update `CHANGELOG.md` — append to the `## Unreleased` section (create it at the top if absent):
   ```
   - refactor: deduplicate status-checker and container-counter construction in factory
   ```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- No behavioral changes — the factory's observable output must be identical
- The factory's public API (`CreateProcessor`, `CreateRunner`, `CreateOneShotRunner`, `CreateServer`, `CreateStatusCommand`, `CreateCombinedStatusCommand`, etc.) must not change signatures or semantics
- No call site outside these two new helpers may call `globalconfig.NewLoader().Load` for the purpose of building a status checker, and no call site may call `executor.NewDockerContainerCounter` directly
- New helpers follow the existing `create*` (private) naming convention — not `Create*` (public)
- Existing factory tests must pass without modification
- Use `github.com/bborbe/errors` for any new error wrapping — never `fmt.Errorf`
- Preserve the file's top-to-bottom wiring order; do not reorder existing functions
</constraints>

<verification>
Run `make precommit` — must pass.

Confirm both helpers are the sole construction sites:
```bash
grep -c "NewDockerContainerCounter" pkg/factory/factory.go
# Expected: 1 (inside createContainerCounter)

grep -c "globalconfig.Load\|globalconfig.NewLoader" pkg/factory/factory.go
# Expected: ≤ 2 (one inside createStatusChecker, one inside CreateRunner/CreateOneShotRunner if they still load globalconfig for MaxContainers — which is separate from status checker)
```

Also verify:
```bash
grep -n "globalconfig.NewLoader" pkg/factory/factory.go
# All matches should be for globalconfig used in MaxContainers resolution (CreateRunner, CreateOneShotRunner) OR inside createStatusChecker — NOT in CreateServer/CreateStatusCommand/CreateCombinedStatusCommand bodies
```
</verification>
