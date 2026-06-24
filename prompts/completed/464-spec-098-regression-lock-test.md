---
status: completed
spec: [098-bug-unify-container-launch-policy]
summary: Added regression_lock_test.go in pkg/launchpolicy and BuildDockerCommandFromPolicyForTest in pkg/executor/export_test.go; regression lock test injects SYS_PTRACE via Policy.WithCapAddForTest and asserts propagation through executor.BuildDockerRunArgs and all three container probes' argv
container: dark-factory-unify-container-launch-exec-464-spec-098-regression-lock-test
dark-factory-version: v0.182.0
created: "2026-06-24T19:30:00Z"
queued: "2026-06-24T19:40:37Z"
started: "2026-06-24T21:19:07Z"
completed: "2026-06-24T21:30:05Z"
---

<summary>

- Adds the regression-lock test the spec requires: structural proof that a launch-shape field added to `launchpolicy.Policy` reaches BOTH the executor's prompt-run argv AND each probe's argv with no further code changes.
- Lives in a single test file `pkg/launchpolicy/regression_lock_test.go` (external test package `launchpolicy_test`) so it can import both the executor's test-helper entry point and the probes' constructors.
- Uses `Policy.WithCapAddForTest([]string{"SYS_PTRACE"})` to inject a synthetic capability through the shared policy, then asserts `--cap-add=SYS_PTRACE` appears in (a) the executor's `buildDockerCommand` argv via `executor.BuildDockerCommandForTest` and (b) each of the boot/mount/claude probes' argv captured from the `subproc.Runner` mock.
- The test fails by construction if a future change adds a launch-shape field at the executor call site or at the probe call site WITHOUT routing it through the policy — the synthetic cap simply will not propagate.
- This prompt adds one regression-lock test file PLUS one test-only helper in `pkg/executor/export_test.go`. No `.go` production file is touched (`_test.go` files only).
- After this prompt: spec AC "Regression-lock test: a new field in the policy reaches both call sites" flips to PASS.

</summary>

<objective>
Add a single Ginkgo test file at `/workspace/pkg/launchpolicy/regression_lock_test.go` that injects a synthetic capability through `Policy.WithCapAddForTest` and asserts it propagates to BOTH the executor's prompt-run argv AND each of the three container probes' assembled argv. The test is the structural guarantee against future divergence between the two call sites.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega; `DescribeTable` for table-driven assertions
- `/home/node/.claude/plugins/marketplaces/coding/docs/test-pyramid-triggers.md` — when a test crosses a structural boundary (here: executor and probes consume the same Policy), a single integration-style assertion is the right level

Read these source files end-to-end before editing:

- `/workspace/pkg/launchpolicy/policy.go` (final shape after prompts 1+2+3) — `NewPolicy`, `Policy`, `Extras`, `BuildOpts`, `WithCapAddForTest`, `ContainerImage`, `ProjectName` accessors.
- `/workspace/pkg/executor/export_test.go` (post-prompt-2) — `BuildDockerCommandForTest` signature. NOTE the test helper takes the OLD long parameter list and internally builds a Policy — the regression-lock test cannot inject a custom Policy through it. SOLUTION: extend `export_test.go` in this prompt with a NEW test-helper that takes a `Policy` directly. See requirement 2.
- `/workspace/pkg/cmd/healthcheck/probes.go` (post-prompt-3) — `NewBootProbe`, `NewClaudeProbe`, `NewMountProbe` take `launchpolicy.Policy` directly. This test calls them and captures the `args` slice from the `subproc.Runner` mock.
- `/workspace/mocks/subproc-runner.go` (counterfeiter-generated) — the `SubprocRunner` mock; method `RunWithWarnAndTimeoutArgsForCall(i int)` returns `(ctx, op, name, args)`.
- `/workspace/pkg/cmd/healthcheck/probes_test.go` — for the pattern of capturing the mock's argv slice via `subprocR.RunWithWarnAndTimeoutArgsForCall(0)`.

Read the parent spec:
- `/workspace/specs/in-progress/098-bug-unify-container-launch-policy.md` — Desired Behavior item 5, Acceptance Criteria row "Regression-lock test", the failure-mode row "Code change adds a launch-shape concern ... bypassing the policy".

The synthetic capability used as the regression marker is `SYS_PTRACE`. It is chosen specifically because:
1. It is NOT in `CanonicalCaps` (so its absence is a clean signal that the policy was not consulted).
2. It is a real Linux capability name (so the argv element matches the same `--cap-add=<name>` shape as the canonical caps — no false negatives from format mismatches).
3. The test is the ONLY production-or-test-code mention of `SYS_PTRACE` in the repository.

</context>

<requirements>

## 1. New test-helper for direct Policy injection

In `/workspace/pkg/executor/export_test.go`, ADD a new exported test helper that accepts a `Policy` directly (alongside the existing `BuildDockerCommandForTest`):

