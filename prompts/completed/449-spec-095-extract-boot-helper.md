---
status: completed
spec: [095-healthcheck-cli]
summary: Introduced BootContainerProbe in pkg/runner/probe.go (Run method, helpers buildRunArgs/startContainer/waitForRunning/verifyBoot/uniqueContainerName/truncate) with full Ginkgo+counterfeiter test coverage (94 specs pass, 83.3% coverage) and `make precommit` exits 0.
container: dark-factory-healthcheck-exec-449-spec-095-extract-boot-helper
dark-factory-version: v0.177.1
created: "2026-06-16T13:00:10Z"
queued: "2026-06-16T13:18:11Z"
started: "2026-06-16T13:18:12Z"
completed: "2026-06-16T13:42:44Z"
branch: dark-factory/healthcheck-cli
---

<summary>
- A new public Go symbol lives under `pkg/runner/` and boots a one-shot container from the configured image, then verifies it exits clean with `/workspace` writable and no privilege-regression error.
- The probe runs against a unique throwaway container name so two concurrent `dark-factory healthcheck` invocations cannot collide.
- A regression test in `pkg/runner/` covers the happy path and the boot-failure path using the existing `executor.ContainerChecker` mock infrastructure.
- The shared symbol is exported, so the upcoming `healthcheck` command package (prompt 2a) imports it directly, and the operator-facing scenario 003 markdown can be updated to point at `dark-factory healthcheck` for the boot check.
- No behavior change is introduced: nothing in `pkg/runner/` outside the new file is modified, and the existing `runHealthCheckLoop` / `startupSequence` / `oneshot` paths are untouched.
</summary>

<objective>
Introduce a single, exported, testable Go symbol under `pkg/runner/` that runs the container-boot probe: starts a throwaway container from `cfg.ContainerImage`, waits until it is running, inspects the output for the privilege-regression signature, and verifies `/workspace` is writable. The symbol is the contract that the upcoming `healthcheck` command and scenario 003 will share.
</objective>

<context>
Read these files first (paths absolute):
- `/workspace/pkg/executor/checker.go` — `ContainerChecker` interface (`IsRunning`, `WaitUntilRunning`) and its counterfeiter directive
- `/workspace/pkg/executor/executor.go` — `Executor` interface, `dockerExecutor.buildDockerCommand`, `dockerExecutor.Execute` — read the run-args shape only; do NOT modify
- `/workspace/pkg/subproc/subproc.go` — `Runner` interface (`RunWithWarnAndTimeout`) and constructor
- `/workspace/pkg/runner/health_check.go` — read for naming style; do NOT modify
- `/workspace/pkg/runner/export_test.go` — the existing `XxxForTest` re-export pattern; follow it
- `/workspace/pkg/runner/runner_test.go` — Ginkgo test setup; `mocks.ContainerChecker` lives at `/workspace/mocks/container-checker.go`
- `/workspace/scenarios/003-smoke-test-container.md` — the markdown scenario that the new helper will share
- Coding plugin docs (in-container): `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md`
</context>

<requirements>

1. Create `/workspace/pkg/runner/probe.go` (new file, package `runner`).
   - License header exactly matches `/workspace/pkg/runner/health_check.go` lines 1-3.
   - Add `// Package runner orchestrates the main dark-factory event loop.` is already in `doc.go`; do NOT modify `doc.go`.

2. Define an exported `BootContainerProbe` struct in `probe.go` with these fields (verify against `/workspace/pkg/executor/executor.go` and `/workspace/pkg/config/config.go` lines 84-127 before pinning types):
   - `ContainerImage string` — from `config.Config.ContainerImage`
   - `ProjectName string` — used as the container-name prefix
   - `ContainerChecker executor.ContainerChecker`
   - `Subproc subproc.Runner`
   - `Clock libtime.CurrentDateTimeGetter` (import as `libtime "github.com/bborbe/time"`, mirroring `/workspace/pkg/runner/lifecycle.go` line 17)
   - `ExtraMounts []config.ExtraMount` — from `config.Config.ExtraMounts`
   - `ClaudeDir string` — resolved via `config.Config.ResolvedClaudeDir()`
   - `WorkspaceDir string` — host directory the container mounts as `/workspace`; agent reads from `os.Getwd()` at call time inside the helper (no need to pass it in)

