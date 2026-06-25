---
status: completed
spec: [098-bug-unify-container-launch-policy]
summary: Added pkg/launchpolicy package with Policy value type, NewPolicy constructor, BuildOpts method, Extras struct, WithCapAddForTest test seam, and CanonicalCaps var; 9 Ginkgo tests with 100% coverage; make precommit exits 0
container: dark-factory-unify-container-launch-exec-461-spec-098-launch-policy
dark-factory-version: v0.182.0
created: "2026-06-24T19:30:00Z"
queued: "2026-06-24T19:40:36Z"
started: "2026-06-24T20:45:42Z"
completed: "2026-06-24T20:49:37Z"
---

<summary>

- Introduces a single shared "launch policy" type that carries everything intrinsic to a dark-factory container: image, project identity, mounts, base env, netrc/gitconfig paths, hide-git, and the canonical capability set (`NET_ADMIN`, `NET_RAW`).
- The policy is the only place in production code where the capability set is named — no inline cap literal anywhere else after later prompts land.
- The policy exposes a per-invocation builder method that produces an `executor.ContainerLaunchOpts` value given the per-invocation extras (container name, entrypoint, command, env overlay, label overlay). After later prompts land, this builder is the ONLY non-test site that composes `ContainerLaunchOpts{...}`.
- Adds unit tests proving the produced argv contains both canonical caps, the standard mounts, and the per-invocation entrypoint/command/labels.
- Commits a docstring-level inventory enumerating every `docker run`-style call site under `pkg/` so future readers can verify the "single source of truth" property by `grep`.
- No call-site changes in this prompt — the executor and the probes still construct their own opts. Wiring happens in prompts 2 and 3.

</summary>

<objective>
Add a new `pkg/launchpolicy` package containing a `Policy` value type that captures the launch-shape inputs every dark-factory container shares (image, project, mounts, base env, netrc/gitconfig, hide-git, canonical caps `NET_ADMIN` + `NET_RAW`) plus a `BuildOpts(extras)` method that returns an `executor.ContainerLaunchOpts` ready for `executor.BuildDockerRunArgs`. Add unit tests. Do NOT touch the executor or probe call sites — wiring is prompts 2 and 3.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-architecture-patterns.md` — public type + private fields + `New*` constructor + receiver methods
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, coverage >= 80%, table-driven assertions
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — not strictly required (this prompt's code returns no errors) but read so the test scaffolding matches house style
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md` — package doc, exported-symbol doc-comments

Read these source files end-to-end before editing — they are the existing surface you must mirror:

- `/workspace/pkg/executor/launch.go` — the existing `ContainerLaunchOpts` struct (every field, in order), `BuildDockerRunArgs(opts)` (argv-shape contract), and the helper `buildHideGitArgsForRoot`. This file is the consumer of the new `Policy.BuildOpts` output. DO NOT MODIFY IT in this prompt.
- `/workspace/pkg/executor/executor.go` lines 471-518 (`buildDockerCommand`): the existing site that composes `ContainerLaunchOpts{...}` for the executor's prompt-run path. Note the inline `CapAdd: []string{"NET_ADMIN", "NET_RAW"}` literal — that is the literal moving into the policy file. DO NOT MODIFY this site in this prompt.
- `/workspace/pkg/cmd/healthcheck/probes.go` lines 82-97 (the `ProbeLaunchConfig` struct) and 349-368 (the `launchOpts` helper): the second site that composes `ContainerLaunchOpts{...}`. Note `ProbeLaunchConfig` has no `CapAdd` field at all — that is the root cause. DO NOT MODIFY this file in this prompt.
- `/workspace/pkg/factory/factory.go` lines 1267-1310 (`CreateHealthcheckCommand`): the construction of `ProbeLaunchConfig` from `cfg`. The factory wiring of the new `Policy` will happen in prompt 3. Read this so the new `Policy`'s constructor takes inputs in a shape the factory can supply.
- `/workspace/pkg/factory/factory.go` lines 673-711 (`CreateSpecGenerator`) and 920-… (`NewDockerExecutor` callers): note spec generation routes through `executor.NewDockerExecutor` — so it inherits the executor's `ContainerLaunchOpts` composition. After prompt 2 lands the cap-set carry-through is automatic for spec-gen.
- `/workspace/pkg/config/config.go` — the `config.Config` struct fields the policy needs (`ContainerImage`, `Env`, `ExtraMounts`, `NetrcFile`, `GitconfigFile`, `HideGit`, `ResolvedClaudeDir()`, `ResolvedProjectOverride()`). Look up the exact spellings before declaring the constructor signature.
- `/workspace/pkg/project/project.go` — `project.Name` type and `project.Resolve(override).String()` if used; mirror the factory's usage.

