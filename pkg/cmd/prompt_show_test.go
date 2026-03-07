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

	"github.com/bborbe/dark-factory/pkg/cmd"
)

var _ = Describe("PromptShowCommand", func() {
	var (
		tempDir       string
		inboxDir      string
		inProgressDir string
		completedDir  string
		logDir        string
		promptShowCmd cmd.PromptShowCommand
		ctx           context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "prompt-show-test-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir = filepath.Join(tempDir, "inbox")
		inProgressDir = filepath.Join(tempDir, "in-progress")
		completedDir = filepath.Join(tempDir, "completed")
		logDir = filepath.Join(tempDir, "log")

		Expect(os.MkdirAll(inboxDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(logDir, 0750)).To(Succeed())

		promptShowCmd = cmd.NewPromptShowCommand(inboxDir, inProgressDir, completedDir, logDir)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Run", func() {
		It("returns error when no identifier given", func() {
			err := promptShowCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prompt identifier required"))
		})

		It("returns error when prompt not found", func() {
			err := promptShowCmd.Run(ctx, []string{"999-nonexistent.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prompt not found"))
		})

		It("shows prompt details for a valid ID", func() {
			promptFile := filepath.Join(inProgressDir, "001-my-prompt.md")
			Expect(
				os.WriteFile(promptFile, []byte("---\nstatus: approved\n---\n# My Prompt"), 0600),
			).To(Succeed())

			err := promptShowCmd.Run(ctx, []string{"001-my-prompt.md"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("shows prompt from inboxDir", func() {
			promptFile := filepath.Join(inboxDir, "002-inbox-prompt.md")
			Expect(
				os.WriteFile(promptFile, []byte("---\nstatus: draft\n---\n# Inbox Prompt"), 0600),
			).To(Succeed())

			err := promptShowCmd.Run(ctx, []string{"002"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("shows prompt from completedDir", func() {
			promptFile := filepath.Join(completedDir, "003-done-prompt.md")
			Expect(
				os.WriteFile(
					promptFile,
					[]byte("---\nstatus: completed\ncompleted: 2026-01-01T00:00:00Z\n---\n# Done"),
					0600,
				),
			).To(Succeed())

			err := promptShowCmd.Run(ctx, []string{"003"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("includes log path when log file exists", func() {
			promptFile := filepath.Join(inProgressDir, "004-logged-prompt.md")
			Expect(
				os.WriteFile(promptFile, []byte("---\nstatus: executing\n---\n# Logged"), 0600),
			).To(Succeed())
			logFile := filepath.Join(logDir, "004-logged-prompt.log")
			Expect(os.WriteFile(logFile, []byte("some log output"), 0600)).To(Succeed())

			err := promptShowCmd.Run(ctx, []string{"004"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("outputs JSON when --json flag is given", func() {
			promptFile := filepath.Join(completedDir, "005-json-prompt.md")
			Expect(
				os.WriteFile(promptFile, []byte("---\nstatus: completed\n---\n# JSON"), 0600),
			).To(Succeed())

			err := promptShowCmd.Run(ctx, []string{"--json", "005"})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
