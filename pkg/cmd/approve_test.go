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

var _ = Describe("ApproveCommand", func() {
	var (
		tempDir           string
		inboxDir          string
		queueDir          string
		mockPromptManager *mocks.Manager
		approveCmd        cmd.ApproveCommand
		ctx               context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "approve-test-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir = filepath.Join(tempDir, "inbox")
		queueDir = filepath.Join(tempDir, "queue")

		err = os.MkdirAll(inboxDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		mockPromptManager = &mocks.Manager{}
		mockPromptManager.NormalizeFilenamesReturns([]prompt.Rename{}, nil)

		approveCmd = cmd.NewApproveCommand(inboxDir, queueDir, mockPromptManager)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Approve by exact filename", func() {
		It("moves file from inbox to queue", func() {
			testFile := filepath.Join(inboxDir, "080-fix.md")
			err := os.WriteFile(testFile, []byte("# Fix"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = approveCmd.Run(ctx, []string{"080-fix.md"})
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(testFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			queuedFile := filepath.Join(queueDir, "080-fix.md")
			_, err = os.Stat(queuedFile)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(queuedFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))

			Expect(mockPromptManager.NormalizeFilenamesCallCount()).To(Equal(1))
		})
	})

	Describe("Approve by short ID", func() {
		It("matches file by numeric prefix in inbox", func() {
			testFile := filepath.Join(inboxDir, "080-workflow-test.md")
			err := os.WriteFile(testFile, []byte("# Workflow Test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = approveCmd.Run(ctx, []string{"080"})
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(testFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			queuedFile := filepath.Join(queueDir, "080-workflow-test.md")
			_, err = os.Stat(queuedFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("matches file by numeric prefix in queue", func() {
			queueFile := filepath.Join(queueDir, "080-failed.md")
			err := os.WriteFile(queueFile, []byte("---\nstatus: failed\n---\n# Failed"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = approveCmd.Run(ctx, []string{"080"})
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(queueFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})

		It("searches inbox before queue", func() {
			inboxFile := filepath.Join(inboxDir, "080-inbox.md")
			err := os.WriteFile(inboxFile, []byte("# Inbox"), 0600)
			Expect(err).NotTo(HaveOccurred())

			queueFile := filepath.Join(queueDir, "080-queue.md")
			err = os.WriteFile(queueFile, []byte("---\nstatus: failed\n---\n# Queue"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = approveCmd.Run(ctx, []string{"080"})
			Expect(err).NotTo(HaveOccurred())

			// Inbox file should have been moved
			_, err = os.Stat(inboxFile)
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("returns error when file not found", func() {
			err := approveCmd.Run(ctx, []string{"999"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("file not found"))
		})
	})

	Describe("No args", func() {
		It("returns error with usage message", func() {
			err := approveCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("usage: dark-factory approve <file>"))
		})
	})

	Describe("Approve in queue", func() {
		It("sets failed queue file to queued status", func() {
			queueFile := filepath.Join(queueDir, "080-failed.md")
			err := os.WriteFile(queueFile, []byte("---\nstatus: failed\n---\n# Failed"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = approveCmd.Run(ctx, []string{"080-failed.md"})
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(queueFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})
	})
})