Read the parent spec end-to-end before editing:
- `/workspace/specs/in-progress/098-bug-unify-container-launch-policy.md` — `Goal`, `Non-goals`, all `Acceptance Criteria` rows, the `Desired Behavior` numbered list (especially items 1, 2, 3, 6), and the `Constraints` block.

Existing argv-shape invariants you MUST preserve (read `BuildDockerRunArgs` and confirm):
- env keys are appended sorted by key (`appendEnv`)
- extra labels are appended sorted by key (`appendExtraLabels`)
- standard mount order: `/workspace`, `/home/node/.claude`, `/home/node/.netrc`, `/home/node/.gitconfig-extra`
- `-v <projectRoot>:/workspace` is unconditional when projectRoot non-empty
- `--cap-add=<value>` for each entry in `CapAdd`, in slice order

</context>

<requirements>

## 1. New package `pkg/launchpolicy`

Create directory `/workspace/pkg/launchpolicy/`. All new files: BSD-style license header (mirror header from `/workspace/pkg/executor/launch.go`), `package launchpolicy`.

### 1.1 `/workspace/pkg/launchpolicy/policy.go`

Package-level doc comment (top of file, above `package launchpolicy`):

```
// Package launchpolicy materialises, exactly once per daemon process, the set of
// launch-shape inputs that are intrinsic to "this is a dark-factory container":
// image, project identity, mounts, base environment, netrc/gitconfig paths,
// hide-git, and the canonical Linux capability set (NET_ADMIN, NET_RAW).
//
// Two call sites consume it: the executor's prompt-run path
// (pkg/executor.dockerExecutor.buildDockerCommand) and the healthcheck probes'
// container-launch path (pkg/cmd/healthcheck.runContainerProbe). Both derive
// their executor.ContainerLaunchOpts from the same Policy value via
// Policy.BuildOpts; per-invocation differences (container name, entrypoint,
// command, env overlay, label overlay) flow through the Extras argument.
//
// A reader who wants to add a new launch-shape concern (new cap, new mount,
// new base env var) adds it here once. Both the executor and the probes pick
// it up with no further changes. The canonical container-startup-site
// inventory is maintained in the comment block on Policy itself; the
// inventory's enumerated set of file:line pairs must equal the UNION of:
//
//   grep -rnE 'exec\.Command(Context)?\(.*"docker"' pkg/ | grep -v _test.go
//   grep -rnE 'RunWithWarnAndTimeout\([^,]+,[^,]+,[^,]+"docker"' pkg/ | grep -v _test.go
//
// (Dark-factory invokes `docker` two ways: directly via `exec.Command*` and
// indirectly via `subproc.Runner.RunWithWarnAndTimeout`. The inventory must
// cover both. Any drift between the inventory and the union of the two grep
// outputs is an unresolved architectural divergence and must be fixed in the
// same change that introduced it.)
package launchpolicy
```

Declare the canonical capability set as the ONLY production reference to `NET_ADMIN` / `NET_RAW` in the codebase after later prompts:

