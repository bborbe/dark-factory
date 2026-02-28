// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("Git", func() {
	Describe("BumpPatchVersion", func() {
		Context("with valid semver tags", func() {
			It("bumps patch version from v0.1.0", func() {
				result, err := git.BumpPatchVersion("v0.1.0")
				Expect(err).To(BeNil())
				Expect(result).To(Equal("v0.1.1"))
			})

			It("bumps patch version from v1.2.3", func() {
				result, err := git.BumpPatchVersion("v1.2.3")
				Expect(err).To(BeNil())
				Expect(result).To(Equal("v1.2.4"))
			})

			It("bumps patch version from v10.20.99", func() {
				result, err := git.BumpPatchVersion("v10.20.99")
				Expect(err).To(BeNil())
				Expect(result).To(Equal("v10.20.100"))
			})
		})

		Context("with invalid tags", func() {
			It("returns error for non-semver tag", func() {
				_, err := git.BumpPatchVersion("invalid")
				Expect(err).NotTo(BeNil())
			})

			It("returns error for tag without v prefix", func() {
				_, err := git.BumpPatchVersion("1.2.3")
				Expect(err).NotTo(BeNil())
			})

			It("returns error for incomplete version", func() {
				_, err := git.BumpPatchVersion("v1.2")
				Expect(err).NotTo(BeNil())
			})
		})
	})
})
