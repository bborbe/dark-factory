// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spec_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/spec"
)

// writePrompt writes a minimal prompt .md file with the given status and spec fields.
func writePrompt(path, status, specID string) {
	fm := fmt.Sprintf("---\nstatus: %s\n", status)
	if specID != "" {
		fm += fmt.Sprintf("spec: %s\n", specID)
	}
	fm += "---\n# Test\n\nContent.\n"
	Expect(os.WriteFile(path, []byte(fm), 0600)).To(Succeed())
}

// writeSpec writes a minimal spec .md file with the given status.
func writeSpec(path, status string) {
	content := fmt.Sprintf("---\nstatus: %s\n---\n# Spec\n\nDescription.\n", status)
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
}

var _ = Describe("Load", func() {
	var ctx context.Context
	var dir string

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		dir, err = os.MkdirTemp("", "spec-load-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(dir)
	})

	Context("with valid frontmatter", func() {
		It("parses the status correctly", func() {
			path := filepath.Join(dir, "019-native.md")
			writeSpec(path, "approved")

			sf, err := spec.Load(ctx, path)
			Expect(err).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal("approved"))
			Expect(sf.Name).To(Equal("019-native"))
			Expect(sf.Path).To(Equal(path))
		})
	})

	Context("without frontmatter", func() {
		It("returns empty status", func() {
			path := filepath.Join(dir, "no-fm.md")
			Expect(
				os.WriteFile(path, []byte("# Just a title\n\nNo frontmatter here.\n"), 0600),
			).To(Succeed())

			sf, err := spec.Load(ctx, path)
			Expect(err).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal(""))
			Expect(sf.Name).To(Equal("no-fm"))
		})
	})
})

var _ = Describe("SetStatus and Save", func() {
	var ctx context.Context
	var dir string

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		dir, err = os.MkdirTemp("", "spec-save-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(dir)
	})

	It("roundtrips status correctly", func() {
		path := filepath.Join(dir, "001-spec.md")
		writeSpec(path, "draft")

		sf, err := spec.Load(ctx, path)
		Expect(err).NotTo(HaveOccurred())
		Expect(sf.Frontmatter.Status).To(Equal("draft"))

		sf.SetStatus("approved")
		Expect(sf.Save(ctx)).To(Succeed())

		sf2, err := spec.Load(ctx, path)
		Expect(err).NotTo(HaveOccurred())
		Expect(sf2.Frontmatter.Status).To(Equal("approved"))
	})
})

var _ = Describe("Lister", func() {
	var ctx context.Context
	var dir string
	var lister spec.Lister

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		dir, err = os.MkdirTemp("", "spec-lister-*")
		Expect(err).NotTo(HaveOccurred())
		lister = spec.NewLister(dir)
	})

	AfterEach(func() {
		_ = os.RemoveAll(dir)
	})

	Describe("Summary", func() {
		It("counts specs by status correctly", func() {
			writeSpec(filepath.Join(dir, "001-a.md"), "draft")
			writeSpec(filepath.Join(dir, "002-b.md"), "draft")
			writeSpec(filepath.Join(dir, "003-c.md"), "approved")
			writeSpec(filepath.Join(dir, "004-d.md"), "prompted")
			writeSpec(filepath.Join(dir, "005-e.md"), "verifying")
			writeSpec(filepath.Join(dir, "006-f.md"), "completed")

			s, err := lister.Summary(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Total).To(Equal(6))
			Expect(s.Draft).To(Equal(2))
			Expect(s.Approved).To(Equal(1))
			Expect(s.Prompted).To(Equal(1))
			Expect(s.Verifying).To(Equal(1))
			Expect(s.Completed).To(Equal(1))
		})

		It("returns empty summary for empty directory", func() {
			s, err := lister.Summary(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Total).To(Equal(0))
		})

		It("returns empty summary for non-existent directory", func() {
			l := spec.NewLister("/nonexistent/path")
			s, err := l.Summary(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Total).To(Equal(0))
		})
	})

	Describe("List", func() {
		It("returns all .md files", func() {
			writeSpec(filepath.Join(dir, "001-a.md"), "draft")
			writeSpec(filepath.Join(dir, "002-b.md"), "approved")

			specs, err := lister.List(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(specs).To(HaveLen(2))
		})

		It("ignores non-.md files", func() {
			writeSpec(filepath.Join(dir, "001-a.md"), "draft")
			Expect(
				os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("text"), 0600),
			).To(Succeed())

			specs, err := lister.List(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(specs).To(HaveLen(1))
		})
	})
})

