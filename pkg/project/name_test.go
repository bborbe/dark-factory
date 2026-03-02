// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/project"
)

var _ = Describe("Name", func() {
	Context("with config override", func() {
		It("returns the override value", func() {
			result := project.Name("my-custom-project")
			Expect(result).To(Equal("my-custom-project"))
		})
	})

	Context("without config override", func() {
		It("returns a non-empty string", func() {
			result := project.Name("")
			Expect(result).NotTo(BeEmpty())
		})

		It("returns a valid container name prefix", func() {
			result := project.Name("")
			// Should not contain invalid Docker name characters
			Expect(result).To(MatchRegexp(`^[a-zA-Z0-9._-]+$`))
		})
	})

	Context("edge cases", func() {
		It("handles empty override by auto-detecting", func() {
			result := project.Name("")
			Expect(result).NotTo(BeEmpty())
			Expect(result).NotTo(Equal(""))
		})
	})
})
