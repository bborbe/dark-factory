---
status: committing
summary: Removed dead Docker-execution helpers from pkg/preflight and dropped containerImage/extraMounts params from NewChecker and its call sites in factory.go
container: dark-factory-324-preflight-cleanup-dead-container-code
dark-factory-version: v0.128.1-3-gf1cfca3-dirty
created: "2026-04-19T20:27:24Z"
queued: "2026-04-19T20:27:24Z"
started: "2026-04-19T20:28:44Z"
---

<summary>
- Remove dead container-related code from the preflight package
- Preflight now runs on host (changed in prompt 322 follow-up), so docker args builder and extra-mount helpers are unused
- Shrink NewChecker signature by dropping unused containerImage and extraMounts parameters
- Factory call sites stop passing container fields
- No behavior change — pure cleanup
</summary>

<objective>
Remove unused Docker-execution code from `pkg/preflight/preflight.go` that remained after the host-exec pivot. The preflight command runs on the host via `sh -c`; the container-related helpers and fields on `NewChecker` are dead.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `pkg/preflight/preflight.go` — the `checker` struct, `NewChecker` constructor (currently takes 7 params including `containerImage` and `extraMounts`), and the dead helpers: `buildPreflightDockerArgs`, `resolveExtraMountSrc`, `resolveHostCacheDir`, `darwinCacheDir`, `linuxCacheDir`.
Read `pkg/preflight/preflight_test.go` — `buildPreflightDockerArgs` has tests; these must be deleted with the helper.
Read `pkg/factory/factory.go` — two call sites of `preflight.NewChecker` (search for `preflight.NewChecker`) pass `cfg.ContainerImage` and `cfg.ExtraMounts`. Both must drop those arguments.
</context>

<requirements>
1. In `pkg/preflight/preflight.go`:
   - Remove fields `containerImage string` and `extraMounts []config.ExtraMount` from the `checker` struct.
   - Change `NewChecker` signature from 7 parameters to 5 by removing `containerImage string` and `extraMounts []config.ExtraMount`. Keep: `command`, `interval`, `projectRoot`, `n`, `projectName` — in that order.
   - Delete functions: `buildPreflightDockerArgs`, `resolveExtraMountSrc`, `resolveHostCacheDir`, `darwinCacheDir`, `linuxCacheDir`.
   - Remove the now-unused imports (`path/filepath`, `os` if no other user, and `github.com/bborbe/dark-factory/pkg/config` if no other user — check each).
   - Update the `NewChecker` GoDoc: drop the `containerImage` and `extraMounts` parameter descriptions.
2. In `pkg/preflight/preflight_test.go`:
   - Delete all tests for `buildPreflightDockerArgs`, `resolveExtraMountSrc`, `resolveHostCacheDir`, `darwinCacheDir`, `linuxCacheDir`.
   - Update the two `NewChecker(...)` call sites (currently passing `"img:latest", nil` for container/mounts) to the new 5-param signature. `NewCheckerWithRunner(...)` calls are unaffected — their signature does not include container image / mounts.
3. In `pkg/factory/factory.go`:
   - Both `preflight.NewChecker(...)` call sites: remove the `cfg.ContainerImage` and `cfg.ExtraMounts` arguments.
4. Do NOT touch the `Config` fields `ContainerImage` / `ExtraMounts` — they are used by the executor, not preflight.
5. Do NOT touch `scenarios/` or `.dark-factory.yaml`.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Repo-relative paths only.
- No behavior change — preflight must still run `sh -c <command>` on host with `cmd.Dir = projectRoot`.
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