```go
// BuildDockerCommandFromPolicyForTest is the policy-injection test helper used
// by the spec-098 regression-lock test. It accepts a Policy directly so a
// test can construct a Policy with a synthetic capability set via
// Policy.WithCapAddForTest and prove the cap propagates to the executor's
// argv. Production callers MUST use NewDockerExecutor; this helper is for
// tests only.
func BuildDockerCommandFromPolicyForTest(
	ctx context.Context,
	policy launchpolicy.Policy,
	model string,
	containerName string,
	promptFilePath string,
	projectRoot string,
	claudeConfigDir string,
	promptBaseName string,
	home string,
) *exec.Cmd {
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

The existing `BuildDockerCommandForTest` stays untouched (the existing `pkg/executor/executor_test.go` cases continue to use it). The new helper is additive.

## 2. The regression-lock test file

Create `/workspace/pkg/launchpolicy/regression_lock_test.go` with BSD header and `package launchpolicy_test`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package launchpolicy_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd/healthcheck"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/launchpolicy"
	"github.com/bborbe/dark-factory/pkg/subproc"
)

// regressionPolicy returns a Policy with capAdd replaced by a single synthetic
// capability (SYS_PTRACE). The test asserts this synthetic value reaches BOTH
// the executor's argv AND each probe's argv — proving the Policy is the
// single source of truth for launch-shape inputs.
//
// SYS_PTRACE is the only mention of the literal in the repository. If a
// future change introduces a second mention, it is either:
//   (a) a legitimate extension of the regression test (this file), OR
//   (b) a regression: a production code path inlined a cap literal again.
// CI may grep for SYS_PTRACE outside this file to catch (b).
func regressionPolicy() launchpolicy.Policy {
	base := launchpolicy.NewPolicy(
		"alpine:latest",      // containerImage
		"regression-project", // projectName
		"/tmp/regression",    // projectRoot
		"/tmp/.claude",       // claudeDir
		"/tmp/home",          // home
		nil,                  // baseEnv
		nil,                  // extraMounts
		"",                   // netrcFile
		"",                   // gitconfigFile
		false,                // hideGit
	)
	return base.WithCapAddForTest([]string{"SYS_PTRACE"})
}

var _ = Describe("spec-098 regression lock: launch policy is the single source of truth", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("propagates a synthetic cap through the executor's prompt-run argv", func() {
		cmd := executor.BuildDockerCommandFromPolicyForTest(
			ctx,
			regressionPolicy(),
			"sonnet",             // model
			"test-container",     // containerName
			"/tmp/prompt.md",     // promptFilePath
			"/tmp/regression",    // projectRoot (parameter retained; sourced from policy)
			"/tmp/.claude",       // claudeConfigDir (parameter retained; sourced from policy)
			"test-prompt",        // promptBaseName
			"/tmp/home",          // home (parameter retained; sourced from policy)
		)
		Expect(cmd.Args).To(ContainElement("--cap-add=SYS_PTRACE"))
		// AND the canonical caps are GONE — the policy's capAdd was wholly replaced.
		// This is the "regression by inlining" detector: if a future change re-introduces
		// the canonical caps via an inline literal in buildDockerCommand, this assertion
		// fails because both the synthetic AND the canonical appear.
		Expect(cmd.Args).NotTo(ContainElement("--cap-add=NET_ADMIN"))
		Expect(cmd.Args).NotTo(ContainElement("--cap-add=NET_RAW"))
	})

	DescribeTable(
		"propagates a synthetic cap through each container probe's argv",
		func(probeName string, newProbe func(launchpolicy.Policy, subproc.Runner) healthcheck.Probe, successMarker string) {
			subprocR := &mocks.SubprocRunner{}
			subprocR.RunWithWarnAndTimeoutReturns([]byte(successMarker+"\n"), nil)
			p := newProbe(regressionPolicy(), subprocR)
			Expect(p.Run(ctx)).To(Succeed())
			_, _, dockerBin, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
			Expect(dockerBin).To(Equal("docker"))
			Expect(args).To(ContainElement("--cap-add=SYS_PTRACE"))
			// Same regression detector for the probe side:
			Expect(args).NotTo(ContainElement("--cap-add=NET_ADMIN"))
			Expect(args).NotTo(ContainElement("--cap-add=NET_RAW"))
		},
		Entry("boot probe",   "boot",   healthcheck.NewBootProbe,   "BOOT_OK"),
		Entry("mount probe",  "mount",  healthcheck.NewMountProbe,  "MOUNT_OK"),
		Entry("claude probe", "claude", healthcheck.NewClaudeProbe, "OK"),
	)
})
```

NOTE: The `launchpolicy_test` package already has its Ginkgo suite bootstrap from prompt 1 (`launchpolicy_suite_test.go`). The new `Describe` block hooks into the same suite — no extra bootstrap.

