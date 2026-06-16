---
status: completed
spec: [096-healthcheck-on-daemon-startup]
container: dark-factory-healthcheck-startup-exec-455-spec-096-daemon-wiring
dark-factory-version: v0.180.2
created: "2026-06-16T20:10:00Z"
queued: "2026-06-16T20:22:17Z"
started: "2026-06-16T20:45:12Z"
completed: "2026-06-16T21:04:15Z"
branch: dark-factory/healthcheck-on-daemon-startup
---

<summary>
- Wires the healthcheck startup gate into the daemon so it runs once at startup, after the existing preflight check and before the prompt-watch loop begins.
- A gate failure exits the daemon non-zero with a category-naming cause, exactly like the existing preflight terminal-failure policy.
- Adds a `--skip-healthcheck` CLI flag that bypasses the gate for one invocation, with no cache read or write; the flag works in any position on the command line.
- When the gate is disabled in config (`healthcheckEnabled: false`), the daemon short-circuits straight past it into the watch loop.
- The flag also surfaces in the `daemon` (and `run`) help text and is rejected on commands where it makes no sense.
- The one-shot `run` command is deliberately NOT given a startup gate — this spec scopes the gate to `daemon` only.
- The existing preflight gate, its cache, and `--skip-preflight` are untouched.
</summary>

<objective>
Wire the `healthcheckgate.Gate` (built in prompt 2) into `dark-factory daemon`: the factory constructs the gate reusing the existing healthcheck-probe builder; the daemon runs it after the preflight check and before the watch loop; a `--skip-healthcheck` CLI flag (position-agnostic) bypasses it; `healthcheckEnabled: false` short-circuits it. Gate failure is terminal (non-zero exit). `run` (one-shot) is out of scope.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.

