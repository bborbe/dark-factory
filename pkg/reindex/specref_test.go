// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reindex_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/reindex"
)

func writePromptFile(path string, spec string, extraFrontmatter string) {
	content := "---\n"
	if spec != "" {
		content += fmt.Sprintf("spec: [\"%s\"]\n", spec)
	}
	content += "status: created\n"
	if extraFrontmatter != "" {
		content += extraFrontmatter + "\n"
	}
	content += "---\n\nbody\n"
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
}

func loadPromptFrontmatter(path string, pm reindex.PromptManager) prompt.Frontmatter {
	pf, err := pm.Load(context.Background(), path)
	Expect(err).NotTo(HaveOccurred())
	Expect(pf).NotTo(BeNil())
	return pf.Frontmatter
}

var _ = Describe("UpdateSpecRefs", func() {
	var (
		ctx   context.Context
		mover reindex.FileMover
		pm    *prompt.Manager
	)

	BeforeEach(func() {
		ctx = context.Background()
		mover = &osFileMover{}
		pm = prompt.NewManager("", "", "", mover, libtime.NewCurrentDateTime())
	})

	It("no spec renames returns nil, nil", func() {
		dir := GinkgoT().TempDir()
		writePromptFile(filepath.Join(dir, "001-prompt.md"), "035", "")

		renames, err := reindex.UpdateSpecRefs(
			ctx,
			nil,
			[]string{dir},
			mover,
			pm,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(renames).To(BeNil())

		// File unchanged
		fm := loadPromptFrontmatter(filepath.Join(dir, "001-prompt.md"), pm)
		Expect([]string(fm.Specs)).To(Equal([]string{"035"}))
	})

	It("updates frontmatter spec field when spec is renamed", func() {
		dir := GinkgoT().TempDir()
		promptPath := filepath.Join(dir, "001-prompt.md")
		writePromptFile(promptPath, "035", "")

		specRenames := []reindex.Rename{
			{
				OldPath: "/specs/in-progress/035-old-spec.md",
				NewPath: "/specs/in-progress/043-old-spec.md",
			},
		}

		_, err := reindex.UpdateSpecRefs(
			ctx,
			specRenames,
			[]string{dir},
			mover,
			pm,
		)
		Expect(err).NotTo(HaveOccurred())

		fm := loadPromptFrontmatter(promptPath, pm)
		Expect([]string(fm.Specs)).To(Equal([]string{"043"}))
	})

	It("renames prompt file with spec-NNN pattern", func() {
		dir := GinkgoT().TempDir()
		oldName := "050-spec-035-foo.md"
		newName := "050-spec-043-foo.md"
		writePromptFile(filepath.Join(dir, oldName), "", "")

		specRenames := []reindex.Rename{
			{
				OldPath: "/specs/035-old-spec.md",
				NewPath: "/specs/043-old-spec.md",
			},
		}

		renames, err := reindex.UpdateSpecRefs(
			ctx,
			specRenames,
			[]string{dir},
			mover,
			pm,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(renames).To(HaveLen(1))
		Expect(filepath.Base(renames[0].OldPath)).To(Equal(oldName))
		Expect(filepath.Base(renames[0].NewPath)).To(Equal(newName))

		Expect(filepath.Join(dir, oldName)).NotTo(BeAnExistingFile())
		Expect(filepath.Join(dir, newName)).To(BeAnExistingFile())
	})

	It("updates both frontmatter and filename", func() {
		dir := GinkgoT().TempDir()
		oldName := "050-spec-035-foo.md"
		newName := "050-spec-043-foo.md"
		writePromptFile(filepath.Join(dir, oldName), "035", "")

		specRenames := []reindex.Rename{
			{
				OldPath: "/specs/035-old-spec.md",
				NewPath: "/specs/043-old-spec.md",
			},
		}

		renames, err := reindex.UpdateSpecRefs(
			ctx,
			specRenames,
			[]string{dir},
			mover,
			pm,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(renames).To(HaveLen(1))

		newPath := filepath.Join(dir, newName)
		Expect(newPath).To(BeAnExistingFile())

		fm := loadPromptFrontmatter(newPath, pm)
		Expect([]string(fm.Specs)).To(Equal([]string{"043"}))
	})

	It("does not touch unrelated prompts", func() {
		dir := GinkgoT().TempDir()
		promptPath := filepath.Join(dir, "001-prompt.md")
		writePromptFile(promptPath, "020", "")

		specRenames := []reindex.Rename{
			{
				OldPath: "/specs/035-old-spec.md",
				NewPath: "/specs/043-old-spec.md",
			},
		}

		_, err := reindex.UpdateSpecRefs(
			ctx,
			specRenames,
			[]string{dir},
			mover,
			pm,
		)
		Expect(err).NotTo(HaveOccurred())

		// Spec ref unchanged
		fm := loadPromptFrontmatter(promptPath, pm)
		Expect([]string(fm.Specs)).To(Equal([]string{"020"}))
	})

	It("handles missing promptDirs gracefully", func() {
		nonExistent := filepath.Join(GinkgoT().TempDir(), "no-such-dir")

		specRenames := []reindex.Rename{
			{
				OldPath: "/specs/035-old-spec.md",
				NewPath: "/specs/043-old-spec.md",
			},
		}

		renames, err := reindex.UpdateSpecRefs(
			ctx,
			specRenames,
			[]string{nonExistent},
			mover,
			pm,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(renames).To(BeNil())
	})

	It("handles multiple spec renames in one call", func() {
		dir := GinkgoT().TempDir()
		writePromptFile(filepath.Join(dir, "001-spec-035-alpha.md"), "035", "")
		writePromptFile(filepath.Join(dir, "002-spec-020-beta.md"), "020", "")
		writePromptFile(filepath.Join(dir, "003-unrelated.md"), "010", "")

		specRenames := []reindex.Rename{
			{
				OldPath: "/specs/035-alpha.md",
				NewPath: "/specs/043-alpha.md",
			},
			{
				OldPath: "/specs/020-beta.md",
				NewPath: "/specs/050-beta.md",
			},
		}

		renames, err := reindex.UpdateSpecRefs(
			ctx,
			specRenames,
			[]string{dir},
			mover,
			pm,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(renames).To(HaveLen(2))

		// Check alpha rename
		fm1 := loadPromptFrontmatter(filepath.Join(dir, "001-spec-043-alpha.md"), pm)
		Expect([]string(fm1.Specs)).To(Equal([]string{"043"}))

		// Check beta rename
		fm2 := loadPromptFrontmatter(filepath.Join(dir, "002-spec-050-beta.md"), pm)
		Expect([]string(fm2.Specs)).To(Equal([]string{"050"}))

		// Unrelated unchanged
		fm3 := loadPromptFrontmatter(filepath.Join(dir, "003-unrelated.md"), pm)
		Expect([]string(fm3.Specs)).To(Equal([]string{"010"}))
	})
})
