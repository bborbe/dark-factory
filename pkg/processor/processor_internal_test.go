// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("determineBump", func() {
	var (
		tempDir     string
		originalDir string
	)

	BeforeEach(func() {
		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		tempDir, err = os.MkdirTemp("", "determinebump-test-*")
		Expect(err).NotTo(HaveOccurred())

		err = os.Chdir(tempDir)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if originalDir != "" {
			err := os.Chdir(originalDir)
			Expect(err).NotTo(HaveOccurred())
		}

		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("returns MinorBump for changelog with 'add'", func() {
		err := os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- Add container name tracking\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for changelog with 'Add' (capitalized)", func() {
		err := os.WriteFile("CHANGELOG.md", []byte("## Unreleased\n\n- Add new feature\n"), 0600)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for changelog with 'implement'", func() {
		err := os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- Implement authentication\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for changelog with 'new'", func() {
		err := os.WriteFile("CHANGELOG.md", []byte("## Unreleased\n\n- New logging system\n"), 0600)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for changelog with 'support'", func() {
		err := os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- Support multiple databases\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for changelog with 'feature'", func() {
		err := os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- Feature flag system\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns PatchBump for changelog with 'fix'", func() {
		err := os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- Fix frontmatter parser\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for changelog with 'refactor'", func() {
		err := os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- Refactor executor logic\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for changelog with 'update'", func() {
		err := os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- Update dependencies\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for generic changelog content", func() {
		err := os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- Improve performance\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns MinorBump when keyword is part of larger word", func() {
		err := os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- Address authentication issues\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns PatchBump when CHANGELOG.md does not exist", func() {
		bump := determineBump()
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump when no Unreleased section", func() {
		err := os.WriteFile(
			"CHANGELOG.md",
			[]byte("# Changelog\n\n## v1.0.0\n\n- Initial release\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := determineBump()
		Expect(bump).To(Equal(git.PatchBump))
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