Read these files fully before editing:
- `/workspace/pkg/runner/runner.go` — `NewRunner(...)` constructor, the `runner` struct, `Run(ctx)` (note the call to `r.runStartupPreflight(ctx)` at "Startup preflight" and that it returns `processor.ErrPreflightFailed` on failure), and the `runStartupPreflight` helper. The gate runs in the same spot, immediately AFTER preflight.
- `/workspace/pkg/factory/factory.go` — `CreateRunner(ctx, cfg, ver, skipPreflight, sources, currentDateTimeGetter)` builds the daemon `Runner` and calls `runner.NewRunner(...)`. Also `CreateHealthcheckCommand(ctx, cfg, currentDateTimeGetter)` (already builds the seven-probe `cmd.HealthcheckCommand`) — REUSE it; do not rebuild probes. Note the existing preflight-checker construction block (`if cfg.PreflightCommand != "" && !skipPreflight { ... preflight.NewChecker(...) }`) — mirror its shape for the gate.
- `/workspace/main.go` — `ParseArgs(rawArgs)` returns `(bool, string, string, []string, bool, bool, string)` and extracts `--skip-preflight` in the first switch loop; `run(ctx)` destructures it; `runCommand(...)` threads `skipPreflight`; `runDaemonCommand(...)` calls `factory.CreateRunner(ctx, cfg, version.Version, skipPreflight, sources, currentDateTimeGetter)`; `printDaemonHelp()` / `printRunHelp()` list flags.
- `/workspace/parse_args_test.go` — `parseArgsResult` struct + `assertParseArgs` destructure 7 return values; `main_internal_test.go` line 81 also destructures `ParseArgs`. Both must be updated to the new arity.
- `/workspace/pkg/runner/runner_test.go` — 10 call sites of `runner.NewRunner(...)` that must be updated for the new constructor param.
- `pkg/healthcheckgate/gate.go` + `cache.go` (created in prompt 2) — `Gate.Check(ctx) error`, `NewGate(enabled, skip, interval, healthcheck, cacheKey, cache, n, projectName, currentDateTimeGetter)` (9 params; `cacheKey string` inserted before `cache Cache` per Option A below), `NewFileCache(root string)` (takes root dir — prompt 2's clean constructor-injection pattern), `CacheKey(image, project, interval)`.

Coding-plugin docs (in-container — read them):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — factory has zero business logic; `Create*` prefix; construct concrete deps and pass in.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Errorf(ctx,...)`, sentinel errors with `stderrors` alias for `errors.Is`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — counterfeiter mocks, external `_test` packages, ≥80% coverage.

Verified facts (quoted from current source):
- `ParseArgs` current signature: `func ParseArgs(rawArgs []string) (bool, string, string, []string, bool, bool, string)` and the first loop already has `case "--skip-preflight": skipPreflight = true`.
- `CreateRunner` current signature: `func CreateRunner(ctx context.Context, cfg config.Config, ver string, skipPreflight bool, sources config.FieldSources, currentDateTimeGetter libtime.CurrentDateTimeGetter) runner.Runner`.
- `NewRunner` current last three params: `..., preflightChecker preflight.Checker, logWriter io.Writer) Runner`.
- `runDaemonCommand` calls `factory.CreateRunner(ctx, cfg, version.Version, skipPreflight, sources, currentDateTimeGetter).Run(ctx)`.
- `runner.Run` returns `processor.ErrPreflightFailed` on preflight failure; `main.go` checks `stderrors.Is(runErr, preflightconditions.ErrPreflightFailed)` and logs a terminal message.
- The factory's preflight block uses `os.Getwd()` for the project root and `project.Resolve(cfg.ResolvedProjectOverride())` for the project name — reuse the same `projectName` variable already in scope in `CreateRunner`.
</context>

<requirements>
1. **Thread `--skip-healthcheck` through `ParseArgs`** in `/workspace/main.go`:
   - Add `skipHealthcheck := false` next to `skipPreflight := false`.
   - In the first `switch arg` loop, add `case "--skip-healthcheck": skipHealthcheck = true`.
   - Change the return type to append `skipHealthcheck bool` as the FINAL return value: `func ParseArgs(rawArgs []string) (bool, string, string, []string, bool, bool, string, bool)`. Append `skipHealthcheck` to EVERY `return ...` statement in `ParseArgs` (there are several early returns — update all of them; it is position-agnostic because it is stripped in the same boolean-flag loop as `--skip-preflight`).
   - Update the `run(ctx)` destructuring: `debug, command, subcommand, args, autoApprove, skipPreflight, model, skipHealthcheck := ParseArgs(filteredArgs)`.

2. **Thread `skipHealthcheck` to the daemon** in `/workspace/main.go`:
   - Add `skipHealthcheck bool` param to `runCommand(...)` and pass it from `run(ctx)`.
   - In `runCommand`, extend the `--skip-healthcheck` validity guard mirroring the existing `--skip-preflight` guard: if `skipHealthcheck` and command is not `daemon`, return `errors.Errorf(ctx, "unknown flag: --skip-healthcheck")`. (Note: `--skip-healthcheck` is valid ONLY for `daemon`, NOT `run` — `run` is out of scope per the spec. The existing `--skip-preflight` guard allows `run, daemon`; the new guard allows only `daemon`.)
   - Add `skipHealthcheck bool` param to `runDaemonCommand(...)` and pass it from `runCommand`.
   - In `runDaemonCommand`, when `skipHealthcheck` is true emit `slog.Info("healthcheck skipped via --skip-healthcheck")` BEFORE calling the runner (so the AC's stderr grep matches even if the gate itself short-circuits). Pass `skipHealthcheck` into `factory.CreateRunner(...)`.
   - Do NOT add `skipHealthcheck` to `runRunCommand` / `CreateOneShotRunner` — `run` mode is out of scope; the gate must not run in one-shot.

3. **Extend `CreateRunner`** in `/workspace/pkg/factory/factory.go`:
   - Add `skipHealthcheck bool` param after `skipPreflight bool`:
     `func CreateRunner(ctx context.Context, cfg config.Config, ver string, skipPreflight bool, skipHealthcheck bool, sources config.FieldSources, currentDateTimeGetter libtime.CurrentDateTimeGetter) runner.Runner`
   - Build the gate after the preflight-checker block, reusing `CreateHealthcheckCommand`:
     ```go
     // Healthcheck startup gate (daemon-only). Reuses the same probe sequence as the
     // `dark-factory healthcheck` CLI. Disabled gates and --skip-healthcheck are handled
     // inside the gate; the factory always constructs it (zero branching).
     cacheKey := healthcheckgate.CacheKey(cfg.ContainerImage, projectName.String(), cfg.ParsedHealthcheckInterval())
     home, _ := os.UserHomeDir()
     cacheRoot := filepath.Join(home, ".dark-factory", "healthcheck-cache")
     healthcheckGate := healthcheckgate.NewGate(
         cfg.HealthcheckEnabledValue(),
         skipHealthcheck,
         cfg.ParsedHealthcheckInterval(),
         CreateHealthcheckCommand(ctx, cfg, currentDateTimeGetter),
         cacheKey,                                  // 5th param: key (inserted before cache)
         healthcheckgate.NewFileCache(cacheRoot),   // 6th param: cache (constructor takes root)
         n,
         projectName.String(),
         currentDateTimeGetter,
     )
     ```
     where `n`, `projectName`, and `cfg` are already in scope in `CreateRunner`. Imports needed: `os`, `path/filepath`, `github.com/bborbe/dark-factory/pkg/healthcheckgate`.

   - **Important cross-prompt note:** the 9-param `NewGate` signature above (with `cacheKey` inserted before `cache`) and the `NewFileCache(root string)` constructor are the prompt-2 contract — they are baked into prompt 2's spec. If prompt 2 shipped with the 8-param form or the no-arg `NewFileCache()`, that's a bug in prompt 2 and must be fixed there, not patched here. Verify the prompt 2 source before extending; do not silently amend prompt 2.
   - Pass `healthcheckGate` as a new final param to `runner.NewRunner(...)` (step 4).
   - Add the import `"github.com/bborbe/dark-factory/pkg/healthcheckgate"`.

4. **Extend `NewRunner`** in `/workspace/pkg/runner/runner.go`:
   - Add `healthcheckGate healthcheckgate.Gate` as a new final param (after `logWriter io.Writer`).
   - Add a `healthcheckGate healthcheckgate.Gate` field to the `runner` struct and assign it in `NewRunner`.
   - Add the import `"github.com/bborbe/dark-factory/pkg/healthcheckgate"`.
   - In `Run(ctx)`, immediately AFTER the existing `if err := r.runStartupPreflight(ctx); err != nil { return err }` block and BEFORE the `runners := []run.Func{...}` slice is built, call the gate:
     ```go
     // Startup healthcheck gate: verify the pipeline stack before the watcher loop begins.
     if err := r.runStartupHealthcheck(ctx); err != nil {
         return err
     }
     ```
   - Add the helper:
     ```go
     // runStartupHealthcheck runs the healthcheck startup gate before the watcher loop.
     // Returns nil when the gate is nil (not wired), disabled, skipped, a fresh cache
     // hit, or the probes pass. Returns a terminal error when the probes fail.
     func (r *runner) runStartupHealthcheck(ctx context.Context) error {
         if r.healthcheckGate == nil {
             return nil
         }
         if err := r.healthcheckGate.Check(ctx); err != nil {
             return errors.Wrap(ctx, err, "healthcheck startup gate")
         }
         return nil
     }
     ```
   - The gate's own error already has prefix `healthcheck failed:`; the outer wrap adds `healthcheck startup gate:` context for logs. The daemon exits non-zero by returning this error from `Run` — no new sentinel needed (the spec requires only a non-zero exit + a `^healthcheck failed: .+$` line in stderr, which the gate's slog.Error and returned error provide).

5. **Update `main.go` callers of the changed signatures**:
   - `runDaemonCommand` calls `factory.CreateRunner(ctx, cfg, version.Version, skipPreflight, skipHealthcheck, sources, currentDateTimeGetter).Run(ctx)`.
   - Leave `runRunCommand` / `CreateOneShotRunner` calling the UNCHANGED `CreateOneShotRunner` (no `skipHealthcheck` param added there).

6. **Help text** in `/workspace/main.go`:
   - In `printDaemonHelp()` add a line under flags: `"  --skip-healthcheck      Skip the healthcheck startup gate for this invocation (daemon only).\n"`.
   - `printHelp` (top-level): NO change. `printHelp` lists `Commands:`, not per-command flag detail — the `--skip-healthcheck` flag is documented in `printDaemonHelp` only, mirroring `--skip-preflight`.
   - Do NOT add `--skip-healthcheck` to the `run` usage/help — it is daemon-only.

7. **Update tests for the new signatures**:
   - `/workspace/parse_args_test.go`: add `skipHealthcheck bool` to `parseArgsResult`; update `assertParseArgs` to destructure 8 values and assert `skipHealthcheck`; add cases proving `--skip-healthcheck` is extracted (position-agnostic): `["daemon","--skip-healthcheck"]` and `["--skip-healthcheck","daemon"]` both yield `command:"daemon", skipHealthcheck:true`; absence yields false.
   - `/workspace/main_internal_test.go` line ~81: update the `ParseArgs` destructure to 8 values.
   - `/workspace/pkg/runner/runner_test.go`: update all 10 `runner.NewRunner(...)` call sites to pass a gate as the new final arg. For tests that should NOT exercise the gate, pass `nil` (the `runStartupHealthcheck` nil-guard makes this a no-op). Add at least ONE new test that passes a counterfeiter `healthcheckgate.Gate` fake (`mocks.HealthcheckGate`) and asserts: (a) when `Gate.Check` returns nil, `Run` proceeds past the gate (reaches the watch-loop setup / does not return the gate error); (b) when `Gate.Check` returns an error, `Run` returns a non-nil error wrapping it (i.e. the daemon would exit non-zero). Use the existing runner_test harness patterns (fake watcher/processor that return quickly or a cancelled ctx) to avoid blocking.

8. **Integration boundary test** (the spec-required test through the real wiring seam): in `pkg/factory` (external `package factory_test`), add a test that calls `CreateRunner(ctx, cfg, "test", false, false, sources, getter)` with a minimal valid `cfg` (use `config.Defaults()` with required fields set) and asserts it returns a non-nil `runner.Runner` without panicking — proving the gate-construction wiring compiles and runs end-to-end. If `CreateRunner` already has a factory test, extend it; do not duplicate a whole new suite.

9. Do NOT modify the preflight checker, `runStartupPreflight`, `preflightInterval`, or `--skip-preflight`. Do NOT add the gate to `CreateOneShotRunner` or `run` mode.
</requirements>

<constraints>
- Copied from spec: Apply only to `dark-factory daemon`. `dark-factory run` (one-shot) is out of scope — the gate must NOT run in one-shot mode.
- Copied from spec: `--skip-healthcheck` is position-agnostic and bypasses the gate for one invocation with no cache read or write.
- Copied from spec: `healthcheckEnabled: false` disables the gate entirely; the daemon proceeds directly to the watch loop.
- Copied from spec: Failure is terminal — daemon exits non-zero with a category-naming cause, matching the existing preflight-failure policy. No retry, no skip-and-continue.
- Copied from spec: `preflightCommand` / `preflightInterval` / `--skip-preflight` must remain byte-for-byte unchanged in behavior; existing preflight tests pass unmodified.
- Copied from spec: The `dark-factory healthcheck` CLI subcommand continues to exist with current behavior — this prompt adds a daemon-startup invocation path; it does not modify or remove the CLI.
- Single design decision: Option A (gate holds the precomputed cache key) is chosen. Do NOT implement Option B or leave the choice open.
- Use `errors.Wrap(ctx,...)` / `errors.Errorf(ctx,...)`. Never `fmt.Errorf`, never `context.Background()` in `pkg/`.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run in `/workspace`:
```bash
make generate   # regenerate mocks if the gate constructor arity changed
make precommit
```
(Repo is `-mod=mod`, has no `vendor/`; `make precommit` covers build, tests, lint, vet, coverage. Never pass `-mod=vendor`.)

- `make precommit` must exit 0.
- `--skip-healthcheck` is parsed position-agnostically (both `daemon --skip-healthcheck` and `--skip-healthcheck daemon`).
- A runner test with a failing `Gate.Check` fake makes `Run` return a non-nil error.
- A runner test with a passing `Gate.Check` fake (or nil gate) lets `Run` proceed past the gate.
- `CreateRunner` returns a non-nil `runner.Runner` with default config.
- Existing preflight tests pass unmodified — `git diff` shows no semantic change to preflight code; the existing `TestPreflight*` tests in `pkg/runner/` continue to pass under `make precommit`.
</verification>