3. Define one exported method:
   ```go
   func (b *BootContainerProbe) Run(ctx context.Context) error
   ```
   - `Run` starts a one-shot `docker run --rm` from `b.ContainerImage` via `b.Subproc.RunWithWarnAndTimeout` (target timeout 30s, but do not exceed 60s). Use the same arg shape as `dockerExecutor.buildDockerCommand` (lines 474-548 of executor.go) but stripped down to what the boot probe needs:
     - `--rm --name <unique-name>`
     - `--label dark-factory.project=<b.ProjectName>`
     - `-v <b.WorkspaceDir>:/workspace`
     - `-v <b.ClaudeDir>:/home/node/.claude`
     - For each `m` in `b.ExtraMounts`: add `-v <src>:<dst>` using `executor.ResolveExtraMountSrc` is package-private — instead call the unexported logic via a small inlined helper in `probe.go` (one short `expandHostPath(src, home string) string` function), so the probe does not import the private `resolveExtraMountSrc` from `executor`. Apply the same `if m.IsReadonly() { append ":ro" }` rule (defined in `/workspace/pkg/config/config.go` lines 76-82). Skip the mount with a `slog.Debug` if `os.Stat(src)` fails (mirroring executor.go lines 533-535).
     - Image positional at the end.
   - The unique container name MUST be `b.ProjectName + "-healthcheck-boot-" + <random>`. Generate `<random>` via `crypto/rand` reading 4 bytes and hex-encoding (16 hex chars). Document inline that this matches the spec's "no collision with prompt containers" requirement and that two concurrent invocations are guaranteed distinct.
   - Inside the container, the probe command is `sh -c 'mkdir -p /workspace/.dark-factory-healthcheck && touch /workspace/.dark-factory-healthcheck/probe && rm -rf /workspace/.dark-factory-healthcheck && echo BOOT_OK'`. Pass via `docker exec` after the container reports running, NOT inline in the `docker run` command — the spec's "boots cleanly" probe needs to inspect `/workspace` writability from inside, which requires the container to be alive.
   - Sequence inside `Run`:
     1. Call `b.Subproc.RunWithWarnAndTimeout(ctx, "docker run", "docker", "run", "--rm", "--name", name, ...)` to start the container. If this returns non-zero, return `errors.Wrap(ctx, err, "start probe container")`.
     2. Call `b.ContainerChecker.WaitUntilRunning(ctx, name, 15*time.Second)` (import `time`; do not use `libtime` for `time.Second`). On error, return `errors.Wrapf(ctx, err, "probe container %s did not start", name)`.
     3. Call `b.Subproc.RunWithWarnAndTimeout(ctx, "docker exec probe", "docker", "exec", name, "sh", "-c", "mkdir -p /workspace/.dark-factory-healthcheck && touch /workspace/.dark-factory-healthcheck/probe && rm -rf /workspace/.dark-factory-healthcheck && echo BOOT_OK")`. If exit non-zero OR stdout does not contain the literal `BOOT_OK`, capture both stdout and stderr (cap each at 200 chars via a `truncate(s string, n int) string` helper in the same file) and return `errors.Errorf(ctx, "probe container boot check failed: stdout=%q stderr=%q", trunc(stdout,200), trunc(stderr,200))`. This is the failure the spec's "UID-remap regression" row in the Failure Modes table maps to.
     4. On success, return `nil`. The container is left to be cleaned up by `docker run --rm`.

4. Wrap every error path with `errors.Wrapf` from `github.com/bborbe/errors` (matching `/workspace/pkg/runner/lifecycle.go` line 14). NEVER use `fmt.Errorf`. NEVER return a bare `err` — use `errors.Wrap` or `errors.Wrapf` with a context-providing message.

5. Honor context cancellation: every external call (subproc + containerChecker) already accepts `ctx`; verify by reading `subproc.RunWithWarnAndTimeout` and `executor.ContainerChecker.WaitUntilRunning`. Do NOT add a manual `select { case <-ctx.Done(): ... }` — the underlying calls handle it.