```go
// CanonicalCaps is the Linux capability set every dark-factory container
// requires. NET_ADMIN + NET_RAW are needed by the claude-yolo entrypoint's
// init-firewall.sh (iptables rules) to run on container backends that
// reject those syscalls without explicit grants (e.g. OrbStack).
//
// This is the SINGLE production reference to the capability literals in the
// repository. The architectural invariant
//   grep -rn "NET_ADMIN" pkg/ | grep -v _test.go | wc -l
// MUST return exactly 1 (this file). Adding the literals anywhere else
// re-introduces spec-098's divergence-by-construction.
var CanonicalCaps = []string{"NET_ADMIN", "NET_RAW"}
```

Use `var` (not `const` — Go consts cannot hold slices). Document why the global is intentional in the comment above (single source of truth is the explicit point).

Declare the `Policy` type. Field set MUST be exactly:

```go
// Policy carries the launch-shape inputs intrinsic to every dark-factory
// container. Constructed once per daemon process (or per command invocation)
// from config + environment; consumed by both the executor's prompt-run path
// and the healthcheck probes.
//
// All fields are unexported. Construct via NewPolicy and consume via BuildOpts.
//
// CONTAINER-STARTUP-SITE INVENTORY (spec 098 AC "Container-startup-site
// inventory is complete"). Every production-code site that invokes
// `exec.Command(ctx, "docker", ...)` OR
// `subproc.Runner.RunWithWarnAndTimeout(ctx, op, "docker", ...)` in pkg/
// is classified below. The enumerated set MUST equal the UNION of
//   grep -rnE 'exec\.Command(Context)?\(.*"docker"' pkg/ | grep -v _test.go
//   grep -rnE 'RunWithWarnAndTimeout\([^,]+,[^,]+,[^,]+"docker"' pkg/ | grep -v _test.go
// at HEAD. CI may grep this comment to detect drift.
//
// Routed through Policy.BuildOpts (docker run / claude-yolo containers):
//   pkg/executor/executor.go:517   exec.CommandContext(ctx, "docker", args...)
//                                  -- prompt-run path (buildDockerCommand)
//                                  -- also serves spec generation via the
//                                     shared executor.NewDockerExecutor
//   pkg/cmd/healthcheck/probes.go:319 a.runner.RunWithWarnAndTimeout(
//                                       ctx, a.op, "docker",
//                                       executor.BuildDockerRunArgs(opts)...)
//                                  -- boot / mount / claude probes
//
// Explicitly out of scope (do NOT invoke claude-yolo; carry no caps, no
// mounts, no /workspace bind):
//   pkg/cmd/healthcheck/probes.go:115  "docker version"
//   pkg/cmd/healthcheck/probes.go:150  "docker image inspect --format=..."
//   pkg/executor/checker.go:72         "docker inspect --format ..."
//   pkg/executor/checker.go:105        "docker ps ..." (NewDockerContainerChecker)
//   pkg/executor/executor.go:244       "docker logs --follow <name>"
//   pkg/executor/executor.go:298       "docker stop <name>"
//   pkg/executor/executor.go:303       "docker kill <name>"
//   pkg/executor/executor.go:352       "docker stop <name>"
//   pkg/executor/executor.go:600       "docker stop <name>"
//   pkg/executor/executor.go:611       "docker rm -f <name>"
//   pkg/executor/stopper.go:32         "docker stop <name>"
//   pkg/status/status.go:546           "docker ps --filter ..."
//   pkg/status/status.go:569           "docker ps --filter ..."
//
// Out-of-scope rationale: these sites do not start a claude-yolo container;
// they query/inspect/stop/kill/log existing containers. They have no mount,
// env, capability, or hide-git surface.
type Policy struct {
	containerImage string
	projectName    string
	projectRoot    string
	claudeDir      string
	home           string
	baseEnv        map[string]string
	extraMounts    []config.ExtraMount
	netrcFile      string
	gitconfigFile  string
	hideGit        bool
	capAdd         []string
}
```

Use `pkg/config.ExtraMount` (import `github.com/bborbe/dark-factory/pkg/config`).

### 1.2 Constructor

