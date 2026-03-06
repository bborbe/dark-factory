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
		It("marks the spec as completed", func() {
			// Two completed prompts referencing spec-001
			writePrompt(filepath.Join(completedDir, "001-first.md"), "completed", "spec-001")
			writePrompt(filepath.Join(completedDir, "002-second.md"), "completed", "spec-001")

			// Create spec file with status queued
			writeSpec(filepath.Join(specsDir, "spec-001.md"), "queued")

			err := ac.CheckAndComplete(ctx, "spec-001")
			Expect(err).NotTo(HaveOccurred())

			// Verify spec file is now completed
			sf, loadErr := spec.Load(ctx, filepath.Join(specsDir, "spec-001.md"))
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(sf.Frontmatter.Status).To(Equal("completed"))
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
})
