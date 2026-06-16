// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healthcheck_test

import (
	"context"
	"testing"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd/healthcheck"
	"github.com/bborbe/dark-factory/pkg/runner"
)

func TestHealthcheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Healthcheck Suite")
}

var _ = Describe("DockerProbe", func() {
	var (
		ctx     context.Context
		subproc *mocks.SubprocRunner
	)

	BeforeEach(func() {
		ctx = context.Background()
		subproc = &mocks.SubprocRunner{}
	})

	It("returns nil when docker version succeeds", func() {
		subproc.RunWithWarnAndTimeoutReturns([]byte(""), nil)
		p := healthcheck.NewDockerProbe(subproc)
		Expect(p.Name()).To(Equal("docker"))
		Expect(p.Run(ctx)).To(Succeed())
		Expect(subproc.RunWithWarnAndTimeoutCallCount()).To(Equal(1))
	})

	It("wraps error with docker daemon unreachable on non-zero exit", func() {
		subproc.RunWithWarnAndTimeoutReturns(
			[]byte("Cannot connect to the Docker daemon at unix:///var/run/docker.sock"),
			errors.Errorf(ctx, "exit 1"),
		)
		p := healthcheck.NewDockerProbe(subproc)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("docker daemon unreachable"))
	})
})

var _ = Describe("ImageProbe", func() {
	var (
		ctx     context.Context
		subproc *mocks.SubprocRunner
	)

	BeforeEach(func() {
		ctx = context.Background()
		subproc = &mocks.SubprocRunner{}
	})

	It("returns nil when image inspect succeeds", func() {
		subproc.RunWithWarnAndTimeoutReturns([]byte("sha256:abc"), nil)
		p := healthcheck.NewImageProbe("alpine:latest", subproc)
		Expect(p.Name()).To(Equal("image"))
		Expect(p.Run(ctx)).To(Succeed())
		_, _, name, args := subproc.RunWithWarnAndTimeoutArgsForCall(0)
		Expect(name).To(Equal("docker"))
		Expect(args).To(ContainElement("alpine:latest"))
	})

	It("returns image-not-present error on non-zero exit", func() {
		subproc.RunWithWarnAndTimeoutReturns(nil, errors.Errorf(ctx, "exit 1"))
		p := healthcheck.NewImageProbe("missing:tag", subproc)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`"missing:tag" not present locally`))
	})
})

var _ = Describe("BootProbe", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns nil when wrapped BootContainerProbe.Run succeeds", func() {
		// Wrap a probe whose Run method we control via a fake wrapper.
		// The simplest approach: pass a BootContainerProbe with a mock subproc
		// and a mock checker. We do not duplicate the integration — we just
		// confirm the wrapper passes through success and failure.
		checker := &mocks.ContainerChecker{}
		subprocR := &mocks.SubprocRunner{}
		// First call (docker run) returns nil output; second (docker exec) returns BOOT_OK
		calls := 0
		subprocR.RunWithWarnAndTimeoutStub = func(
			_ context.Context, _ string, _ string, _ ...string,
		) ([]byte, error) {
			calls++
			if calls == 1 {
				return []byte(""), nil
			}
			return []byte("BOOT_OK\n"), nil
		}
		checker.WaitUntilRunningReturns(nil)

		probe := &runner.BootContainerProbe{
			ContainerImage:   "alpine:latest",
			ProjectName:      "test-proj",
			ContainerChecker: checker,
			Subproc:          subprocR,
			ExtraMounts:      nil,
			ClaudeDir:        "/tmp",
		}
		p := healthcheck.NewBootProbe(probe)
		Expect(p.Name()).To(Equal("boot"))
		Expect(p.Run(ctx)).To(Succeed())
	})

	It("wraps error from BootContainerProbe.Run", func() {
		// Use a non-functional BootContainerProbe (no checker, no runner) and
		// rely on its first internal call to fail. We just confirm the error
		// path is wrapped.
		subprocR := &mocks.SubprocRunner{}
		checker := &mocks.ContainerChecker{}
		probe := &runner.BootContainerProbe{
			ContainerImage:   "alpine:latest",
			ProjectName:      "test-proj",
			ContainerChecker: checker,
			Subproc:          subprocR,
		}
		// subproc returns success on docker run, then WaitUntilRunning fails
		subprocR.RunWithWarnAndTimeoutReturns([]byte(""), nil)
		checker.WaitUntilRunningReturns(errors.Errorf(ctx, "container did not start"))

		p := healthcheck.NewBootProbe(probe)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("container boot probe failed"))
	})
})

var _ = Describe("MountProbe", func() {
	var (
		ctx     context.Context
		subproc *mocks.SubprocRunner
	)

	BeforeEach(func() {
		ctx = context.Background()
		subproc = &mocks.SubprocRunner{}
	})

	It("returns nil when mount write succeeds", func() {
		subproc.RunWithWarnAndTimeoutReturns([]byte("MOUNT_OK\n"), nil)
		p := healthcheck.NewMountProbe("alpine:latest", subproc)
		Expect(p.Name()).To(Equal("mount"))
		Expect(p.Run(ctx)).To(Succeed())
		_, _, name, args := subproc.RunWithWarnAndTimeoutArgsForCall(0)
		Expect(name).To(Equal("docker"))
		Expect(args).To(ContainElement("alpine:latest"))
	})

	It("returns mount-not-writable error when exit non-zero", func() {
		subproc.RunWithWarnAndTimeoutReturns(nil, errors.Errorf(ctx, "exit 1"))
		p := healthcheck.NewMountProbe("alpine:latest", subproc)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("workspace mount not writable"))
	})

	It("returns mount-not-writable error when stdout missing MOUNT_OK", func() {
		subproc.RunWithWarnAndTimeoutReturns([]byte("partial"), nil)
		p := healthcheck.NewMountProbe("alpine:latest", subproc)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("workspace mount not writable"))
	})
})
