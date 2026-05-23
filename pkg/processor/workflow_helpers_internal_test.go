// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("syncPromptFileToOriginalRepo", func() {
	ctx := context.Background()

	It("is idempotent when destination already exists", func() {
		tempDir := GinkgoT().TempDir()
		inProgressDir := filepath.Join(tempDir, "prompts", "in-progress")
		completedDir := filepath.Join(tempDir, "prompts", "completed")
		Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())

		completedPath := filepath.Join(completedDir, "001-test.md")
		// Destination already exists
		Expect(
			os.WriteFile(completedPath, []byte("---\nstatus: completed\n---\n# Test\n"), 0600),
		).To(Succeed())
		promptPath := filepath.Join(inProgressDir, "001-test.md")
		// Source absent

		promptMgr := prompt.NewManager(
			filepath.Join(tempDir, "prompts", "inbox"),
			inProgressDir,
			completedDir,
			"",
			&osFileMover{},
			libtime.NewCurrentDateTime(),
		)

		err := syncPromptFileToOriginalRepo(ctx, promptMgr, promptPath, completedPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(fileExists(completedPath)).To(BeTrue()) // destination still there
	})

	It("moves file from in-progress to completed when source exists", func() {
		tempDir := GinkgoT().TempDir()
		inProgressDir := filepath.Join(tempDir, "prompts", "in-progress")
		completedDir := filepath.Join(tempDir, "prompts", "completed")
		Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())

		promptPath := filepath.Join(inProgressDir, "001-test.md")
		completedPath := filepath.Join(completedDir, "001-test.md")
		// Source exists with committing status
		Expect(
			os.WriteFile(promptPath, []byte("---\nstatus: committing\n---\n# Test\n"), 0600),
		).To(Succeed())

		// Destination absent

		promptMgr := prompt.NewManager(
			filepath.Join(tempDir, "prompts", "inbox"),
			inProgressDir,
			completedDir,
			"",
			&osFileMover{},
			libtime.NewCurrentDateTime(),
		)

		err := syncPromptFileToOriginalRepo(ctx, promptMgr, promptPath, completedPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(fileExists(promptPath)).To(BeFalse())
		Expect(fileExists(completedPath)).To(BeTrue())
		Expect(readPromptStatus(completedPath)).To(Equal("completed"))
	})

	It("returns clone-sync-mismatch error when both source and destination are absent", func() {
		tempDir := GinkgoT().TempDir()
		inProgressDir := filepath.Join(tempDir, "prompts", "in-progress")
		completedDir := filepath.Join(tempDir, "prompts", "completed")
		Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())

		promptPath := filepath.Join(inProgressDir, "001-test.md")
		completedPath := filepath.Join(completedDir, "001-test.md")
		// Both absent

		promptMgr := prompt.NewManager(
			filepath.Join(tempDir, "prompts", "inbox"),
			inProgressDir,
			completedDir,
			"",
			&osFileMover{},
			libtime.NewCurrentDateTime(),
		)

		err := syncPromptFileToOriginalRepo(ctx, promptMgr, promptPath, completedPath)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("clone-sync-mismatch"))
		Expect(err.Error()).To(ContainSubstring(promptPath))
		Expect(err.Error()).To(ContainSubstring(completedPath))
	})
})
