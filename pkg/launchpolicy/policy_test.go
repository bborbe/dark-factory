// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package launchpolicy_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/launchpolicy"
)

func testPolicy() launchpolicy.Policy {
	return launchpolicy.NewPolicy(
		"docker.io/bborbe/claude-yolo:v0.10.1",
		"test-project",
		"/host/project",
		"/host/.claude",
		"/host/home",
		map[string]string{"ANTHROPIC_MODEL": "sonnet"},
		nil,   // ExtraMounts
		"",    // NetrcFile
		"",    // GitconfigFile
		false, // HideGit
	)
}

var _ = Describe("Policy", func() {
	It("BuildOpts produces argv containing the canonical caps", func() {
		opts := testPolicy().BuildOpts(launchpolicy.Extras{ContainerName: "test-name"})
		args := executor.BuildDockerRunArgs(opts)
		Expect(args).To(ContainElement("--cap-add=NET_ADMIN"))
		Expect(args).To(ContainElement("--cap-add=NET_RAW"))
	})

	It("BuildOpts produces argv with the standard /workspace and claude-dir mounts", func() {
		opts := testPolicy().BuildOpts(launchpolicy.Extras{ContainerName: "test-name"})
		args := executor.BuildDockerRunArgs(opts)
		Expect(args).To(ContainElement("/host/project:/workspace"))
		Expect(args).To(ContainElement("/host/.claude:/home/node/.claude"))
	})

	It("BuildOpts wires the project label from the policy", func() {
		opts := testPolicy().BuildOpts(launchpolicy.Extras{ContainerName: "test-name"})
		args := executor.BuildDockerRunArgs(opts)
		Expect(args).To(ContainElement("dark-factory.project=test-project"))
	})

	It("BuildOpts wires per-invocation entrypoint and command", func() {
		extras := launchpolicy.Extras{
			ContainerName: "x",
			Entrypoint:    "/bin/sh",
			Command:       []string{"-c", "echo BOOT_OK"},
		}
		opts := testPolicy().BuildOpts(extras)
		args := executor.BuildDockerRunArgs(opts)
		Expect(args).To(ContainElement("--entrypoint"))
		Expect(args).To(ContainElement("/bin/sh"))
		Expect(args).To(ContainElement("-c"))
		Expect(args).To(ContainElement("echo BOOT_OK"))
	})

	It("BuildOpts merges baseEnv with EnvOverlay, overlay wins on collision", func() {
		extras := launchpolicy.Extras{
			ContainerName: "x",
			EnvOverlay: map[string]string{
				"ANTHROPIC_MODEL":  "haiku",
				"YOLO_PROMPT_FILE": "/tmp/prompt.md",
			},
		}
		opts := testPolicy().BuildOpts(extras)
		args := executor.BuildDockerRunArgs(opts)
		Expect(args).To(ContainElement("ANTHROPIC_MODEL=haiku"))
		Expect(args).To(ContainElement("YOLO_PROMPT_FILE=/tmp/prompt.md"))
		Expect(args).NotTo(ContainElement("ANTHROPIC_MODEL=sonnet"))
	})

	It("BuildOpts wires per-invocation ExtraLabels alongside the project label", func() {
		extras := launchpolicy.Extras{
			ContainerName: "x",
			ExtraLabels:   map[string]string{"dark-factory.prompt": "my-prompt"},
		}
		opts := testPolicy().BuildOpts(extras)
		args := executor.BuildDockerRunArgs(opts)
		Expect(args).To(ContainElement("dark-factory.prompt=my-prompt"))
		Expect(args).To(ContainElement("dark-factory.project=test-project"))
	})

	It("NewPolicy defensively copies baseEnv so caller mutation does not leak", func() {
		src := map[string]string{"K": "v1"}
		p := launchpolicy.NewPolicy(
			"docker.io/bborbe/claude-yolo:v0.10.1",
			"test-project",
			"/host/project",
			"/host/.claude",
			"/host/home",
			src,
			nil,
			"",
			"",
			false,
		)
		src["K"] = "MUTATED"
		opts := p.BuildOpts(launchpolicy.Extras{ContainerName: "x"})
		args := executor.BuildDockerRunArgs(opts)
		Expect(args).To(ContainElement("K=v1"))
		Expect(args).NotTo(ContainElement("K=MUTATED"))
	})

	It("WithCapAddForTest replaces the cap set without mutating the original", func() {
		orig := testPolicy()
		modified := orig.WithCapAddForTest([]string{"SYS_PTRACE"})
		origArgs := executor.BuildDockerRunArgs(
			orig.BuildOpts(launchpolicy.Extras{ContainerName: "x"}),
		)
		modArgs := executor.BuildDockerRunArgs(
			modified.BuildOpts(launchpolicy.Extras{ContainerName: "x"}),
		)
		Expect(origArgs).To(ContainElement("--cap-add=NET_ADMIN"))
		Expect(origArgs).NotTo(ContainElement("--cap-add=SYS_PTRACE"))
		Expect(modArgs).To(ContainElement("--cap-add=SYS_PTRACE"))
		Expect(modArgs).NotTo(ContainElement("--cap-add=NET_ADMIN"))
	})

	It("CanonicalCaps contains exactly NET_ADMIN and NET_RAW in that order", func() {
		Expect(launchpolicy.CanonicalCaps).To(Equal([]string{"NET_ADMIN", "NET_RAW"}))
	})

	It("ContainerImage returns the image passed to NewPolicy", func() {
		p := testPolicy()
		Expect(p.ContainerImage()).To(Equal("docker.io/bborbe/claude-yolo:v0.10.1"))
	})

	It("ProjectName returns the project name passed to NewPolicy", func() {
		p := testPolicy()
		Expect(p.ProjectName()).To(Equal("test-project"))
	})
})
