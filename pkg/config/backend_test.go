// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
)

var _ = Describe("Backend", func() {
	ctx := context.Background()

	Describe("Validate", func() {
		It("succeeds for BackendDocker", func() {
			err := config.BackendDocker.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds for BackendLocal", func() {
			err := config.BackendLocal.Validate(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails for an unknown value", func() {
			err := config.Backend("bogus").Validate(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("docker, local"))
		})
	})

	Describe("AvailableBackends", func() {
		It("Contains BackendDocker", func() {
			Expect(config.AvailableBackends.Contains(config.BackendDocker)).To(BeTrue())
		})

		It("Contains BackendLocal", func() {
			Expect(config.AvailableBackends.Contains(config.BackendLocal)).To(BeTrue())
		})

		It("does not contain a bogus backend", func() {
			Expect(config.AvailableBackends.Contains(config.Backend("bogus"))).To(BeFalse())
		})
	})

	Describe("String", func() {
		It("returns docker for BackendDocker", func() {
			Expect(config.BackendDocker.String()).To(Equal("docker"))
		})

		It("returns local for BackendLocal", func() {
			Expect(config.BackendLocal.String()).To(Equal("local"))
		})
	})

	Describe("Ptr", func() {
		It("returns a pointer to the backend value", func() {
			ptr := config.BackendLocal.Ptr()
			Expect(ptr).NotTo(BeNil())
			Expect(*ptr).To(Equal(config.BackendLocal))
		})
	})
})