```go
// NewPolicy returns a Policy capturing the launch-shape inputs from cfg + the
// resolved process environment (home, projectRoot). capAdd is initialised to
// CanonicalCaps; callers cannot override (see spec 098 Non-goal "Do NOT make
// capabilities configurable").
//
// projectName is the value of the dark-factory.project label.
// projectRoot is the host path mounted at /workspace.
// home is the host HOME, used for ~/ expansion in mount paths.
// baseEnv is the operator-configured env map (cfg.Env) plus daemon-injected
//   values such as ANTHROPIC_MODEL. Prompt-specific keys (YOLO_PROMPT_FILE,
//   YOLO_OUTPUT) are NOT part of the base — they are passed in via Extras
//   so unrelated invocations stay clean.
func NewPolicy(
	containerImage string,
	projectName string,
	projectRoot string,
	claudeDir string,
	home string,
	baseEnv map[string]string,
	extraMounts []config.ExtraMount,
	netrcFile string,
	gitconfigFile string,
	hideGit bool,
) Policy {
	envCopy := make(map[string]string, len(baseEnv))
	for k, v := range baseEnv {
		envCopy[k] = v
	}
	capsCopy := make([]string, len(CanonicalCaps))
	copy(capsCopy, CanonicalCaps)
	return Policy{
		containerImage: containerImage,
		projectName:    projectName,
		projectRoot:    projectRoot,
		claudeDir:      claudeDir,
		home:           home,
		baseEnv:        envCopy,
		extraMounts:    extraMounts,
		netrcFile:      netrcFile,
		gitconfigFile:  gitconfigFile,
		hideGit:        hideGit,
		capAdd:         capsCopy,
	}
}
```

Defensive copies of `baseEnv` and `capAdd` prevent a caller's later mutation of the source slice/map from corrupting the policy.

### 1.3 `Extras` and `BuildOpts`

```go
// Extras carries the per-invocation inputs that differ between the executor's
// prompt-run and the healthcheck probes' one-shot containers. Everything in
// Extras is layered ON TOP of the Policy's base launch shape.
type Extras struct {
	// ContainerName is the value passed to --name. Required.
	ContainerName string
	// Entrypoint is passed as --entrypoint <value> when non-empty.
	Entrypoint string
	// Command is appended after the image (positional container args).
	Command []string
	// EnvOverlay is merged into the Policy's base env. Keys in EnvOverlay
	// win on collision with baseEnv — the executor's prompt-specific values
	// (YOLO_PROMPT_FILE, YOLO_OUTPUT, ANTHROPIC_MODEL) are passed this way.
	EnvOverlay map[string]string
	// ExtraLabels is appended as --label KEY=VALUE flags after the project
	// label. Used e.g. by the executor's "dark-factory.prompt=<basename>"
	// label. Empty / nil leaves no extra labels.
	ExtraLabels map[string]string
}

// BuildOpts returns an executor.ContainerLaunchOpts ready for
// executor.BuildDockerRunArgs. The returned value carries the policy's
// base launch shape plus the per-invocation extras.
//
// THIS IS THE ONLY production-code site that composes
// executor.ContainerLaunchOpts{...} after spec 098 lands. The architectural
// invariant
//   grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go | wc -l
// MUST return exactly 1 (this method).
func (p Policy) BuildOpts(extras Extras) executor.ContainerLaunchOpts {
	mergedEnv := make(map[string]string, len(p.baseEnv)+len(extras.EnvOverlay))
	for k, v := range p.baseEnv {
		mergedEnv[k] = v
	}
	for k, v := range extras.EnvOverlay {
		mergedEnv[k] = v
	}
	return executor.ContainerLaunchOpts{
		ContainerName:  extras.ContainerName,
		ContainerImage: p.containerImage,
		ProjectName:    p.projectName,
		ProjectRoot:    p.projectRoot,
		ClaudeDir:      p.claudeDir,
		Home:           p.home,
		Env:            mergedEnv,
		ExtraMounts:    p.extraMounts,
		NetrcFile:      p.netrcFile,
		GitconfigFile:  p.gitconfigFile,
		HideGit:        p.hideGit,
		ExtraLabels:    extras.ExtraLabels,
		CapAdd:         p.capAdd,
		Entrypoint:     extras.Entrypoint,
		Command:        extras.Command,
	}
}
```

