---
status: approved
spec: [095-healthcheck-cli]
created: "2026-06-16T13:00:10Z"
queued: "2026-06-16T13:18:11Z"
branch: dark-factory/healthcheck-cli
---

<summary>
- A new `pkg/cmd/healthcheck.go` registers a `HealthcheckCommand` interface, constructor, and Cobra-friendly `Run(ctx, args)` method, mirroring `pkg/cmd/doctor.go`'s shape.
- A counterfeiter fake is generated for the new interface at `/workspace/mocks/healthcheck-command.go` and used in unit tests.
- The skeleton accepts `--no-claude` and `--help`/`-h` and routes them; unknown flags return an error.
- The four LOCAL probes (Docker daemon reachable, container image present, container boots via `pkg/runner.BootContainerProbe`, mount writable) are implemented as separate exported `Probe` interface methods under `pkg/cmd/healthcheck/`.
- The probes run in fixed order, fail-fast: a failure in any one short-circuits the rest and reports a categorized row in the stdout table.
- A unit test per probe in isolation, plus an integration-style test for the ordered sequence, covers Docker-down, image-missing, and the `--no-claude` exits-zero-on-green case.
- The new subcommand is wired into `main.go`'s `ParseArgs` switch and `printCommandHelp` so `dark-factory healthcheck --help` prints the usage and exits 0.
- The binary still builds; `make test` passes; no bare `return err` or `fmt.Errorf` appears in the diff.
</summary>

<objective>
Add the `dark-factory healthcheck` skeleton plus the four local probe implementations (Docker, image, boot, mount) and wire the subcommand through `main.go`. The Claude / gh / notifications probes are deferred to prompt 2b; this prompt ships a working command on a green sandbox with `--no-claude`.
</objective>

<context>
Read these files first (paths absolute):
- `/workspace/pkg/cmd/doctor.go` — primary shape to mirror (interface, constructor, private struct, `Run`, `*Help`, `//counterfeiter:generate` directive)
- `/workspace/pkg/cmd/doctor_test.go` — Ginkgo test pattern, mock setup
- `/workspace/pkg/executor/checker.go` — `executor.ContainerChecker` (use this for the image probe and the boot probe)
- `/workspace/pkg/executor/executor.go` — read only for `containerExecutor` types referenced by the factory
- `/workspace/pkg/subproc/subproc.go` — `subproc.Runner` is the `docker`/`docker image inspect` shim
- `/workspace/pkg/runner/probe.go` — produced by prompt 1; import and use `runner.BootContainerProbe` (pointer receiver)
- `/workspace/pkg/config/config.go` lines 84-127 — full `Config` and `NotificationsConfig`
- `/workspace/pkg/factory/factory.go` line 1125 onwards — `CreateDoctorCommand` is the closest factory pattern; mirror its structure
- `/workspace/main.go` lines 31-216 (dispatch), lines 125-147 (`printCommandHelp`), lines 1028-1063 (`printHelp`), line 1257 (`ParseArgs` switch)
- Coding plugin docs (in-container): `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-cli-guide.md`
</context>

<requirements>

