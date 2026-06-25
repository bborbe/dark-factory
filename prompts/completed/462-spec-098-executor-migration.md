---
status: completed
spec: [098-bug-unify-container-launch-policy]
summary: Migrated executor's prompt-run path to consume launchpolicy.Policy via BuildOpts; removed inline CapAdd literal from executor.go; moved ContainerLaunchOpts struct to pkg/launchpolicy (type alias in pkg/executor for callers); added ClaudeDir/BaseEnv/ContainerImage/ProjectName accessors to Policy; wired Policy through both executor.NewDockerExecutor call sites in factory.go; updated export_test.go and executor_test.go to use new constructor signature.
container: dark-factory-unify-container-launch-exec-462-spec-098-executor-migration
dark-factory-version: v0.182.0
created: "2026-06-24T19:30:00Z"
queued: "2026-06-24T19:40:36Z"
started: "2026-06-24T20:49:38Z"
completed: "2026-06-24T21:11:43Z"
---

<summary>

- Migrates the executor's prompt-run path to derive its per-invocation `ContainerLaunchOpts` from the shared `launchpolicy.Policy` introduced in prompt 1.
- Removes the inline `CapAdd: []string{"NET_ADMIN", "NET_RAW"}` literal from `pkg/executor/executor.go` — the capability set is now sourced from `Policy.CapAdd` via `Policy.BuildOpts`.
- Threads a `launchpolicy.Policy` value into the executor at construction time (`NewDockerExecutor`) — the executor stops carrying separate `containerImage`, `projectName`, `netrcFile`, `gitconfigFile`, `env`, `extraMounts`, `claudeDir`, `hideGit` fields and instead carries the policy plus the per-invocation `model` (a prompt-specific concern).
- Wires the policy through the factory: `CreateExecutor` callers (both the prompt-execution path and the spec-generation path) construct one `launchpolicy.Policy` from `cfg` + resolved environment and pass it in.
- Existing executor tests in `pkg/executor/executor_test.go` continue to pass — the cap-assertion lines (currently 451-452) keep asserting both caps, the test helper `BuildDockerCommandForTest` is updated to construct an internal `Policy` from its arguments so the test surface stays identical to callers.
- After this prompt: `grep -rn "NET_ADMIN" pkg/ | grep -v _test.go | wc -l` returns 1 (the only match is in `pkg/launchpolicy/policy.go`). The probes are still un-migrated (prompt 3).

</summary>