Imports for `policy.go`:
```go
import (
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
)
```

(`executor` imports `config`; `launchpolicy` importing both does NOT create a cycle — `launchpolicy` is the new leaf consumer of both.)

### 1.4 Optional convenience accessors

For prompt 3 (probes wiring) and prompt 4 (regression-lock test), expose a read-only accessor for the cap set so a test can inject a synthetic cap. Add at the end of `policy.go`:

```go
// WithCapAddForTest returns a copy of p with capAdd replaced by caps. Test-only
// override; production callers cannot vary the cap set (spec 098 Non-goal).
// The method name carries "ForTest" so reviewers and tooling can flag any
// non-test caller as a violation.
func (p Policy) WithCapAddForTest(caps []string) Policy {
	capsCopy := make([]string, len(caps))
	copy(capsCopy, caps)
	p.capAdd = capsCopy
	return p
}
```

This is the seam prompt 4 uses to verify "a new field in the policy reaches both call sites" without smuggling production-mutable state into `Policy`.

## 2. Unit tests

### 2.1 `/workspace/pkg/launchpolicy/launchpolicy_suite_test.go`

Standard Ginkgo bootstrap (mirror `/workspace/pkg/executor/executor_suite_test.go`). BSD header, `package launchpolicy_test`, `TestLaunchpolicy(t *testing.T)` calling `RegisterFailHandler(Fail)` then `RunSpecs(t, "Launchpolicy Suite")`.

### 2.2 `/workspace/pkg/launchpolicy/policy_test.go`

`package launchpolicy_test`, BSD header. Use Ginkgo `Describe` / `It` (not `DescribeTable` unless natural).

Helper: a constructor for a representative test policy mirroring real factory inputs:

```go
func testPolicy() launchpolicy.Policy {
	return launchpolicy.NewPolicy(
		"docker.io/bborbe/claude-yolo:v0.10.1",
		"test-project",
		"/host/project",
		"/host/.claude",
		"/host/home",
		map[string]string{"ANTHROPIC_MODEL": "sonnet"},
		nil, // ExtraMounts
		"",  // NetrcFile
		"",  // GitconfigFile
		false, // HideGit
	)
}
```

Required `It` blocks (assert by `executor.BuildDockerRunArgs(opts)` argv shape — that is the integration boundary the policy serves):

1. `"BuildOpts produces argv containing the canonical caps"`:
   - opts := testPolicy().BuildOpts(launchpolicy.Extras{ContainerName: "test-name"})
   - args := executor.BuildDockerRunArgs(opts)
   - Expect(args).To(ContainElement("--cap-add=NET_ADMIN"))
   - Expect(args).To(ContainElement("--cap-add=NET_RAW"))

2. `"BuildOpts produces argv with the standard /workspace and claude-dir mounts"`:
   - Expect(args).To(ContainElement("/host/project:/workspace"))
   - Expect(args).To(ContainElement("/host/.claude:/home/node/.claude"))

3. `"BuildOpts wires the project label from the policy"`:
   - Expect(args).To(ContainElement("dark-factory.project=test-project"))

4. `"BuildOpts wires per-invocation entrypoint and command"`:
   - extras := launchpolicy.Extras{ContainerName: "x", Entrypoint: "/bin/sh", Command: []string{"-c", "echo BOOT_OK"}}
   - Assert `--entrypoint`, `/bin/sh`, and the `-c` / `echo BOOT_OK` positional args all appear after the image positional. (Use `Expect(args).To(ContainElement(...))` for presence; index-based ordering checks optional.)

5. `"BuildOpts merges baseEnv with EnvOverlay, overlay wins on collision"`:
   - extras := launchpolicy.Extras{ContainerName: "x", EnvOverlay: map[string]string{"ANTHROPIC_MODEL": "haiku", "YOLO_PROMPT_FILE": "/tmp/prompt.md"}}
   - Expect(args).To(ContainElement("ANTHROPIC_MODEL=haiku"))    // overlay wins
   - Expect(args).To(ContainElement("YOLO_PROMPT_FILE=/tmp/prompt.md")) // overlay-only key present
   - args MUST NOT contain `ANTHROPIC_MODEL=sonnet` (the base value was overridden)