1. Create `/workspace/pkg/cmd/healthcheck.go` (new file, package `cmd`).
   - License header matches `/workspace/pkg/cmd/doctor.go` lines 1-3.
   - Add `//counterfeiter:generate -o ../../mocks/healthcheck-command.go --fake-name HealthcheckCommand . HealthcheckCommand` above the interface.
   - Define `HealthcheckCommand interface { Run(ctx context.Context, args []string) error }` (mirroring `DoctorCommand` at doctor.go line 23).
   - Define `HealthcheckHelp()` that prints to `os.Stdout` the literal substring `Usage: dark-factory healthcheck` AND the literal substring `--no-claude` (the spec's AC: "`dark-factory healthcheck --help` stdout contains the literal substring `--no-claude`"). Suggested wording:
     ```
     Usage: dark-factory healthcheck [--no-claude]
     Probe the full pipeline-execution stack (Docker, image, boot, Claude, mount, gh, notifications) and exit 0 on full pass.
     Flags:
       --no-claude   Skip the Claude session probe (the only token-spending probe)
       --help, -h    Show this help
     ```
   - Define `NewHealthcheckCommand(...) HealthcheckCommand` constructor. The constructor takes ONE dependency: a `Probes` value of the new interface type defined in step 2 below. Mirror the `NewDoctorCommand` shape: private struct, public constructor, public interface, no logic in the factory.

2. Create `/workspace/pkg/cmd/healthcheck/probes.go` (new directory, new package `healthcheck`).
   - License header matches `/workspace/pkg/cmd/doctor.go` lines 1-3.
   - Define a single exported `Probe` interface:
     ```go
     type Probe interface {
         // Name returns the probe category (e.g. "docker", "image"). Stable across runs.
         Name() string
         // Run executes the probe; returns nil on success, an error otherwise.
         Run(ctx context.Context) error
     }
     ```
   - Add `//counterfeiter:generate -o ../../../mocks/healthcheck-probe.go --fake-name HealthcheckProbe . Probe` above it. (Verify the relative path by listing `mocks/` and matching the pattern in `pkg/cmd/doctor.go` line 20.)
   - Define exported constructor functions for the FOUR local probes:
     - `NewDockerProbe(subproc.Runner) Probe` — runs `docker version` via `subproc.Runner.RunWithWarnAndTimeout` with op label `"docker version"`. If the command exits non-zero or stderr contains `Cannot connect to the Docker daemon`, return `errors.Wrapf(ctx, err, "docker daemon unreachable")`. Use a 5-second warn / 10-second timeout when calling.
     - `NewImageProbe(containerImage string, subproc.Runner) Probe` — runs `docker image inspect --format={{.Id}} <image>`. If exit non-zero, return `errors.Errorf(ctx, "container image %q not present locally", containerImage)`. The `containerImage` arg is a string passed in at construction (caller resolves from `cfg.ContainerImage`).
     - `NewBootProbe(*runner.BootContainerProbe) Probe` — wraps the `BootContainerProbe` from prompt 1. The pointer type is required because `BootContainerProbe.Run` has a pointer receiver (see prompt 1 requirement 3). The probe's `Name()` returns `"boot"`. `Run` calls `b.Run(ctx)`. If it errors, return the error wrapped via `errors.Wrapf(ctx, err, "container boot probe failed")`.
     - `NewMountProbe(subproc.Runner) Probe` — runs one `docker run --rm <image> sh -c 'mkdir -p /workspace/.healthcheck-mount && touch /workspace/.healthcheck-mount/p && rm -rf /workspace/.healthcheck-mount && echo MOUNT_OK'` (mirror the boot probe's tail; reuse the same image passed via the constructor). Timeout 15s. On non-zero OR stdout missing `MOUNT_OK`, return `errors.Wrapf(ctx, err, "workspace mount not writable: stdout=%q", trunc(stdout,200))`. (Define a small unexported `truncate` helper in the same file.)
   - Each probe's `Run` must log ONE structured `slog.Info` line on success with the field `probe=<name>`, and ONE `slog.Error` line on failure with the same field plus `"error", err`. The literal `probe=docker` / `probe=image` / `probe=boot` / `probe=mount` strings MUST appear (the spec AC: "one log line per probe to stderr at INFO level" with those exact names).

3. Modify `/workspace/pkg/cmd/healthcheck.go` `Run` to:
   - Parse `args` for `--no-claude` (strips it from the list passed to the probes) and rejects any other unknown flag with `errors.Errorf(ctx, "unknown flag: %q", arg)`. **Do NOT handle `--help` / `-h` in `Run`** — `main.go` line 65 intercepts help flags BEFORE `runCommand` reaches the healthcheck case (via `containsHelpFlag(args)` → `printCommandHelp(command)`), and req 7 already wires the help dispatch in `printCommandHelp`. Duplicating it in `Run` would be dead code.
   - Execute the FOUR local probes in fixed order: docker, image, boot, mount. Iterate with a for-range; on first error, log the failure category via `slog.Error`, print a categorized table on stdout (see step 4), and return the wrapped error.
   - If all four pass, print `all probes passed` (or equivalent — exact wording agent decides) on stdout and return nil. NOTE: the Claude/gh/notifications probes will be appended in prompt 2b; the sequence in this prompt is intentionally incomplete.
   - The exit code semantics (0 on pass, non-zero on fail) are owned by `main.go`'s `os.Exit(1)` on returned error.

4. Stdout table format mirrors `pkg/cmd/doctor.go` lines 73-80:
   - On failure: one row per probed category in the order they ran, with a 2-space-indented detail line below. Example shape (the spec's AC grep requires the row pattern `^image[[:space:]].*does-not-exist` to match — that means the format must put the category on its own line, then the detail line. Adapt the doctor.go shape which prints `category\n  targets fixCommand`):
     ```
     image
       container image "claude-yolo:does-not-exist" not present locally
     ```
   - On pass: a single line `all probes passed`. Print to `os.Stdout`. The category name on its own line is the row the AC grep matches.

5. Add `pkg/cmd/healthcheck_test.go` (new file, package `cmd_test`) using Ginkgo v2 + Gomega, mirroring `/workspace/pkg/cmd/doctor_test.go` lines 1-100.
   - At least the following `It` blocks:
     - "returns error for unknown flag" — args `["--unknown"]`, expect error containing `unknown flag`
     - "exits 0 on all probes pass" — wire four `&mocks.HealthcheckProbe{}` whose `NameReturns("docker"/"image"/"boot"/"mount")` and `RunReturns(nil)`; expect nil error
     - "exits non-zero on first probe failure" — first probe returns `errors.Errorf(ctx, "boom")`; remaining probes' `Run` MUST NOT be called (assert via `RunCallCount()`)
     - "stops at first failure and reports category in stdout" — first probe returns error; capture stdout via redirecting `os.Stdout` (or via the table render path); assert output contains the failing probe's `Name()` on its own line
     - "respects --no-claude" — the arg is stripped before iteration (assert by passing `--no-claude` plus a probe whose `Name()` returns `"claude"`; in this prompt the Claude probe does NOT exist yet, so the test asserts only that `--no-claude` is consumed without error and the four local probes still run)

6. Add `pkg/cmd/healthcheck/probes_test.go` (new file, package `healthcheck_test`) using Ginkgo v2 + Gomega. At least one `It` per local probe, exercising the success path with a fake `subproc.Runner` and the failure path with a fake that returns non-zero exit. The `BootContainerProbe` integration is covered by the `runner/probe_test.go` from prompt 1 — do NOT duplicate it here.

7. Wire the new command into `/workspace/main.go`:
   - Add `"healthcheck"` to the top-level command list in `ParseArgs` (line 1257, the switch returning `debug, command, "", rest, ...`).
   - Add a `case "healthcheck":` arm in `runCommand` (line 167 onwards). It calls `factory.CreateHealthcheckCommand(ctx, cfg, currentDateTimeGetter).Run(ctx, args)`. No config validation required (the command is read-only and does not require `cfg.Validate`).
   - Add `case "healthcheck": cmd.HealthcheckHelp()` in `printCommandHelp` (line 125).
   - Add a new `case "healthcheck"` to the help table in `printHelp` (line 1028), with the same `usage` column shape as the doctor row: `healthcheck [--no-claude]        Probe the full pipeline-execution stack`.

8. Wire the factory in `/workspace/pkg/factory/factory.go`. Add a new exported function:
   ```go
   func CreateHealthcheckCommand(
       ctx context.Context,
       cfg config.Config,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) cmd.HealthcheckCommand
   ```
   The function:
   - Creates a `subproc.NewRunner()` (use the default thresholds — do NOT tune them; the spec's wall-clock AC is met with defaults).
   - Creates an `executor.NewDockerContainerChecker(currentDateTimeGetter)`.
   - Creates a `&runner.BootContainerProbe{...}` with `ContainerImage: cfg.ContainerImage`, `ProjectName: cfg.ResolvedProjectOverride()`, `ContainerChecker: containerChecker`, `Subproc: subprocRunner`, `Clock: currentDateTimeGetter`, `ExtraMounts: cfg.ExtraMounts`, `ClaudeDir: cfg.ResolvedClaudeDir()`.
   - Wires the four `healthcheck.New<Name>Probe(...)` constructors into a slice in fixed order: docker, image, boot, mount. The slice is passed to `cmd.NewHealthcheckCommand`.
   - The constructor returns `cmd.NewHealthcheckCommand(...)`. The factory mirrors `CreateDoctorCommand` (factory.go:1125): **construction-only — instantiate concrete deps and pass them in. No branches, no error handling beyond what constructors return.**

9. Run `go generate ./...` once for the new counterfeiter directives.

10. Run `make test`. The build must succeed.

11. Verify the new `healthcheck` subcommand end-to-end against a green sandbox (no Claude required — `--no-claude` must work):
    ```bash
    go build -o /tmp/df-095 .
    /tmp/df-095 healthcheck --help              # exit 0, stdout contains "Usage: dark-factory healthcheck" and "--no-claude"
    /tmp/df-095 healthcheck --no-claude         # exit 0
    ```
    If Docker is unavailable in the YOLO container, the docker probe will fail; that is acceptable for this prompt as long as the command exits non-zero and the `docker` category appears in stdout.

</requirements>

<constraints>
- The shape of `dark-factory doctor`'s exit semantics (0 = clean, non-zero = findings) and table layout must be preserved by `healthcheck` — operators read both outputs and the mental model must stay one model.
- `pkg/runner/health_check.go` (spec 043, periodic container-liveness check) is not modified by this spec. It is a different surface; both can co-exist.
- The container-boot helper extracted under `pkg/runner/` must remain callable from both the `healthcheck` command and the scenario-003 harness; the helper must not depend on Cobra, on the `pkg/cmd/` package, or on the scenarios package — only on `pkg/runner/` and below.
- The `.dark-factory.yaml` schema is not extended by this spec (no new fields, no new defaults).
- All new Go code conforms to the project's coding rules: `errors.Wrapf` from `github.com/bborbe/errors` for every error path; no bare `return err`; no `fmt.Errorf`; `log/slog` to stderr.
- The probe execution must respect context cancellation: `Ctrl-C` aborts within ~1s and the command exits with a non-zero code distinct from a probe failure (agent decides exact code at impl time).
- See `docs/troubleshooting.md` for the operator-facing diagnostic flow this command slots into; the doc update lives in prompt 3.
- See `docs/running.md` for the operator-facing "how to run dark-factory" surface this command is added to.
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
```bash
make test
```
Manual:
- `git diff origin/master..HEAD -- '*.go' | grep -nE '^\+\s*return err$'` returns 0 lines.
- `git diff origin/master..HEAD -- '*.go' | grep -nE '^\+.*fmt\.Errorf'` returns 0 lines in files under `pkg/cmd/healthcheck` or `pkg/runner/probe`.
- `go build -o /tmp/df-095 . && /tmp/df-095 healthcheck --help | grep -E 'Usage: dark-factory healthcheck|--no-claude'` returns 2 lines.
- `/tmp/df-095 healthcheck --no-claude; echo $?` prints `0` against a green sandbox (with `pr: false` and no notifications).
</verification>
