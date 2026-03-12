// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("DetermineBumpFromChangelog", func() {
	var ctx context.Context
	var dir string

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		dir, err = os.MkdirTemp("", "changelog-test-*")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(dir)).To(Succeed())
	})

	Context("when CHANGELOG.md is missing", func() {
		It("returns PatchBump", func() {
			bump := git.DetermineBumpFromChangelog(ctx, dir)
			Expect(bump).To(Equal(git.PatchBump))
		})
	})

	Context("when CHANGELOG.md has no ## Unreleased section", func() {
		BeforeEach(func() {
			content := "# Changelog\n\n## v1.0.0\n\n- fix: some fix\n"
			Expect(
				os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(content), 0600),
			).To(Succeed())
		})

		It("returns PatchBump", func() {
			bump := git.DetermineBumpFromChangelog(ctx, dir)
			Expect(bump).To(Equal(git.PatchBump))
		})
	})

	Context("when ## Unreleased contains only fix lines", func() {
		BeforeEach(func() {
			content := "# Changelog\n\n## Unreleased\n\n- fix: correct typo\n- chore: update deps\n\n## v1.0.0\n"
			Expect(
				os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(content), 0600),
			).To(Succeed())
		})

		It("returns PatchBump", func() {
			bump := git.DetermineBumpFromChangelog(ctx, dir)
			Expect(bump).To(Equal(git.PatchBump))
		})
	})

	Context("when ## Unreleased contains a feat: line", func() {
		BeforeEach(func() {
			content := "# Changelog\n\n## Unreleased\n\n- fix: some fix\n- feat: add new feature\n\n## v1.0.0\n"
			Expect(
				os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(content), 0600),
			).To(Succeed())
		})

		It("returns MinorBump", func() {
			bump := git.DetermineBumpFromChangelog(ctx, dir)
			Expect(bump).To(Equal(git.MinorBump))
		})
	})

	Context("when ## Unreleased contains only feat: lines", func() {
		BeforeEach(func() {
			content := "# Changelog\n\n## Unreleased\n\n- feat: new feature\n"
			Expect(
				os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(content), 0600),
			).To(Succeed())
		})

		It("returns MinorBump", func() {
			bump := git.DetermineBumpFromChangelog(ctx, dir)
			Expect(bump).To(Equal(git.MinorBump))
		})
	})
})