6. `"BuildOpts wires per-invocation ExtraLabels alongside the project label"`:
   - extras := launchpolicy.Extras{ContainerName: "x", ExtraLabels: map[string]string{"dark-factory.prompt": "my-prompt"}}
   - Expect(args).To(ContainElement("dark-factory.prompt=my-prompt"))
   - Expect(args).To(ContainElement("dark-factory.project=test-project"))

7. `"NewPolicy defensively copies baseEnv so caller mutation does not leak"`:
   - src := map[string]string{"K": "v1"}
   - p := launchpolicy.NewPolicy(..., src, ...)
   - src["K"] = "MUTATED"
   - opts := p.BuildOpts(launchpolicy.Extras{ContainerName: "x"})
   - args := executor.BuildDockerRunArgs(opts)
   - Expect(args).To(ContainElement("K=v1"))
   - Expect(args).NotTo(ContainElement("K=MUTATED"))

8. `"WithCapAddForTest replaces the cap set without mutating the original"`:
   - orig := testPolicy()
   - modified := orig.WithCapAddForTest([]string{"SYS_PTRACE"})
   - origArgs := executor.BuildDockerRunArgs(orig.BuildOpts(launchpolicy.Extras{ContainerName: "x"}))
   - modArgs := executor.BuildDockerRunArgs(modified.BuildOpts(launchpolicy.Extras{ContainerName: "x"}))
   - Expect(origArgs).To(ContainElement("--cap-add=NET_ADMIN"))
   - Expect(origArgs).NotTo(ContainElement("--cap-add=SYS_PTRACE"))
   - Expect(modArgs).To(ContainElement("--cap-add=SYS_PTRACE"))
   - Expect(modArgs).NotTo(ContainElement("--cap-add=NET_ADMIN"))

9. `"CanonicalCaps contains exactly NET_ADMIN and NET_RAW in that order"`:
   - Expect(launchpolicy.CanonicalCaps).To(Equal([]string{"NET_ADMIN", "NET_RAW"}))
   - Locks the spec's Non-goal "Do NOT add capabilities beyond NET_ADMIN and NET_RAW".

Coverage target: this package must reach >= 80% on its own files (`policy.go`). The `WithCapAddForTest` line is covered by test 8.

## 3. No production wiring in this prompt

Do NOT touch:
- `/workspace/pkg/executor/executor.go` (`buildDockerCommand` — prompt 2)
- `/workspace/pkg/cmd/healthcheck/probes.go` (`ProbeLaunchConfig`, `launchOpts` — prompt 3)
- `/workspace/pkg/factory/factory.go` (wiring — prompts 2 and 3)

After this prompt the production code still has TWO `ContainerLaunchOpts{...}` composition sites (executor + probes), and the inline `NET_ADMIN` literal still lives in `pkg/executor/executor.go`. That is intentional — the spec's AC `wc -l == 1` invariants land after prompts 2 + 3.

## 4. Architectural-invariant grep at end of this prompt

Document expected post-prompt-1 state in a brief test or in the verification block:

```bash
grep -rn "NET_ADMIN" pkg/ | grep -v _test.go
# expected after THIS prompt: 2 lines:
#   pkg/launchpolicy/policy.go:<line>  // CanonicalCaps doc-comment
#   pkg/launchpolicy/policy.go:<line>  // CanonicalCaps = []string{"NET_ADMIN", "NET_RAW"}
#   pkg/executor/executor.go:509       CapAdd: []string{"NET_ADMIN", "NET_RAW"},
# (still 1 line in executor.go — removal lands in prompt 2)

grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go
# expected after THIS prompt: 3 lines:
#   pkg/launchpolicy/policy.go:<line>  (BuildOpts)
#   pkg/executor/executor.go:494       (executor — removed in prompt 2)
#   pkg/cmd/healthcheck/probes.go:355  (probes — removed in prompt 3)
```

