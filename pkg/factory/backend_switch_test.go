// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/formatter"
	"github.com/bborbe/dark-factory/pkg/launchpolicy"
)

// Internal (package factory) test: exercises the unexported backend-select
// helpers directly to prove backend: local routes to the in-process variants
// and the default (docker) routes to the docker variants — without a daemon.
var _ = Describe("backend switch helpers", func() {
	var (
		ctx  context.Context
		cdtg libtime.CurrentDateTimeGetter
	)

	BeforeEach(func() {
		ctx = context.Background()
		cdtg = libtime.NewCurrentDateTime()
	})

	Describe("createExecutionChecker", func() {
		It("routes backend: local to a checker whose IsRunning is (false, nil)", func() {
			running, err := createExecutionChecker(config.BackendLocal, cdtg).IsRunning(ctx, "any")
			Expect(err).NotTo(HaveOccurred())
			Expect(running).To(BeFalse())
		})
		It("routes docker (default) to a non-nil checker", func() {
			Expect(createExecutionChecker(config.BackendDocker, cdtg)).NotTo(BeNil())
		})
	})

	Describe("createExecutionStopper", func() {
		It("routes backend: local to a no-op stopper (StopContainer returns nil)", func() {
			Expect(
				createExecutionStopper(config.BackendLocal).StopContainer(ctx, "any"),
			).To(Succeed())
		})
		It("routes docker (default) to a non-nil stopper", func() {
			Expect(createExecutionStopper(config.BackendDocker)).NotTo(BeNil())
		})
	})

	Describe("createContainerCounter", func() {
		It(
			"routes backend: local to the noop counter — CountRunning is (0, nil), no docker ps",
			func() {
				n, err := createContainerCounter(config.BackendLocal).CountRunning(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(n).To(Equal(0))
			},
		)
		It("routes docker (default) to a non-nil counter", func() {
			Expect(createContainerCounter(config.BackendDocker)).NotTo(BeNil())
		})
	})

	Describe("createExecutor", func() {
		It("returns a non-nil executor for both backends", func() {
			fmtr := formatter.NewFormatter(cdtg)
			policy := launchpolicy.Policy{}
			Expect(
				createExecutor(config.BackendLocal, policy, "model", 0, cdtg, fmtr),
			).NotTo(BeNil())
			Expect(
				createExecutor(config.BackendDocker, policy, "model", 0, cdtg, fmtr),
			).NotTo(BeNil())
		})
	})
})
