// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("determineBump", func() {
	It("returns MinorBump for title with 'add'", func() {
		bump := determineBump("Add container name tracking")
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for title with 'Add' (capitalized)", func() {
		bump := determineBump("Add new feature")
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for title with 'implement'", func() {
		bump := determineBump("Implement authentication")
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for title with 'new'", func() {
		bump := determineBump("New logging system")
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for title with 'support'", func() {
		bump := determineBump("Support multiple databases")
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for title with 'feature'", func() {
		bump := determineBump("Feature flag system")
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns PatchBump for title with 'fix'", func() {
		bump := determineBump("Fix frontmatter parser")
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for title with 'refactor'", func() {
		bump := determineBump("Refactor executor logic")
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for title with 'update'", func() {
		bump := determineBump("Update dependencies")
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for generic title", func() {
		bump := determineBump("Improve performance")
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns MinorBump when keyword is part of larger word", func() {
		bump := determineBump("Address authentication issues")
		Expect(bump).To(Equal(git.MinorBump))
	})
})

var _ = Describe("sanitizeContainerName", func() {
	It("keeps valid characters", func() {
		name := sanitizeContainerName("abc-123_XYZ")
		Expect(name).To(Equal("abc-123_XYZ"))
	})

	It("replaces special characters with hyphens", func() {
		name := sanitizeContainerName("test@file#name")
		Expect(name).To(Equal("test-file-name"))
	})

	It("handles spaces", func() {
		name := sanitizeContainerName("hello world")
		Expect(name).To(Equal("hello-world"))
	})

	It("handles multiple consecutive special characters", func() {
		name := sanitizeContainerName("test@@##name")
		Expect(name).To(Equal("test----name"))
	})
})