The grep totals collapse to the spec's required `1` only AFTER prompts 2 and 3 land. This prompt's verification just confirms the new package compiles and tests pass.

</requirements>

<constraints>

- Do NOT modify `pkg/executor/launch.go` (`BuildDockerRunArgs` signature is fixed per spec 098 Constraint).
- Do NOT modify `pkg/executor/executor.go` (`buildDockerCommand` migration is prompt 2; touching it here causes prompt 2 merge conflicts).
- Do NOT modify `pkg/cmd/healthcheck/probes.go` (probe migration is prompt 3).
- Do NOT modify `pkg/factory/factory.go` (wiring is prompts 2 and 3).
- Capabilities are NOT exposed in `.dark-factory.yaml` or any config surface (spec 098 Non-goal). The `Policy` constructor does NOT accept a cap-set parameter; `CanonicalCaps` is the only knob and it is a package-level `var` for documentation, not configuration.
- The cap set is exactly `{NET_ADMIN, NET_RAW}` — do NOT add SYS_PTRACE, SYS_ADMIN, or any other cap (spec 098 Non-goal). `SYS_PTRACE` appears only in test 8 of this prompt's tests as a synthetic injection via `WithCapAddForTest`.
- `WithCapAddForTest` is named `*ForTest` deliberately so any production caller is visually flagged. Do NOT add a non-test mutator that varies the cap set.
- Errors wrapped with `bborbe/errors` if any error path is added — but this prompt's code returns no errors, so the import is unneeded for `policy.go`. Tests do not need `bborbe/errors` either.
- BSD-style license header on every new file (mirror `/workspace/pkg/executor/launch.go`).
- Coverage for `pkg/launchpolicy` must be >= 80% after this prompt.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

Run from `/workspace`:

```bash
cd /workspace && go test ./pkg/launchpolicy/...
# expected: PASS, coverage line shows >= 80%
```

Architectural grep checks (interim — full collapse to `1` lands after prompts 2+3):

```bash
cd /workspace
grep -rn "NET_ADMIN" pkg/ | grep -v _test.go
# expected: 2 production-code lines (launchpolicy/policy.go: CanonicalCaps + its doc; executor/executor.go: inline literal — removed in prompt 2)

grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go
# expected: 3 production-code lines (launchpolicy/policy.go BuildOpts + executor/executor.go buildDockerCommand + cmd/healthcheck/probes.go launchOpts)

grep -n "WithCapAddForTest" pkg/ -r | grep -v _test.go
# expected: 1 line — the method declaration in pkg/launchpolicy/policy.go. NO production caller.
```

Inventory mechanical check (must hold today, hold after this prompt, and continue to hold every subsequent prompt — drift means the inventory comment is stale). The inventory covers BOTH invocation styles dark-factory uses to call `docker`:

```bash
cd /workspace

# Style 1: direct exec.Command{,Context}("docker", ...)
grep -rnE 'exec\.Command(Context)?\(.*"docker"' pkg/ | grep -v _test.go | sort

# Style 2: indirect via subproc.Runner.RunWithWarnAndTimeout(..., "docker", ...)
grep -rnE 'RunWithWarnAndTimeout\([^,]+,[^,]+,[^,]+"docker"' pkg/ | grep -v _test.go | sort

# Union of the two outputs (sorted, de-duped) must EQUAL the set of file:line
# pairs in the policy's inventory comment block (Routed + OutOfScope sections).
# Manually cross-check against the comment in pkg/launchpolicy/policy.go.
```

If a new file:line appears in either grep but is missing from the inventory comment (or vice versa), the inventory has drifted and must be updated in the same change that introduced the new call site.

Full precommit:

```bash
cd /workspace && make precommit
# expected: exit 0
```

On single-target failure (lint / errcheck / gosec / coverage), fix and re-run only that target, then re-run `make precommit` once.

</verification>
