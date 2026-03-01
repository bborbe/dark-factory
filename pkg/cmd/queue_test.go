// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("QueueCommand", func() {
	var (
		tempDir           string
		inboxDir          string
		queueDir          string
		mockPromptManager *mocks.Manager
		queueCmd          cmd.QueueCommand
		ctx               context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "queue-test-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir = filepath.Join(tempDir, "inbox")
		queueDir = filepath.Join(tempDir, "queue")

		err = os.MkdirAll(inboxDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		mockPromptManager = &mocks.Manager{}
		mockPromptManager.NormalizeFilenamesReturns([]prompt.Rename{}, nil)

		queueCmd = cmd.NewQueueCommand(inboxDir, queueDir, mockPromptManager)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Queue single file", func() {
		It("queues a file from inbox to queue", func() {
			// Create test file in inbox
			testFile := filepath.Join(inboxDir, "test.md")
			err := os.WriteFile(testFile, []byte("# Test Prompt\n\nContent"), 0600)
			Expect(err).NotTo(HaveOccurred())

			// Run queue command
			err = queueCmd.Run(ctx, []string{"test.md"})
			Expect(err).NotTo(HaveOccurred())

			// Verify file moved to queue
			_, err = os.Stat(testFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			queuedFile := filepath.Join(queueDir, "test.md")
			_, err = os.Stat(queuedFile)
			Expect(err).NotTo(HaveOccurred())

			// Verify frontmatter was set
			content, err := os.ReadFile(queuedFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: queued"))

			// Verify NormalizeFilenames was called
			Expect(mockPromptManager.NormalizeFilenamesCallCount()).To(Equal(1))
		})

		It("returns error for nonexistent file", func() {
			err := queueCmd.Run(ctx, []string{"nonexistent.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("file not found"))
		})

		It("handles file with normalization", func() {
			// Create test file in inbox
			testFile := filepath.Join(inboxDir, "test.md")
			err := os.WriteFile(testFile, []byte("# Test Prompt\n\nContent"), 0600)
			Expect(err).NotTo(HaveOccurred())

			// Mock normalization to rename the file
			mockPromptManager.NormalizeFilenamesReturns([]prompt.Rename{
				{
					OldPath: filepath.Join(queueDir, "test.md"),
					NewPath: filepath.Join(queueDir, "001-test.md"),
				},
			}, nil)

			// Run queue command
			err = queueCmd.Run(ctx, []string{"test.md"})
			Expect(err).NotTo(HaveOccurred())

			// Verify NormalizeFilenames was called
			Expect(mockPromptManager.NormalizeFilenamesCallCount()).To(Equal(1))
		})
	})

	Describe("Queue all files", func() {
		It("queues all .md files from inbox to queue", func() {
			// Create test files in inbox
			testFile1 := filepath.Join(inboxDir, "test1.md")
			testFile2 := filepath.Join(inboxDir, "test2.md")
			err := os.WriteFile(testFile1, []byte("# Test 1"), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(testFile2, []byte("# Test 2"), 0600)
			Expect(err).NotTo(HaveOccurred())

			// Run queue command with no args
			err = queueCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())

			// Verify files moved to queue
			_, err = os.Stat(testFile1)
			Expect(os.IsNotExist(err)).To(BeTrue())
			_, err = os.Stat(testFile2)
			Expect(os.IsNotExist(err)).To(BeTrue())

			queuedFile1 := filepath.Join(queueDir, "test1.md")
			queuedFile2 := filepath.Join(queueDir, "test2.md")
			_, err = os.Stat(queuedFile1)
			Expect(err).NotTo(HaveOccurred())
			_, err = os.Stat(queuedFile2)
			Expect(err).NotTo(HaveOccurred())

			// Verify NormalizeFilenames was called twice (once per file)
			Expect(mockPromptManager.NormalizeFilenamesCallCount()).To(Equal(2))
		})

		It("skips non-.md files", func() {
			// Create test files in inbox
			testFile := filepath.Join(inboxDir, "test.md")
			txtFile := filepath.Join(inboxDir, "test.txt")
			err := os.WriteFile(testFile, []byte("# Test"), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(txtFile, []byte("text"), 0600)
			Expect(err).NotTo(HaveOccurred())

			// Run queue command
			err = queueCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())

			// Verify only .md file moved
			_, err = os.Stat(testFile)
			Expect(os.IsNotExist(err)).To(BeTrue())
			_, err = os.Stat(txtFile)
			Expect(err).NotTo(HaveOccurred()) // txt file should still exist

			// Verify NormalizeFilenames was called once (for the .md file)
			Expect(mockPromptManager.NormalizeFilenamesCallCount()).To(Equal(1))
		})

		It("skips subdirectories", func() {
			// Create subdirectory in inbox
			subdir := filepath.Join(inboxDir, "subdir")
			err := os.MkdirAll(subdir, 0750)
			Expect(err).NotTo(HaveOccurred())

			// Create test file in subdirectory
			testFile := filepath.Join(subdir, "test.md")
			err = os.WriteFile(testFile, []byte("# Test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			// Run queue command
			err = queueCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())

			// Verify file in subdirectory was not moved
			_, err = os.Stat(testFile)
			Expect(err).NotTo(HaveOccurred())

			// Verify NormalizeFilenames was not called
			Expect(mockPromptManager.NormalizeFilenamesCallCount()).To(Equal(0))
		})

		It("handles empty inbox", func() {
			// Run queue command with empty inbox
			err := queueCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())

			// Verify NormalizeFilenames was not called
			Expect(mockPromptManager.NormalizeFilenamesCallCount()).To(Equal(0))
		})
	})
})