6. Emit a structured `slog.Info` line on success: `slog.Info("container-boot probe passed", "image", b.ContainerImage, "container", name)`. Emit `slog.Error` on failure with the same fields plus `slog.Any("err", err)`. Do NOT use `log.Printf`.

7. Add the probe's container-checker dependency to the existing `export_test.go` only IF a test in another package needs it. Internal tests in `probe_test.go` (same package `runner`) do NOT need a re-export.

8. Create `/workspace/pkg/runner/probe_test.go` (new file, package `runner_test`). It MUST use Ginkgo v2 + Gomega + counterfeiter mocks, mirroring `/workspace/pkg/runner/health_check_test.go` lines 1-100.
   - Test: "returns nil on green path" — wire `&mocks.ContainerChecker{}` whose `WaitUntilRunningReturns(nil)` and `IsRunningReturns(true, nil)`, wire a real `subproc.NewRunnerWithThresholds(2*time.Second, 30*time.Second)` (or a mock), drive a stub via `ContainerImage` pointing at a test image your test does not actually pull — instead inject a fake `subproc.Runner` that returns canned success. The probe must surface success on the happy path.
   - Test: "returns error when WaitUntilRunning fails" — `mocks.ContainerChecker.WaitUntilRunningReturns(errors.Errorf(ctx, "boom"))` — assert the returned error wraps the message `"did not start"`.
   - Test: "returns error when stdout missing BOOT_OK" — same as green path but the fake `subproc.Runner` returns stdout=`""`, exit code 0. Assert error contains `"BOOT_OK"`.
   - Test: "uses a unique container name per invocation" — call `b.Run` twice in the same test, capture the two container names from the `subproc.Runner` mock's `RunWithWarnAndTimeout` calls via `RunWithWarnAndTimeoutArgsForCall(0)` / `ArgsForCall(1)`, assert the two `--name` values differ. The fake `subproc.Runner` already exists at `/workspace/mocks/subproc-runner.go` (counterfeiter directive at `pkg/subproc/subproc.go:26`); use `mocks.SubprocRunner` directly — do NOT regenerate.
   - Skip live-Docker coverage; the unit tests mock all Docker interactions.

9. The new file `probe.go` does NOT depend on Cobra, on `pkg/cmd/`, or on the `scenarios` package. Verify by running `grep -E '"github.com/bborbe/dark-factory/pkg/cmd"|"github.com/spf13/cobra"' pkg/runner/probe.go` after writing — must return empty.

10. Run `go generate ./...` once to ensure counterfeiter fakes regenerate cleanly. Then run `make test` from the project root (this is a Go code change, so precommit gate applies).

</requirements>

<constraints>
- The shape of `dark-factory doctor`'s exit semantics (0 = clean, non-zero = findings) and table layout must be preserved by `healthcheck` — operators read both outputs and the mental model must stay one model.
- `pkg/runner/health_check.go` (spec 043, periodic container-liveness check) is not modified by this spec. It is a different surface; both can co-exist.
- The container-boot helper extracted under `pkg/runner/` must remain callable from both the `healthcheck` command and the scenario-003 harness; the helper must not depend on Cobra, on the `pkg/cmd/` package, or on the scenarios package — only on `pkg/runner/` and below.
- The `.dark-factory.yaml` schema is not extended by this spec (no new fields, no new defaults).
- All new Go code conforms to the project's coding rules: `errors.Wrapf` from `github.com/bborbe/errors` for every error path; no bare `return err`; no `fmt.Errorf`; `log/slog` to stderr.
- The probe execution must respect context cancellation: `Ctrl-C` aborts within ~1s and the command exits with a non-zero code distinct from a probe failure (agent decides exact code at impl time).
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
```bash
# Inside the project root
make test
make precommit
```
Manual:
- Open `pkg/runner/probe.go` and confirm no `fmt.Errorf`, no bare `return err`, no import of `pkg/cmd` or `cobra`.
- Open `pkg/runner/probe_test.go` and confirm at least 4 It() blocks cover the four cases in requirement 8.
- Run `grep -rE '"github.com/bborbe/dark-factory/pkg/cmd"|"github.com/spf13/cobra"' pkg/runner/probe.go pkg/runner/probe_test.go` — must return empty.
</verification>
