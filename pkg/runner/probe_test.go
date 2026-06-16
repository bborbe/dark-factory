// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
	"context"
	"os"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/runner"
)

var _ = Describe("BootContainerProbe", func() {
	var (
		ctx      context.Context
		cancel   context.CancelFunc
		subprocR *mocks.SubprocRunner
		probe    *runner.BootContainerProbe
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		subprocR = &mocks.SubprocRunner{}
		probe = &runner.BootContainerProbe{
			ContainerImage: "test-image:latest",
			ProjectName:    "test-proj",
			Subproc:        subprocR,
			ExtraMounts:    []config.ExtraMount{},
			ClaudeDir:      "/tmp/fake-claude-dir",
		}
	})

	AfterEach(func() {
		cancel()
	})

	It("returns nil on green path", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte("BOOT_OK\n"), nil)

		err := probe.Run(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(subprocR.RunWithWarnAndTimeoutCallCount()).To(Equal(1))
	})

	It("returns error when stdout missing BOOT_OK", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte(""), nil)

		err := probe.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("BOOT_OK"))
	})

	It("returns error when docker run fails", func() {
		subprocR.RunWithWarnAndTimeoutReturns(
			nil,
			errors.Errorf(ctx, "image not found"),
		)

		err := probe.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("probe container boot check failed"))
	})

	It("appends extraMounts to the docker run command", func() {
		tmpDir, err := os.MkdirTemp("", "probe-extramount-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		roTrue := true
		probe.ExtraMounts = []config.ExtraMount{
			{Src: tmpDir, Dst: "/mnt/data", ReadOnly: &roTrue},
		}
		subprocR.RunWithWarnAndTimeoutReturns([]byte("BOOT_OK\n"), nil)

		Expect(probe.Run(ctx)).To(Succeed())
		_, _, _, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		expectedMount := tmpDir + ":/mnt/data:ro"
		Expect(args).To(ContainElement(expectedMount))
	})

	It("skips extraMounts whose src does not exist", func() {
		probe.ExtraMounts = []config.ExtraMount{
			{Src: "/nonexistent/path/that/does/not/exist", Dst: "/mnt/whatever"},
		}
		subprocR.RunWithWarnAndTimeoutReturns([]byte("BOOT_OK\n"), nil)

		Expect(probe.Run(ctx)).To(Succeed())
		_, _, _, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		for i, a := range args {
			if a == "-v" && i+1 < len(args) {
				Expect(args[i+1]).NotTo(ContainSubstring("/mnt/whatever"))
			}
		}
	})

	It("resolves relative extraMounts src against workspace dir", func() {
		probe.ExtraMounts = []config.ExtraMount{
			{Src: "relative-subdir", Dst: "/mnt/rel"},
		}
		subprocR.RunWithWarnAndTimeoutReturns([]byte("BOOT_OK\n"), nil)

		Expect(probe.Run(ctx)).To(Succeed())
		_, _, _, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		Expect(args).NotTo(BeEmpty())
	})

	It("invokes docker run with --entrypoint /bin/sh and -c probeCommand", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte("BOOT_OK\n"), nil)

		Expect(probe.Run(ctx)).To(Succeed())
		_, _, _, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		Expect(args).To(ContainElement("--entrypoint"))
		Expect(args).To(ContainElement("/bin/sh"))
		Expect(args).To(ContainElement("-c"))
	})

	It("uses a unique container name per invocation", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte("BOOT_OK\n"), nil)

		err := probe.Run(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = probe.Run(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(subprocR.RunWithWarnAndTimeoutCallCount()).To(Equal(2))
		_, _, _, args0 := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		_, _, _, args1 := subprocR.RunWithWarnAndTimeoutArgsForCall(1)
		name0 := extractName(args0)
		name1 := extractName(args1)
		Expect(name0).NotTo(Equal(name1))
		Expect(name0).To(HavePrefix("test-proj-healthcheck-boot-"))
		Expect(name1).To(HavePrefix("test-proj-healthcheck-boot-"))
	})
})

// extractName walks `docker run --rm --name <name> ...` arg list and returns the name.
func extractName(args []string) string {
	for i, a := range args {
		if a == "--name" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

var _ = Describe("truncate", func() {
	It("returns the string when shorter than n", func() {
		got := runner.TruncateForTest("hello", 10)
		Expect(got).To(Equal("hello"))
	})

	It("truncates and appends ellipsis when longer than n", func() {
		got := runner.TruncateForTest("hello world", 8)
		Expect(got).To(Equal("hello..."))
	})

	It("handles n smaller than 3 without panic", func() {
		got := runner.TruncateForTest("hello", 2)
		Expect(got).To(Equal("he"))
	})
})

var _ = Describe("uniqueContainerName", func() {
	It("returns name with project prefix and 16 hex chars", func() {
		p := &runner.BootContainerProbe{ProjectName: "my-proj"}
		name, err := p.UniqueContainerNameForTest()
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(HavePrefix("my-proj-healthcheck-boot-"))
		Expect(name).To(HaveLen(len("my-proj-healthcheck-boot-") + 16))
	})
})