var _ = Describe("AutoCompleter", func() {
	var (
		ctx          context.Context
		queueDir     string
		completedDir string
		specsDir     string
		ac           spec.AutoCompleter
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		queueDir, err = os.MkdirTemp("", "spec-test-queue-*")
		Expect(err).NotTo(HaveOccurred())

		completedDir, err = os.MkdirTemp("", "spec-test-completed-*")
		Expect(err).NotTo(HaveOccurred())

		specsDir, err = os.MkdirTemp("", "spec-test-specs-*")
		Expect(err).NotTo(HaveOccurred())

		ac = spec.NewAutoCompleter(queueDir, completedDir, specsDir)
	})

	AfterEach(func() {
		_ = os.RemoveAll(queueDir)
		_ = os.RemoveAll(completedDir)
		_ = os.RemoveAll(specsDir)
	})

	Context("when specID is empty", func() {
		It("does nothing and returns nil", func() {
			err := ac.CheckAndComplete(ctx, "")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when all linked prompts are completed", func() {
		It("marks the spec as verifying", func() {
			// Two completed prompts referencing spec-001
			writePrompt(filepath.Join(completedDir, "001-first.md"), "completed", "spec-001")
			writePrompt(filepath.Join(completedDir, "002-second.md"), "completed", "spec-001")

			// Create spec file with status queued
			writeSpec(filepath.Join(specsDir, "spec-001.md"), "queued")

			err := ac.CheckAndComplete(ctx, "spec-001")
			Expect(err).NotTo(HaveOccurred())

			// Verify spec file is now verifying
			sf, loadErr := spec.Load(ctx, filepath.Join(specsDir, "spec-001.md"))
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal("verifying"))
		})
	})

	Context("when some linked prompts are still in queue", func() {
		It("does NOT mark the spec as completed", func() {
			// One completed, one still queued
			writePrompt(filepath.Join(completedDir, "001-first.md"), "completed", "spec-002")
			writePrompt(filepath.Join(queueDir, "002-second.md"), "queued", "spec-002")

			// Create spec file with status queued
			writeSpec(filepath.Join(specsDir, "spec-002.md"), "queued")

			err := ac.CheckAndComplete(ctx, "spec-002")
			Expect(err).NotTo(HaveOccurred())

			// Verify spec file is still queued
			sf, loadErr := spec.Load(ctx, filepath.Join(specsDir, "spec-002.md"))
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal("queued"))
		})
	})

	Context("when spec is already completed", func() {
		It("is a no-op", func() {
			writePrompt(filepath.Join(completedDir, "001-first.md"), "completed", "spec-003")

			// Create spec file already completed
			writeSpec(filepath.Join(specsDir, "spec-003.md"), "completed")

			err := ac.CheckAndComplete(ctx, "spec-003")
			Expect(err).NotTo(HaveOccurred())

			// Status unchanged
			sf, loadErr := spec.Load(ctx, filepath.Join(specsDir, "spec-003.md"))
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal("completed"))
		})
	})

	Context("when spec is already verifying", func() {
		It("is a no-op", func() {
			writePrompt(filepath.Join(completedDir, "001-first.md"), "completed", "spec-009")

			// Create spec file already in verifying state
			writeSpec(filepath.Join(specsDir, "spec-009.md"), "verifying")

			err := ac.CheckAndComplete(ctx, "spec-009")
			Expect(err).NotTo(HaveOccurred())

			// Status unchanged
			sf, loadErr := spec.Load(ctx, filepath.Join(specsDir, "spec-009.md"))
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal("verifying"))
		})
	})

	Context("when no prompts reference the spec", func() {
		It("does nothing", func() {
			// No prompts link to spec-004
			writeSpec(filepath.Join(specsDir, "spec-004.md"), "queued")

			err := ac.CheckAndComplete(ctx, "spec-004")
			Expect(err).NotTo(HaveOccurred())

			// Status unchanged
			sf, loadErr := spec.Load(ctx, filepath.Join(specsDir, "spec-004.md"))
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal("queued"))
		})
	})

	Context("when prompts use multi-spec array", func() {
		It("counts prompts that include the spec in their array", func() {
			// Prompt belongs to both spec-005 and spec-006 — completed
			fm := "---\nstatus: completed\nspec: [\"spec-005\", \"spec-006\"]\n---\n# Test\n\nContent.\n"
			Expect(
				os.WriteFile(filepath.Join(completedDir, "001-multi.md"), []byte(fm), 0600),
			).To(Succeed())

			writeSpec(filepath.Join(specsDir, "spec-005.md"), "queued")

			err := ac.CheckAndComplete(ctx, "spec-005")
			Expect(err).NotTo(HaveOccurred())

			sf, loadErr := spec.Load(ctx, filepath.Join(specsDir, "spec-005.md"))
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal("verifying"))
		})

		It("does not complete spec when one of multiple prompts is still queued", func() {
			// Prompt 1: completed, references spec-007
			writePrompt(filepath.Join(completedDir, "001-done.md"), "completed", "spec-007")
			// Prompt 2: queued, multi-spec including spec-007
			fm := "---\nstatus: queued\nspec: [\"spec-007\", \"spec-008\"]\n---\n# Test\n\nContent.\n"
			Expect(
				os.WriteFile(filepath.Join(queueDir, "002-multi.md"), []byte(fm), 0600),
			).To(Succeed())

			writeSpec(filepath.Join(specsDir, "spec-007.md"), "queued")

			err := ac.CheckAndComplete(ctx, "spec-007")
			Expect(err).NotTo(HaveOccurred())

			sf, loadErr := spec.Load(ctx, filepath.Join(specsDir, "spec-007.md"))
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal("queued"))
		})
	})
})
