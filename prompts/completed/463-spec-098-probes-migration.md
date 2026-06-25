---
status: completed
spec: [098-bug-unify-container-launch-policy]
summary: Migrated boot/mount/claude healthcheck probes from ProbeLaunchConfig to launchpolicy.Policy; removed ProbeLaunchConfig struct and launchOpts helper; rewired factory to use launchpolicy.NewPolicy; added DescribeTable test asserting each probe's argv carries --cap-add=NET_ADMIN and --cap-add=NET_RAW.
container: dark-factory-unify-container-launch-exec-463-spec-098-probes-migration
dark-factory-version: v0.182.0
created: "2026-06-24T19:30:00Z"
queued: "2026-06-24T19:40:37Z"
started: "2026-06-24T21:11:44Z"
completed: "2026-06-24T21:19:05Z"
---

<summary>

- Migrates the healthcheck probes (boot, mount, claude) to derive their per-invocation `ContainerLaunchOpts` from the shared `launchpolicy.Policy` introduced in prompt 1.
- Removes the `launchOpts` helper in `pkg/cmd/healthcheck/probes.go` (which composed `executor.ContainerLaunchOpts{...}` directly without `CapAdd`) — the probes now call `policy.BuildOpts(extras)` and pass the result straight to `executor.BuildDockerRunArgs`.
- Removes the `ProbeLaunchConfig` struct (the probe-specific intermediate that omitted `CapAdd`) and updates `NewBootProbe` / `NewClaudeProbe` / `NewMountProbe` to accept a `launchpolicy.Policy` directly.
- Wires the factory: `CreateHealthcheckCommand` constructs ONE `launchpolicy.Policy` from `cfg` and passes it to all three container probes.
- Adds unit tests asserting each probe's assembled argv contains `--cap-add=NET_ADMIN` and `--cap-add=NET_RAW` (spec AC `TestProbeArgvContainsCaps`).
- After this prompt: `grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go | wc -l` returns 1 (only `pkg/launchpolicy/policy.go`'s `BuildOpts`). The probes' docstring "share the same launch path production uses" is now true by construction.

</summary>

<objective>
Migrate the boot / mount / claude probes to consume `launchpolicy.Policy` instead of `ProbeLaunchConfig`. Remove the probe-specific `launchOpts` helper and the `ProbeLaunchConfig` struct. Wire the factory's `CreateHealthcheckCommand` to construct the policy once and pass it to all three container probes. Add unit tests proving the assembled argv carries both canonical caps.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, table-driven assertions via `DescribeTable`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-architecture-patterns.md` — public type + private fields; the probes' fields rename `launch ProbeLaunchConfig` -> `policy launchpolicy.Policy`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrap`/`errors.Errorf` continue to wrap subproc errors

Read these source files end-to-end before editing:

- `/workspace/pkg/launchpolicy/policy.go` (prompt 1, possibly extended in prompt 2 with accessors) — `NewPolicy`, `Policy`, `Extras`, `BuildOpts`. THIS IS THE INPUT.
- `/workspace/pkg/cmd/healthcheck/probes.go` end-to-end. Key sections:
  - lines 82-97: `ProbeLaunchConfig` — to be REMOVED.
  - lines 180-215: `bootProbe` (struct + `NewBootProbe` + `Run`) — `launch ProbeLaunchConfig` field renamed to `policy launchpolicy.Policy`.
  - lines 218-249: `claudeProbe` — same rename.
  - lines 259-287: `mountProbe` — same rename.
  - lines 289-301: `runContainerProbeArgs` struct — `launch ProbeLaunchConfig` field renamed to `policy launchpolicy.Policy`.
  - lines 303-347: `runContainerProbe` — calls `launchOpts(a.launch, name)` and then mutates `opts.Entrypoint`/`opts.Command`. This is replaced by a direct `a.policy.BuildOpts(Extras{...})` call.
  - lines 349-368: `launchOpts` helper — REMOVED.
- `/workspace/pkg/cmd/healthcheck/probes_test.go` end-to-end. The existing `BootProbe` / `MountProbe` / `ClaudeProbe` `Describe` blocks (lines 83-220) all construct `healthcheck.ProbeLaunchConfig{...}` via a `testLaunch()` closure. These ALL need to be rewritten to construct a `launchpolicy.Policy` instead. The existing `It` blocks (e.g. "returns nil when stdout contains the BOOT_OK marker") MUST still pass after the migration.
- `/workspace/pkg/factory/factory.go` `CreateHealthcheckCommand` (currently lines 1253-1310). The `ProbeLaunchConfig` construction at lines 1281-1292 is replaced by a `launchpolicy.NewPolicy(...)` call.
- `/workspace/pkg/executor/launch.go` — `ContainerLaunchOpts`, `BuildDockerRunArgs`. Argv-shape contract unchanged.

Read the parent spec:
- `/workspace/specs/in-progress/098-bug-unify-container-launch-policy.md` — Desired Behavior items 3 and 4, Acceptance Criteria rows 4 and 5, the `TestProbeArgvContainsCaps` requirement.

The factory's current probe wiring (verbatim, lines 1281-1298 of `pkg/factory/factory.go`):

```go
launch := healthcheck.ProbeLaunchConfig{
	ContainerImage: cfg.ContainerImage,
	ProjectName:    projectName,
	ProjectRoot:    projectRoot,
	ClaudeDir:      cfg.ResolvedClaudeDir(),
	Home:           home,
	Env:            env,
	ExtraMounts:    cfg.ExtraMounts,
	NetrcFile:      cfg.NetrcFile,
	GitconfigFile:  cfg.GitconfigFile,
	HideGit:        cfg.HideGit,
}
probes := cmd.Probes{
	healthcheck.NewDockerProbe(subprocRunner),
	healthcheck.NewImageProbe(cfg.ContainerImage, subprocRunner),
	healthcheck.NewBootProbe(launch, subprocRunner),
	healthcheck.NewClaudeProbe(launch, claudeRunner),
	healthcheck.NewMountProbe(launch, subprocRunner),
}
```

Note the factory's `env` map (built at lines 1270-1280) merges `cfg.Env` + a daemon-injected `ANTHROPIC_MODEL` from `cfg.Model`. That merge logic is preserved — the resulting map is passed in as the policy's `baseEnv`.

</context>

<requirements>

## 1. Remove `ProbeLaunchConfig` and `launchOpts`

In `/workspace/pkg/cmd/healthcheck/probes.go`:

1.1. DELETE the entire `ProbeLaunchConfig` struct declaration (currently lines 82-97) including its doc comment.

1.2. DELETE the entire `launchOpts` helper (currently lines 349-368) including its doc comment. After this prompt, NO function in `pkg/cmd/healthcheck/` composes `executor.ContainerLaunchOpts{...}` directly.

## 2. Reshape the three container probes

In `/workspace/pkg/cmd/healthcheck/probes.go`:

2.1. `bootProbe`: change the field `launch ProbeLaunchConfig` to `policy launchpolicy.Policy`. Update `NewBootProbe`'s signature:

```go
// NewBootProbe returns a Probe that boots a throwaway container via the
// shared launchpolicy.Policy launch path and verifies /workspace is writable.
// The probe uses the SAME mounts, env, hideGit, extraMounts, AND capabilities
// (NET_ADMIN, NET_RAW) as a production prompt container — by construction,
// not by convention. Any regression in the launch shape surfaces here.
func NewBootProbe(policy launchpolicy.Policy, r subproc.Runner) Probe {
	return &bootProbe{policy: policy, runner: r}
}
```

Update `bootProbe.Run` to pass the policy through `runContainerProbeArgs`:

```go
func (b *bootProbe) Run(ctx context.Context) error {
	return runContainerProbe(ctx, runContainerProbeArgs{
		policy:        b.policy,
		runner:        b.runner,
		category:      b.Name(),
		op:            "docker run boot probe",
		entrypoint:    "/bin/sh",
		command:       []string{"-c", bootProbeCommand},
		successMarker: "BOOT_OK",
		failurePrefix: "container boot probe failed",
	})
}
```

2.2. `claudeProbe`: same rename and signature update:

```go
func NewClaudeProbe(policy launchpolicy.Policy, r subproc.Runner) Probe {
	return &claudeProbe{policy: policy, runner: r}
}

func (c *claudeProbe) Run(ctx context.Context) error {
	return runContainerProbe(ctx, runContainerProbeArgs{
		policy:        c.policy,
		runner:        c.runner,
		category:      c.Name(),
		op:            "claude session probe",
		entrypoint:    "claude",
		command:       []string{"-p", claudeProbePrompt},
		successMarker: "OK",
		failurePrefix: "claude session probe failed",
	})
}
```

2.3. `mountProbe`: same:

```go
func NewMountProbe(policy launchpolicy.Policy, r subproc.Runner) Probe {
	return &mountProbe{policy: policy, runner: r}
}

func (m *mountProbe) Run(ctx context.Context) error {
	return runContainerProbe(ctx, runContainerProbeArgs{
		policy:        m.policy,
		runner:        m.runner,
		category:      m.Name(),
		op:            "docker run mount probe",
		entrypoint:    "/bin/sh",
		command:       []string{"-c", mountProbeCommand},
		successMarker: "MOUNT_OK",
		failurePrefix: "workspace mount not writable",
	})
}
```

## 3. Rewrite `runContainerProbe` to use `BuildOpts`

Replace `runContainerProbeArgs.launch ProbeLaunchConfig` with `policy launchpolicy.Policy`:

```go
type runContainerProbeArgs struct {
	policy        launchpolicy.Policy
	runner        subproc.Runner
	category      string
	op            string
	entrypoint    string
	command       []string
	successMarker string
	failurePrefix string
}
```

Replace `runContainerProbe`'s body — the section calling `launchOpts(a.launch, name)` and then mutating `opts.Entrypoint`/`opts.Command`:

```go
func runContainerProbe(ctx context.Context, a runContainerProbeArgs) error {
	name, err := uniqueContainerName(a.policy.ProjectName(), a.category)
	if err != nil {
		return errors.Wrap(ctx, err, "generate "+a.category+" probe container name")
	}
	opts := a.policy.BuildOpts(launchpolicy.Extras{
		ContainerName: name,
		Entrypoint:    a.entrypoint,
		Command:       a.command,
	})
	// #nosec G204 -- args are derived from the configured policy + a category-prefixed random container name, not user input
	out, err := a.runner.RunWithWarnAndTimeout(
		ctx,
		a.op,
		"docker",
		executor.BuildDockerRunArgs(opts)...)
	if err != nil {
		slog.Error(
			"healthcheck probe failed",
			"probe", a.category,
			"container", name,
			"stdout", truncate(string(out)),
			"error", err,
		)
		return errors.Errorf(ctx, "%s: stdout=%q", a.failurePrefix, truncate(string(out)))
	}
	if !strings.Contains(string(out), a.successMarker) {
		slog.Error(
			"healthcheck probe failed",
			"probe", a.category,
			"container", name,
			"stdout", truncate(string(out)),
		)
		return errors.Errorf(
			ctx,
			"%s: missing %s marker in stdout=%q",
			a.failurePrefix,
			a.successMarker,
			truncate(string(out)),
		)
	}
	slog.Info("healthcheck probe passed", "probe", a.category, "container", name)
	return nil
}
```

Notes:
- `uniqueContainerName(a.policy.ProjectName(), a.category)` uses the `ProjectName()` accessor added in prompt 2. If prompt 2 did not add this accessor, ADD it now in `pkg/launchpolicy/policy.go` (same signature: `func (p Policy) ProjectName() string { return p.projectName }`).
- The probe-specific `dark-factory.probe=<category>` label mentioned in the OLD `launchOpts` doc comment was never actually emitted (the old `launchOpts` did not set `ExtraLabels`). Preserve that behavior — do NOT add a probe label here. (If desired, the operator can grep container names: every probe container has `-healthcheck-<category>-` in its name via `uniqueContainerName`.)

Imports for `probes.go` after edits — replace any `pkg/config` import if it becomes unused after `ExtraMount` reference goes away (the import was used inside `ProbeLaunchConfig`). Run `goimports`. Add:

```go
"github.com/bborbe/dark-factory/pkg/launchpolicy"
```

Keep:
```go
"github.com/bborbe/dark-factory/pkg/executor"
```

## 4. Wire the factory

In `/workspace/pkg/factory/factory.go`, `CreateHealthcheckCommand` (currently lines 1253-1310):

4.1. Replace lines 1281-1292 (the `launch := healthcheck.ProbeLaunchConfig{...}` block) with a `launchpolicy.NewPolicy(...)` call:

```go
policy := launchpolicy.NewPolicy(
	cfg.ContainerImage,
	projectName,
	projectRoot,
	cfg.ResolvedClaudeDir(),
	home,
	env, // already merged: cfg.Env + ANTHROPIC_MODEL injection above
	cfg.ExtraMounts,
	cfg.NetrcFile,
	cfg.GitconfigFile,
	cfg.HideGit,
)
```

4.2. Update the probe constructors (lines 1296-1298) to pass `policy` instead of `launch`:

```go
healthcheck.NewBootProbe(policy, subprocRunner),
healthcheck.NewClaudeProbe(policy, claudeRunner),
healthcheck.NewMountProbe(policy, subprocRunner),
```

4.3. The `env` map preparation (lines 1270-1280) is unchanged — that logic resolves the same daemon-injected `ANTHROPIC_MODEL` value the executor's prompt-run path provides. It is the policy's `baseEnv`.

Imports: add `"github.com/bborbe/dark-factory/pkg/launchpolicy"` if not already present (prompt 2 added it to factory.go for the executor wiring; if so, the import is already there).

## 5. Update existing probe tests

In `/workspace/pkg/cmd/healthcheck/probes_test.go`:

5.1. Replace the `testLaunch()` closure inside `BootProbe`, `MountProbe`, `ClaudeProbe` `Describe` blocks (lines 92-100, 131-139, 179-187) with a `testPolicy()` closure:

```go
testPolicy := func() launchpolicy.Policy {
	return launchpolicy.NewPolicy(
		"alpine:latest",                              // containerImage
		"test-proj",                                  // projectName (or "myproject" in the ClaudeProbe block — match the existing per-block value)
		"/tmp",                                       // projectRoot
		"/tmp",                                       // claudeDir
		"/tmp",                                       // home
		nil,                                          // baseEnv
		nil,                                          // extraMounts
		"",                                           // netrcFile
		"",                                           // gitconfigFile
		false,                                        // hideGit
	)
}
```

(Keep each `Describe` block's existing per-block project name — `"test-proj"` for boot/mount, `"myproject"` for claude — so the existing assertions on container-name shape continue to hold.)

5.2. Update every probe-construction line: `healthcheck.NewBootProbe(testLaunch(), subprocR)` -> `healthcheck.NewBootProbe(testPolicy(), subprocR)`. Same for `NewMountProbe` and `NewClaudeProbe`.

5.3. The existing assertions on `args` (e.g. `Expect(args).To(ContainElement("alpine:latest"))`, `Expect(args).To(ContainElement("--entrypoint"))`, etc.) MUST still pass — the policy produces the same argv shape.

5.4. Add the import to `probes_test.go`:
```go
"github.com/bborbe/dark-factory/pkg/launchpolicy"
```

Drop `"github.com/bborbe/dark-factory/pkg/config"` if it becomes unused after the `ProbeLaunchConfig` reference goes away. (`goimports` handles this.)

## 6. New test: `TestProbeArgvContainsCaps`

Add to `/workspace/pkg/cmd/healthcheck/probes_test.go` a NEW `Describe` block named `"probe argv carries the canonical caps"` covering all three container probes. Use `DescribeTable` for compactness:

```go
var _ = Describe("probe argv carries the canonical caps", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})

	testPolicy := func() launchpolicy.Policy {
		return launchpolicy.NewPolicy(
			"alpine:latest", "test-proj", "/tmp", "/tmp", "/tmp",
			nil, nil, "", "", false,
		)
	}

	DescribeTable(
		"the assembled argv contains both NET_ADMIN and NET_RAW",
		func(probeName string, newProbe func(launchpolicy.Policy, subproc.Runner) healthcheck.Probe, successMarker string) {
			subprocR := &mocks.SubprocRunner{}
			subprocR.RunWithWarnAndTimeoutReturns([]byte(successMarker+"\n"), nil)
			p := newProbe(testPolicy(), subprocR)
			Expect(p.Run(ctx)).To(Succeed())
			_, _, name, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
			Expect(name).To(Equal("docker"))
			Expect(args).To(ContainElement("--cap-add=NET_ADMIN"))
			Expect(args).To(ContainElement("--cap-add=NET_RAW"))
		},
		Entry("boot probe",   "boot",   healthcheck.NewBootProbe,   "BOOT_OK"),
		Entry("mount probe",  "mount",  healthcheck.NewMountProbe,  "MOUNT_OK"),
		Entry("claude probe", "claude", healthcheck.NewClaudeProbe, "OK"),
	)
})
```

This is the test the spec AC names — it asserts each probe's assembled argv contains both canonical caps. The test name in the `Describe` line should make the cap assertion grep-discoverable; the spec mentions `TestProbeArgvContainsCaps` (which is the test FUNCTION name in raw `testing` style — in Ginkgo the test function is `TestHealthcheck`, but the spec's AC text says "or DescribeTable equivalent" — this is the equivalent).

Required imports in `probes_test.go`:
```go
"github.com/bborbe/dark-factory/pkg/cmd/healthcheck"
"github.com/bborbe/dark-factory/pkg/launchpolicy"
"github.com/bborbe/dark-factory/pkg/subproc"
"github.com/bborbe/dark-factory/mocks"
```

(Most are already imported by the file; `launchpolicy` is the only addition from this prompt's table.)

## 7. Verify the architectural invariants

After this prompt:

```bash
cd /workspace
grep -rn "NET_ADMIN" pkg/ | grep -v _test.go
# expected: 1 line — pkg/launchpolicy/policy.go (CanonicalCaps)

grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go
# expected: 1 line — pkg/launchpolicy/policy.go (BuildOpts method body)

grep -n "ContainerLaunchOpts{" pkg/cmd/healthcheck/probes.go
# expected: 0 lines

grep -n "ProbeLaunchConfig" pkg/ -r
# expected: 0 lines anywhere (production code, tests, factory)

grep -n "launchOpts" pkg/cmd/healthcheck/probes.go
# expected: 0 lines
```

These are the spec's AC rows 1-4 — they all flip from FAIL to PASS in this prompt.

## 8. Coverage and lint

`pkg/cmd/healthcheck` coverage must stay >= 80%. Run:

```bash
cd /workspace && go test -coverprofile=/tmp/cover.out ./pkg/cmd/healthcheck/... && go tool cover -func=/tmp/cover.out | tail -1
```

`make precommit` MUST exit 0.

</requirements>

<constraints>

- `BuildDockerRunArgs` signature unchanged (spec 098 Constraint).
- The `docker version` probe (`dockerProbe`) and `docker image inspect` probe (`imageProbe`) are NOT touched (spec 098 Non-goal "Do NOT add capabilities to the `docker version` or `docker image inspect` probes — those do not invoke `claude-yolo`"). Their bodies, struct, and constructors stay verbatim.
- The `gh auth status` probe (`ghProbe`) and notifications probe (`notificationsProbe`) are NOT touched.
- After this prompt: `grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go | wc -l` returns 1 (`pkg/launchpolicy/policy.go`). The grep for `NET_ADMIN` also returns 1. Both spec ACs flip to PASS.
- The probes' existing `Describe` blocks (BootProbe, MountProbe, ClaudeProbe) — the `It` blocks like "returns nil when stdout contains the BOOT_OK marker" — MUST continue to pass. The only change to those blocks is the `testLaunch()` -> `testPolicy()` rewrite. Do NOT delete existing `It` blocks; do NOT change their assertions outside the helper rename.
- The new `Describe("probe argv carries the canonical caps", ...)` block ADDS coverage; it does not replace existing tests.
- The cap set is exactly `{NET_ADMIN, NET_RAW}` — sourced from `launchpolicy.CanonicalCaps` (spec 098 Non-goal).
- Errors wrapped with `bborbe/errors` (`errors.Wrap`, `errors.Errorf`) — never `fmt.Errorf`, never `context.Background()` in pkg/.
- BSD-style license header on every modified file.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

```bash
cd /workspace

# 1. Compilation + tests
go test ./pkg/cmd/healthcheck/... ./pkg/factory/... ./pkg/launchpolicy/...
# expected: PASS

# 2. The full architectural-invariant grep — all spec ACs land here
grep -rn "NET_ADMIN" pkg/ | grep -v _test.go
# expected: 1 line

grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go
# expected: 1 line (BuildOpts in launchpolicy)

grep -n "NET_ADMIN" pkg/executor/executor.go
# expected: 0 lines

grep -n "ContainerLaunchOpts{" pkg/cmd/healthcheck/probes.go
# expected: 0 lines

grep -rn "ProbeLaunchConfig" pkg/
# expected: 0 lines

# 3. The new probe-argv-cap test
go test -v ./pkg/cmd/healthcheck/... -run 'probe argv' 2>&1 | tail -10
# expected: PASS for boot/mount/claude

# 4. Coverage
go test -coverprofile=/tmp/cover.out ./pkg/cmd/healthcheck/... && go tool cover -func=/tmp/cover.out | tail -1
# expected: >= 80%

# 5. Full precommit
make precommit
# expected: exit 0
```

</verification>