## 3. No production code changes

This prompt adds ONLY:
- One new test file: `/workspace/pkg/launchpolicy/regression_lock_test.go`.
- One new test-only function: `BuildDockerCommandFromPolicyForTest` in `/workspace/pkg/executor/export_test.go`.

No production code changes. If you find yourself editing `pkg/executor/executor.go`, `pkg/cmd/healthcheck/probes.go`, `pkg/factory/factory.go`, or `pkg/launchpolicy/policy.go` (beyond what prompts 1-3 already shipped) — STOP and re-read this prompt. The regression-lock test must be a pure observer.

## 4. Imports / dependency-cycle check

The new test file imports both `pkg/executor` (for `BuildDockerCommandFromPolicyForTest`) and `pkg/cmd/healthcheck` (for `NewBootProbe` / `NewMountProbe` / `NewClaudeProbe`). Both packages already import `pkg/launchpolicy`, so the test package importing both creates NO cycle (test-only imports are exempt from the production import graph; even so, the production graph is acyclic: `launchpolicy` -> `executor` and `launchpolicy` -> `config`; `healthcheck` -> `executor` + `launchpolicy`; both lead to `launchpolicy` as a leaf in the new graph).

Verify after writing:
```bash
cd /workspace && go test ./pkg/launchpolicy/... -count=1
# expected: PASS — if the build fails with "import cycle", revisit the import statement above.
```

## 5. Optional grep-discoverability of the regression marker

Add ONE comment line at the top of `regression_lock_test.go` that makes the regression-marker discoverable by grep:

```go
// SYS_PTRACE is the synthetic-cap marker used to prove a Policy field reaches
// both the executor's prompt-run argv and each container probe's argv. Mention
// of SYS_PTRACE outside this file (in production code or in any other test
// file) is either an extension of this regression test or a regression itself.
```

After this prompt:
```bash
grep -rn "SYS_PTRACE" pkg/ mocks/ cmd/
# expected: matches ONLY in pkg/launchpolicy/regression_lock_test.go (the comment + the literal in the test body).
```

</requirements>

<constraints>

- This is a TEST-ONLY prompt. Production code changes from prompts 1-3 are assumed in place; this prompt asserts the structural invariant they collectively establish.
- The regression marker is `SYS_PTRACE` and ONLY `SYS_PTRACE`. Do NOT introduce additional synthetic caps — adding more would dilute the grep-discoverability of regressions.
- `WithCapAddForTest` is the policy seam — do NOT call `NewPolicy` with a manual cap argument (the constructor doesn't accept one; it must not, per spec 098 Non-goal).
- The test asserts BOTH presence of `SYS_PTRACE` AND absence of `NET_ADMIN` / `NET_RAW`. The absence assertion is the "policy fully replaced caps" check — without it, an inline cap literal could co-exist with the policy and the regression would not surface.
- `BuildDockerCommandFromPolicyForTest` lives in `export_test.go` so it is NOT exported in the production module ABI. Do NOT add it to `executor.go` or any non-`_test.go` file.
- Errors wrapped with `bborbe/errors` IF this prompt adds an error path — but the regression test does not, so no `errors` import is needed.
- BSD-style license header on every new file.
- Coverage for `pkg/launchpolicy` stays >= 80%.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

```bash
cd /workspace

# 1. The regression-lock suite passes
go test -v ./pkg/launchpolicy/... -run 'regression lock' 2>&1 | tail -20
# expected: 4 passes (1 executor It + 3 DescribeTable entries)

# 2. Full launchpolicy + dependent suites still green
go test ./pkg/launchpolicy/... ./pkg/executor/... ./pkg/cmd/healthcheck/...
# expected: PASS

# 3. SYS_PTRACE is mentioned only in the regression-lock file
grep -rn "SYS_PTRACE" pkg/ mocks/ cmd/
# expected: matches only in pkg/launchpolicy/regression_lock_test.go

# 4. The architectural invariants from prompts 1-3 still hold (no regression here)
grep -rn "NET_ADMIN" pkg/ | grep -v _test.go
# expected: 1 line (pkg/launchpolicy/policy.go CanonicalCaps)
grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go
# expected: 1 line (pkg/launchpolicy/policy.go BuildOpts)

# 5. Full precommit
cd /workspace && make precommit
# expected: exit 0

# 6. Negative-case manual verification (informational; do NOT commit the regression):
#    To prove the test actually fails when the policy is bypassed, temporarily
#    edit pkg/executor/executor.go's buildDockerCommand to add an inline
#    `args = append(args, "--cap-add=NET_ADMIN")` at the end and re-run
#    `go test ./pkg/launchpolicy/...`. The executor-side It should FAIL because
#    the NotTo assertion for NET_ADMIN catches the inline reintroduction.
#    Revert the temporary edit before continuing.
```

</verification>
