// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("UnapproveCommand", func() {
	var (
		tempDir       string
		inboxDir      string
		queueDir      string
		promptManager *mocks.CmdPromptManager
		unapproveCmd  cmd.UnapproveCommand
		ctx           context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "unapprove-test-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir = filepath.Join(tempDir, "inbox")
		queueDir = filepath.Join(tempDir, "queue")

		err = os.MkdirAll(inboxDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		promptManager = &mocks.CmdPromptManager{}
		promptManager.NormalizeFilenamesReturns([]prompt.Rename{}, nil)
		realPM := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime())
		promptManager.LoadStub = func(ctx context.Context, path string) (*prompt.PromptFile, error) {
			return realPM.Load(ctx, path)
		}

		unapproveCmd = cmd.NewUnapproveCommand(
			inboxDir,
			queueDir,
			promptManager,
		)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("No args", func() {
		It("returns error with usage message", func() {
			err := unapproveCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("usage: dark-factory prompt unapprove <file>"))
		})
	})

	Describe("Unapprove approved prompt", func() {
		It(
			"moves approved file from queue to inbox with number stripped and sets status to draft",
			func() {
				queueFile := filepath.Join(queueDir, "010-fix-something.md")
				err := os.WriteFile(
					queueFile,
					[]byte("---\nstatus: approved\n---\n# Fix Something"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				err = unapproveCmd.Run(ctx, []string{"010-fix-something.md"})
				Expect(err).NotTo(HaveOccurred())

				// Queue file should be gone
				_, err = os.Stat(queueFile)
				Expect(os.IsNotExist(err)).To(BeTrue())

				// Inbox file should exist with number stripped
				inboxFile := filepath.Join(inboxDir, "fix-something.md")
				_, err = os.Stat(inboxFile)
				Expect(err).NotTo(HaveOccurred())

				content, err := os.ReadFile(inboxFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(ContainSubstring("status: draft"))

				Expect(promptManager.NormalizeFilenamesCallCount()).To(Equal(1))
			},
		)

		It("matches prompt by numeric short ID", func() {
			queueFile := filepath.Join(queueDir, "005-workflow.md")
			err := os.WriteFile(queueFile, []byte("---\nstatus: approved\n---\n# Workflow"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = unapproveCmd.Run(ctx, []string{"005"})
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(queueFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			inboxFile := filepath.Join(inboxDir, "workflow.md")
			_, err = os.Stat(inboxFile)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Renumbering remaining prompts", func() {
		It("calls NormalizeFilenames after removing the prompt", func() {
			queueFile := filepath.Join(queueDir, "010-fix.md")
			err := os.WriteFile(queueFile, []byte("---\nstatus: approved\n---\n# Fix"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = unapproveCmd.Run(ctx, []string{"010"})
			Expect(err).NotTo(HaveOccurred())

			Expect(promptManager.NormalizeFilenamesCallCount()).To(Equal(1))
			_, dir := promptManager.NormalizeFilenamesArgsForCall(0)
			Expect(dir).To(Equal(queueDir))
		})
	})

	Describe("Error cases", func() {
		It("returns error when file not found", func() {
			err := unapproveCmd.Run(ctx, []string{"999"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("file not found"))
		})

		It("returns error when prompt is executing", func() {
			queueFile := filepath.Join(queueDir, "010-executing.md")
			err := os.WriteFile(queueFile, []byte("---\nstatus: executing\n---\n# Executing"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = unapproveCmd.Run(ctx, []string{"010"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot unapprove prompt with status"))
		})

		It("returns error when prompt is completed", func() {
			queueFile := filepath.Join(queueDir, "010-completed.md")
			err := os.WriteFile(queueFile, []byte("---\nstatus: completed\n---\n# Completed"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = unapproveCmd.Run(ctx, []string{"010"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot unapprove prompt with status"))
		})

		It("returns error when prompt is draft (not approved)", func() {
			queueFile := filepath.Join(queueDir, "010-draft.md")
			err := os.WriteFile(queueFile, []byte("---\nstatus: draft\n---\n# Draft"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = unapproveCmd.Run(ctx, []string{"010"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot unapprove prompt with status"))
		})
	})
})
