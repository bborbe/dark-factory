// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/version"
)

var _ = Describe("Version", func() {
	Context("Getter", func() {
		It("returns the version passed to constructor", func() {
			getter := version.NewGetter("v1.2.3")
			Expect(getter.Get()).To(Equal("v1.2.3"))
		})

		It("returns dev when no version is set", func() {
			getter := version.NewGetter("dev")
			Expect(getter.Get()).To(Equal("dev"))
		})
	})

	Context("Version variable", func() {
		It("defaults to dev", func() {
			// This test verifies the default value
			// The actual value may be overridden at build time
			Expect(version.Version).To(Equal("dev"))
		})
	})
})
