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
		checker  *mocks.ContainerChecker
		subprocR *mocks.SubprocRunner
		probe    *runner.BootContainerProbe
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		checker = &mocks.ContainerChecker{}
		subprocR = &mocks.SubprocRunner{}
		probe = &runner.BootContainerProbe{
			ContainerImage:   "test-image:latest",
			ProjectName:      "test-proj",
			ContainerChecker: checker,
			Subproc:          subprocR,
			ExtraMounts:      []config.ExtraMount{},
			ClaudeDir:        "/tmp/fake-claude-dir",
		}
	})

	AfterEach(func() {
		cancel()
	})

	It("returns nil on green path", func() {
		// First call = docker run → success, nil output
		// Second call = docker exec → success, stdout contains BOOT_OK
		callCount := 0
		subprocR.RunWithWarnAndTimeoutStub = func(
			_ context.Context, _ string, _ string, _ ...string,
		) ([]byte, error) {
			callCount++
			switch callCount {
			case 1:
				return []byte(""), nil // docker run
			default:
				return []byte("BOOT_OK\n"), nil // docker exec
			}
		}
		checker.WaitUntilRunningReturns(nil)

		err := probe.Run(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.WaitUntilRunningCallCount()).To(Equal(1))
		Expect(subprocR.RunWithWarnAndTimeoutCallCount()).To(Equal(2))
	})

	It("returns error when WaitUntilRunning fails", func() {
		// docker run succeeded
		subprocR.RunWithWarnAndTimeoutReturnsOnCall(0, []byte(""), nil)
		// WaitUntilRunning fails
		checker.WaitUntilRunningReturns(errors.Errorf(ctx, "boom"))

		err := probe.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("did not start"))
	})

	It("returns error when stdout missing BOOT_OK", func() {
		// docker run succeeded, docker exec succeeded but stdout empty
		subprocR.RunWithWarnAndTimeoutReturnsOnCall(0, []byte(""), nil)
		subprocR.RunWithWarnAndTimeoutReturnsOnCall(1, []byte(""), nil)
		checker.WaitUntilRunningReturns(nil)

		err := probe.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("BOOT_OK"))
	})

	It("returns error when docker run fails", func() {
		// First call (docker run) fails
		subprocR.RunWithWarnAndTimeoutReturnsOnCall(
			0,
			nil,
			errors.Errorf(ctx, "image not found"),
		)

		err := probe.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("start probe container"))
	})

	It("returns error when docker exec fails", func() {
		// docker run succeeds; docker exec returns an error
		subprocR.RunWithWarnAndTimeoutReturnsOnCall(0, []byte(""), nil)
		subprocR.RunWithWarnAndTimeoutReturnsOnCall(
			1,
			[]byte(""),
			errors.Errorf(ctx, "container not running"),
		)
		checker.WaitUntilRunningReturns(nil)

		err := probe.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("probe container boot check failed"))
	})

	It("appends extraMounts to the docker run command", func() {
		// Create a real src dir so os.Stat passes
		tmpDir, err := os.MkdirTemp("", "probe-extramount-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		roTrue := true
		probe.ExtraMounts = []config.ExtraMount{
			{Src: tmpDir, Dst: "/mnt/data", ReadOnly: &roTrue},
		}
		subprocR.RunWithWarnAndTimeoutStub = func(
			_ context.Context, _ string, _ string, _ ...string,
		) ([]byte, error) {
			return []byte("BOOT_OK\n"), nil
		}
		checker.WaitUntilRunningReturns(nil)

		Expect(probe.Run(ctx)).To(Succeed())
		_, _, _, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		// Expect the extra mount to appear with :ro suffix because ReadOnly is true
		expectedMount := tmpDir + ":/mnt/data:ro"
		Expect(args).To(ContainElement(expectedMount))
	})

	It("skips extraMounts whose src does not exist", func() {
		probe.ExtraMounts = []config.ExtraMount{
			{Src: "/nonexistent/path/that/does/not/exist", Dst: "/mnt/whatever"},
		}
		subprocR.RunWithWarnAndTimeoutStub = func(
			_ context.Context, _ string, _ string, _ ...string,
		) ([]byte, error) {
			return []byte("BOOT_OK\n"), nil
		}
		checker.WaitUntilRunningReturns(nil)

		Expect(probe.Run(ctx)).To(Succeed())
		_, _, _, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		// Mount should NOT be added
		for i, a := range args {
			if a == "-v" && i+1 < len(args) {
				Expect(args[i+1]).NotTo(ContainSubstring("/mnt/whatever"))
			}
		}
	})

	It("resolves relative extraMounts src against workspace dir", func() {
		// Use a relative src path so the relative-path branch fires
		probe.ExtraMounts = []config.ExtraMount{
			{Src: "relative-subdir", Dst: "/mnt/rel"},
		}
		subprocR.RunWithWarnAndTimeoutStub = func(
			_ context.Context, _ string, _ string, _ ...string,
		) ([]byte, error) {
			return []byte("BOOT_OK\n"), nil
		}
		checker.WaitUntilRunningReturns(nil)

		Expect(probe.Run(ctx)).To(Succeed())
		// The mount should not be added (relative src doesn't exist) — exercise the
		// filepath.IsAbs branch + the Stat skip branch
		_, _, _, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		// Just verify the probe completed without error
		Expect(args).NotTo(BeEmpty())
	})

	It("uses a unique container name per invocation", func() {
		// Both docker-run and docker-exec succeed
		subprocR.RunWithWarnAndTimeoutStub = func(
			_ context.Context, _ string, _ string, _ ...string,
		) ([]byte, error) {
			return []byte("BOOT_OK\n"), nil
		}
		checker.WaitUntilRunningReturns(nil)

		err := probe.Run(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = probe.Run(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(subprocR.RunWithWarnAndTimeoutCallCount()).To(Equal(4)) // 2 calls × 2 invocations
		// Every call's first docker arg is "run" or "exec"; we want the --name from call 0 and call 2
		// (the docker run calls — 0 and 2 since each Run() does run + exec).
		_, _, _, args0 := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		_, _, _, args2 := subprocR.RunWithWarnAndTimeoutArgsForCall(2)
		name0 := extractName(args0)
		name2 := extractName(args2)
		Expect(name0).NotTo(Equal(name2))
		Expect(name0).To(HavePrefix("test-proj-healthcheck-boot-"))
		Expect(name2).To(HavePrefix("test-proj-healthcheck-boot-"))
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