<objective>
Migrate the executor's prompt-run path to consume `launchpolicy.Policy` instead of composing `ContainerLaunchOpts` inline. Remove the inline cap literal from `pkg/executor/executor.go`. Wire the policy through the factory so spec generation (which uses the same `executor.NewDockerExecutor`) inherits the policy automatically. Preserve `BuildDockerRunArgs` signature, `insertPromptFileMount` behavior, `validateClaudeAuth`, and post-run commit logic exactly.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — factory composition; this prompt threads a new dependency through `executor.NewDockerExecutor`'s factory wrappers
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-architecture-patterns.md` — public constructor + private struct; the `Policy` injection follows the same shape

Read these source files end-to-end before editing:

- `/workspace/pkg/launchpolicy/policy.go` (created by prompt 1) — the `Policy` type, `NewPolicy(...)` signature, `Extras` shape, `BuildOpts(extras) executor.ContainerLaunchOpts`. THIS IS THE INPUT FOR THE MIGRATION.
- `/workspace/pkg/executor/executor.go` end-to-end, paying attention to:
  - lines 47-93: `NewDockerExecutor` constructor + `dockerExecutor` struct fields. Both shapes change in this prompt.
  - lines 99-180 (approximately): `Execute(ctx, promptContent, logFile, containerName)` — calls `buildDockerCommand` with `projectRoot`, `home` resolved at request time. Note the doc-comment-level `slog.Info(... "image", e.containerImage ...)` line — that field reference changes.
  - lines 471-518: `buildDockerCommand` — composes `ContainerLaunchOpts{...}` inline with `CapAdd: []string{"NET_ADMIN", "NET_RAW"}` at line 509. This site is rewritten.
  - lines 520-534: `insertPromptFileMount` — preserved verbatim (the prompt-file mount stays at the executor boundary per spec Constraints).
  - lines 539-…: `validateClaudeAuth` — preserved verbatim.
- `/workspace/pkg/executor/export_test.go` — `BuildDockerCommandForTest` signature: it currently accepts a long parameter list (`containerImage`, `projectName`, `model`, …) and constructs a `dockerExecutor` directly. This test helper must continue to compile and produce a `*exec.Cmd` whose `Args` slice satisfies the existing assertions in `executor_test.go`.
- `/workspace/pkg/executor/executor_test.go` — focus on lines 372-505: the `buildDockerCommand` `Describe` block. The `buildCmd` closure at line 373 wires args into `BuildDockerCommandForTest`. The cap-assertion lines 451-452 MUST pass unchanged after this prompt:
  ```go
  Expect(cmd.Args).To(ContainElement("--cap-add=NET_ADMIN"))
  Expect(cmd.Args).To(ContainElement("--cap-add=NET_RAW"))
  ```
- `/workspace/pkg/factory/factory.go` lines 670-711 (`CreateSpecGenerator` — calls `executor.NewDockerExecutor`) and lines 910-960 approximately (the second `executor.NewDockerExecutor` site — find it with `grep -n "NewDockerExecutor" pkg/factory/factory.go`, currently lines 682 and 920). BOTH call sites are updated in this prompt to construct + pass a `Policy`.
- `/workspace/pkg/config/config.go` — confirm field names: `ContainerImage`, `Env`, `ExtraMounts`, `NetrcFile`, `GitconfigFile`, `HideGit`, methods `ResolvedClaudeDir()`, `ResolvedProjectOverride()`.

Read the parent spec:
- `/workspace/specs/in-progress/098-bug-unify-container-launch-policy.md` — Goal, Desired Behavior items 2 and 6, Constraints (especially "prompt-file mount, validateClaudeAuth, and post-run commit logic unchanged"), Acceptance Criteria rows 2, 3, 6, 10.

</context>

<requirements>

## 1. Reshape `dockerExecutor` to carry a `Policy`

In `/workspace/pkg/executor/executor.go`:

1.1. Replace the field block on `dockerExecutor` (currently lines 79-93). The new shape:

```go
type dockerExecutor struct {
	policy                launchpolicy.Policy
	model                 string
	commandRunner         commandRunner
	maxPromptDuration     time.Duration // 0 = disabled
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	formatter             formatter.Formatter
}
```

Fields REMOVED from the struct (they live in the policy now):
- `containerImage`, `projectName`, `netrcFile`, `gitconfigFile`, `env`, `extraMounts`, `claudeDir`, `hideGit`

Fields RETAINED:
- `model` — a prompt-specific env value (`ANTHROPIC_MODEL`), layered via `Extras.EnvOverlay` at request time, not part of the policy's base env
- `commandRunner`, `maxPromptDuration`, `currentDateTimeGetter`, `formatter` — unchanged

Add the import:
```go
"github.com/bborbe/dark-factory/pkg/launchpolicy"
```

Remove now-unused imports if any (e.g. if `config.ExtraMount` is no longer referenced inside this file, drop the import — check with `goimports` after editing).

1.2. Rewrite `NewDockerExecutor` (currently lines 47-76) to accept a `launchpolicy.Policy` instead of the individual launch-shape fields:

```go
// NewDockerExecutor creates a new Executor using Docker. The launch shape
// (image, project, mounts, base env, netrc/gitconfig, hide-git, capabilities)
// is sourced from the shared launchpolicy.Policy — see pkg/launchpolicy.
// Prompt-specific concerns (model, max duration, formatter) remain on the
// executor itself.
func NewDockerExecutor(
	policy launchpolicy.Policy,
	model string,
	maxPromptDuration time.Duration,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	fmtr formatter.Formatter,
) Executor {
	return &dockerExecutor{
		policy:                policy,
		model:                 model,
		commandRunner:         &defaultCommandRunner{},
		maxPromptDuration:     maxPromptDuration,
		currentDateTimeGetter: currentDateTimeGetter,
		formatter:             fmtr,
	}
}
```

Note: the old constructor took 12 parameters; the new one takes 5. This is intentional — the launch-shape inputs collapse to a single `Policy` argument.

1.3. Update every `e.<removed-field>` reference inside `pkg/executor/executor.go` to read from `e.policy` via accessors OR to flow through `buildDockerCommand`'s new policy-based path. Search-and-fix sites:

- `e.containerImage` references: replace with `e.policy.ContainerImage()` (see step 1.5 for the accessor).
- `e.projectName` references: replace with `e.policy.ProjectName()`.
- The `slog.Info(... "image", e.containerImage ...)` line near line 139 and any similar log lines: switch to the accessor.
- `extractPromptBaseName(containerName, e.projectName)` at line 131: switch to `e.policy.ProjectName()`.

Run `grep -n "e\.containerImage\|e\.projectName\|e\.netrcFile\|e\.gitconfigFile\|e\.env\|e\.extraMounts\|e\.claudeDir\|e\.hideGit" pkg/executor/executor.go` to enumerate all sites — each must be updated.

1.4. Rewrite `buildDockerCommand` (currently lines 471-518) to derive opts from the policy:

```go
// buildDockerCommand builds the docker run command for a prompt execution.
// The launch shape is sourced from the executor's launchpolicy.Policy; only
// the prompt-specific concerns (prompt-file mount, ANTHROPIC_MODEL,
// YOLO_PROMPT_FILE, YOLO_OUTPUT, the dark-factory.prompt label) flow through
// the Extras overlay.
func (e *dockerExecutor) buildDockerCommand(
	ctx context.Context,
	containerName string,
	promptFilePath string,
	projectRoot string,
	claudeConfigDir string,
	promptBaseName string,
	home string,
) *exec.Cmd {
	envOverlay := map[string]string{
		"YOLO_PROMPT_FILE": "/tmp/prompt.md",
		"ANTHROPIC_MODEL":  e.model,
		"YOLO_OUTPUT":      "json",
	}
	extras := launchpolicy.Extras{
		ContainerName: containerName,
		EnvOverlay:    envOverlay,
		ExtraLabels: map[string]string{
			"dark-factory.prompt": promptBaseName,
		},
	}
	opts := e.policy.BuildOpts(extras)
	args := BuildDockerRunArgs(opts)
	args = insertPromptFileMount(args, promptFilePath, e.policy.ContainerImage())
	// #nosec G204 -- args are derived from configured policy + sanitized container name, not user input
	return exec.CommandContext(ctx, "docker", args...)
}
```

CRITICAL — the function signature still takes `projectRoot`, `claudeConfigDir`, `home` because the existing test helper `BuildDockerCommandForTest` passes them in. These parameters are now IGNORED by the rewritten body (the policy carries the per-daemon-process projectRoot, claudeDir, home — they no longer vary per Execute call). For now, keep the parameters in the signature to avoid a cascade of test changes; tag them with a leading `_` to silence the unused-parameter lint:

```go
func (e *dockerExecutor) buildDockerCommand(
	ctx context.Context,
	containerName string,
	promptFilePath string,
	_ string, // projectRoot — sourced from policy; param retained for test-helper compatibility
	_ string, // claudeConfigDir — sourced from policy
	promptBaseName string,
	_ string, // home — sourced from policy
) *exec.Cmd {
```

(Alternative considered and rejected: changing the signature here would force changes in `Execute`'s call site at line 137 and in `BuildDockerCommandForTest` and in every test that calls it — too much churn. The signature stability is intentional.)

NOTE: `Execute` (line 99+) still calls `os.Getwd()` and `os.UserHomeDir()` to compute `projectRoot` and `home`. These remain in `Execute` for now (they participate in pre-flight `validateClaudeAuth` and `slog` lines). Pass them through to `buildDockerCommand` unchanged; the new body ignores them.

1.5. Add accessor methods on `Policy` (back in `/workspace/pkg/launchpolicy/policy.go`):

```go
// ContainerImage returns the image reference (consumed by callers needing it
// outside BuildOpts, e.g. the executor's insertPromptFileMount).
func (p Policy) ContainerImage() string { return p.containerImage }

// ProjectName returns the value of the dark-factory.project label.
func (p Policy) ProjectName() string { return p.projectName }
```

Add unit-test coverage for both accessors in `pkg/launchpolicy/policy_test.go` (one short `It` each — Expect the constructor input value is returned).

## 2. Remove the inline cap literal — verify

After step 1.4, run:

```bash
grep -n "NET_ADMIN" pkg/executor/executor.go
# expected: 0 lines
grep -rn "NET_ADMIN" pkg/ | grep -v _test.go
# expected: 1 line — pkg/launchpolicy/policy.go (the CanonicalCaps declaration)
```

If either grep returns more than expected, locate the residual reference and remove it.

## 3. Update the test helper `BuildDockerCommandForTest`

In `/workspace/pkg/executor/export_test.go`, rewrite `BuildDockerCommandForTest` (currently lines 61-97) to construct a `Policy` from its long parameter list before calling the new-shape `buildDockerCommand`:

```go
func BuildDockerCommandForTest(
	ctx context.Context,
	containerImage string,
	projectName string,
	model string,
	netrcFile string,
	gitconfigFile string,
	env map[string]string,
	extraMounts []config.ExtraMount,
	containerName string,
	promptFilePath string,
	projectRoot string,
	claudeConfigDir string,
	promptBaseName string,
	home string,
	hideGit bool,
) *exec.Cmd {
	policy := launchpolicy.NewPolicy(
		containerImage,
		projectName,
		projectRoot,
		claudeConfigDir,
		home,
		env,
		extraMounts,
		netrcFile,
		gitconfigFile,
		hideGit,
	)
	e := &dockerExecutor{
		policy: policy,
		model:  model,
	}
	return e.buildDockerCommand(
		ctx,
		containerName,
		promptFilePath,
		projectRoot,
		claudeConfigDir,
		promptBaseName,
		home,
	)
}
```

Same treatment for `NewDockerExecutorWithRunnerForTest` (currently lines 27-58) — construct a `Policy` internally and pass it into a `&dockerExecutor{policy: ..., model: ..., commandRunner: runner, ...}` literal:

```go
func NewDockerExecutorWithRunnerForTest(
	containerImage string,
	projectName string,
	model string,
	netrcFile string,
	gitconfigFile string,
	env map[string]string,
	extraMounts []config.ExtraMount,
	claudeDir string,
	maxPromptDuration time.Duration,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	runner CommandRunnerForTest,
	fmtr formatter.Formatter,
	hideGit bool,
) Executor {
	// projectRoot defaults to "" in the test helper; tests that depend on it
	// already pass their own ProjectRoot via the buildCmd helper, which calls
	// BuildDockerCommandForTest with explicit projectRoot. Here we only
	// construct the executor instance — the policy's projectRoot field is not
	// dereferenced until Execute is called, which the runner-injection tests
	// drive directly.
	policy := launchpolicy.NewPolicy(
		containerImage,
		projectName,
		"", // projectRoot — set per-call in tests via the executor's Execute path
		claudeDir,
		"", // home
		env,
		extraMounts,
		netrcFile,
		gitconfigFile,
		hideGit,
	)
	return &dockerExecutor{
		policy:                policy,
		model:                 model,
		commandRunner:         runner,
		maxPromptDuration:     maxPromptDuration,
		currentDateTimeGetter: currentDateTimeGetter,
		formatter:             fmtr,
	}
}
```

Add the import in `export_test.go`:
```go
"github.com/bborbe/dark-factory/pkg/launchpolicy"
```

The existing `executor_test.go` test cases at lines 405-505 continue to compile and pass UNCHANGED — they consume `BuildDockerCommandForTest`'s public signature, which is identical.

## 4. Update factory call sites

In `/workspace/pkg/factory/factory.go`, both `executor.NewDockerExecutor` call sites must construct a `Policy` and pass the new shorter argument list.

4.1. Find both call sites:
```bash
grep -n "executor.NewDockerExecutor" pkg/factory/factory.go
# expected: 2 lines (currently around 682 and 920)
```

4.2. **PROCESS — read each call site VERBATIM before editing.** Both call sites pass values to the old `executor.NewDockerExecutor(...)` positionally. The new `launchpolicy.NewPolicy(...)` takes the same values, just bundled into one struct. The rule is: **for each positional argument the OLD call passed, pass the SAME expression to `NewPolicy`** — do NOT swap any field for `cfg.<Something>`; the old code's expression is the source of truth, even if `cfg.<Something>` looks equivalent.

Concrete divergences known today (verify by reading both sites before applying):

- **`CreateSpecGenerator` (around line 682)**: this factory function takes `containerImage string` as a parameter (NOT `cfg.ContainerImage`). The old `executor.NewDockerExecutor` call at line ~682 passes this parameter positionally. The new `NewPolicy` must therefore pass the same `containerImage` parameter — NOT `cfg.ContainerImage`. Confirm by reading the function signature and the old positional call.
- **The other call site (around line 920, the prompt-execution path)**: uses `cfg.ContainerImage` (or whatever the local resolution is). Preserve verbatim.
- **`hideGit` source**: `CreateSpecGenerator` uses `resolveSpecGeneratorHideGit(cfg)`; the line-920 site uses `cfg.HideGit` (or whatever the old call passes). Preserve each verbatim.
- **`projectRoot`, `home`**: if a call site already resolves these via `os.Getwd()` / `os.UserHomeDir()`, reuse those locals. If a call site does NOT resolve them today (because the old call passed nothing for them), add the `_, _ := os.Getwd()` / `os.UserHomeDir()` resolution next to the existing locals using the same best-effort pattern (`_` for error).

Generic shape after the call-site-verbatim reading:

```go
import "github.com/bborbe/dark-factory/pkg/launchpolicy"

// projectRoot/home: reuse the call site's existing locals; add resolution only if absent
projectRoot, _ := os.Getwd()
home, _ := os.UserHomeDir()

policy := launchpolicy.NewPolicy(
	<containerImage source from THIS call site>,           // see divergences above
	project.Resolve(cfg.ResolvedProjectOverride()).String(),
	projectRoot,
	cfg.ResolvedClaudeDir(),
	home,
	cfg.Env, // base env; ANTHROPIC_MODEL is overlaid at executor request time
	cfg.ExtraMounts,
	cfg.NetrcFile,
	cfg.GitconfigFile,
	<hideGit source from THIS call site>,                  // see divergences above
)
exec := executor.NewDockerExecutor(
	policy,
	cfg.Model,
	cfg.ParsedMaxPromptDuration(),
	currentDateTimeGetter,
	formatter.NewFormatter(currentDateTimeGetter),
)
```

If the project-name / env / extraMounts / netrc / gitconfig sources differ between sites (they shouldn't today, but read each site verbatim to confirm), apply the same rule: pass each call site the SAME expression the OLD positional argument used.

**Before applying — confirm `NewPolicy`'s signature.** Read `pkg/launchpolicy/policy.go` (delivered by prompt 1) and verify the parameter ORDER and ARITY match the call above. If prompt 1 settled on a different ordering or split, adjust the positional list accordingly; the contract is "same values, bundled" — not the literal order shown here.

4.3. If a call site does not currently resolve `projectRoot` and `home`, add the `os.Getwd()` / `os.UserHomeDir()` calls. Both are best-effort (existing code uses `_` for the error — keep the same pattern to avoid changing semantics).

If `os` is not already imported at the relevant site, add it.

## 5. Tests

5.1. The existing `pkg/executor/executor_test.go` test suite MUST continue to pass without changes to the cap-assertion lines (currently 451-452) or to any other assertion outside the test's own helper plumbing:

```bash
cd /workspace && go test ./pkg/executor/...
# expected: PASS
```

Run `git diff pkg/executor/executor_test.go` after the prompt — the diff SHOULD be empty (the test helper is wrapped by `BuildDockerCommandForTest` in `export_test.go`, which absorbs the constructor change).

If the diff is non-empty due to test-helper signature drift, that is acceptable ONLY if the cap-assertion lines themselves remain unchanged and the test still proves both `--cap-add=NET_ADMIN` and `--cap-add=NET_RAW` are present in the produced argv.

5.2. Add a new `It` to `pkg/executor/executor_test.go`'s `buildDockerCommand` `Describe` block (insertion point: right after the existing "always includes network capabilities" test at line 440) asserting the policy-sourced behavior end-to-end:

```go
It("sources capabilities from the launchpolicy, not from an inline literal", func() {
	cmd := defaultBuild(
		ctx,
		"test",
		"/tmp/test",
		"/workspace",
		"/home/user/.claude",
		"test",
		"/home/user",
	)
	// Both canonical caps appear because the test helper constructs a Policy
	// initialised with launchpolicy.CanonicalCaps.
	Expect(cmd.Args).To(ContainElement("--cap-add=NET_ADMIN"))
	Expect(cmd.Args).To(ContainElement("--cap-add=NET_RAW"))
})
```

This is documentation of intent; the assertions are the same as 451-452 but the test name makes the seam explicit. (Optional; if it duplicates existing coverage too closely, skip — the spec's regression-lock prompt covers the "policy → both sites" assertion structurally.)

## 6. Coverage and lint

The `pkg/executor` coverage MUST stay at or above its current level. Run:

```bash
cd /workspace && go test -coverprofile=/tmp/cover.out ./pkg/executor/... && go tool cover -func=/tmp/cover.out | tail -1
```

Expected: >= 80%.

`make precommit` MUST exit 0. The compile is the load-bearing check this prompt — any unresolved field-rename leaves the package broken.

</requirements>

<constraints>

- `BuildDockerRunArgs` signature and behavior are unchanged (spec 098 Constraint).
- `insertPromptFileMount`, `validateClaudeAuth`, and post-run commit logic are unchanged (spec 098 Constraint).
- Existing executor tests' cap assertions at `pkg/executor/executor_test.go:451-452` continue to assert both `--cap-add=NET_ADMIN` and `--cap-add=NET_RAW` (spec 098 AC "Existing executor tests pass unchanged").
- The cap set is exactly `{NET_ADMIN, NET_RAW}` — sourced from `launchpolicy.CanonicalCaps` (spec 098 Non-goal). Do NOT add a cap variation field.
- Capabilities are NOT exposed via `.dark-factory.yaml` — the policy's cap field is initialised exclusively from `CanonicalCaps` in `NewPolicy`. Do NOT add a constructor parameter for caps.
- After this prompt: `grep -rn "NET_ADMIN" pkg/ | grep -v _test.go | wc -l` returns 1 (the `pkg/launchpolicy/policy.go` `CanonicalCaps` declaration). The probes still compose `ContainerLaunchOpts` directly — that is fixed in prompt 3, so the `wc -l == 1` for `ContainerLaunchOpts{` lands in prompt 3, not here.
- Both `executor.NewDockerExecutor` call sites in `pkg/factory/factory.go` (currently lines 682 and 920) MUST be updated in this prompt — leaving one un-migrated breaks the build (signature change is not backwards-compatible).
- The two call sites have DIFFERENT `hideGit` sources (one uses `resolveSpecGeneratorHideGit(cfg)`, the other uses `cfg.HideGit`) — read each verbatim before changing; do NOT swap them.
- Errors wrapped with `bborbe/errors` (`errors.Wrap(ctx, err, "...")`) — never `fmt.Errorf`, never `context.Background()` in pkg/.
- BSD-style license header on every modified or new file.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

```bash
cd /workspace
# 1. Compilation + tests
go test ./pkg/executor/... ./pkg/launchpolicy/... ./pkg/factory/...
# expected: PASS

# 2. The inline cap literal is gone from the executor
grep -n "NET_ADMIN" pkg/executor/executor.go
# expected: 0 lines

# 3. Production code references the cap literal in exactly ONE file
grep -rn "NET_ADMIN" pkg/ | grep -v _test.go
# expected: 1 line, in pkg/launchpolicy/policy.go (the CanonicalCaps declaration)

# 4. ContainerLaunchOpts composition sites — still 2 production sites (executor migrated to BuildOpts which is now site 1; probes still site 2 — fixed in prompt 3)
grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go
# expected: 2 lines:
#   pkg/launchpolicy/policy.go:<line> (BuildOpts)
#   pkg/cmd/healthcheck/probes.go:355 (probes — fixed in prompt 3)

# 5. Cap-assertion lines in executor tests unchanged
git diff pkg/executor/executor_test.go -- '*.go'
# expected: only the new "sources capabilities from the launchpolicy" It block (if added);
#          no edit to existing lines 451-452.

# 6. Coverage retained
go test -coverprofile=/tmp/cover.out ./pkg/executor/... && go tool cover -func=/tmp/cover.out | tail -1
# expected: >= 80%

# 7. Full precommit
cd /workspace && make precommit
# expected: exit 0
```

On lint/errcheck failure inside the migrated files, fix the named target and re-run only that target. If `goimports` removes an import (e.g. `config.ExtraMount` no longer used in `executor.go`), accept the cleanup.

</verification>
