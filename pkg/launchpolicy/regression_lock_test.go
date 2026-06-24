// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// SYS_PTRACE is the synthetic-cap marker used to prove a Policy field reaches
// both the executor's prompt-run argv and each container probe's argv. Authorized
// test-only uses are in policy_test.go (WithCapAddForTest unit test) and this file
// (regression-lock). Any production-code mention is a regression.

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
// Authorized test-only uses of SYS_PTRACE: policy_test.go (WithCapAddForTest
// unit test) and this file. CI may grep for SYS_PTRACE in production code
// (grep -rn "SYS_PTRACE" pkg/ | grep -v _test.go) to catch regressions.
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

	It(
		"propagates a synthetic cap through executor.BuildDockerRunArgs (executor call path)",
		func() {
			// Both buildDockerCommand and runContainerProbe call executor.BuildDockerRunArgs
			// after deriving opts from policy.BuildOpts. Testing BuildDockerRunArgs here
			// locks the invariant that the Policy's capAdd field reaches the final argv at
			// the executor call site (pkg/executor/executor.go).
			policy := regressionPolicy()
			opts := policy.BuildOpts(launchpolicy.Extras{
				ContainerName: "test-container",
				EnvOverlay: map[string]string{
					"YOLO_PROMPT_FILE": "/tmp/prompt.md",
					"ANTHROPIC_MODEL":  "sonnet",
					"YOLO_OUTPUT":      "json",
				},
			})
			args := executor.BuildDockerRunArgs(opts)
			Expect(args).To(ContainElement("--cap-add=SYS_PTRACE"))
			// The canonical caps are GONE — the policy's capAdd was wholly replaced.
			// If a future change re-introduces the canonical caps via an inline literal in
			// BuildDockerRunArgs itself, this assertion fails.
			Expect(args).NotTo(ContainElement("--cap-add=NET_ADMIN"))
			Expect(args).NotTo(ContainElement("--cap-add=NET_RAW"))
		},
	)

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
		Entry("boot probe", "boot", healthcheck.NewBootProbe, "BOOT_OK"),
		Entry("mount probe", "mount", healthcheck.NewMountProbe, "MOUNT_OK"),
		Entry("claude probe", "claude", healthcheck.NewClaudeProbe, "OK"),
	)
})
