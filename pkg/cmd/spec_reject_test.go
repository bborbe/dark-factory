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

	"github.com/bborbe/dark-factory/pkg/cmd"
)

var _ = Describe("SpecRejectCommand", func() {
	var (
		tempDir              string
		specsInboxDir        string
		specsInProgressDir   string
		specsRejectedDir     string
		promptsInboxDir      string
		promptsInProgressDir string
		promptsCompletedDir  string
		promptsRejectedDir   string
		specRejectCmd        cmd.SpecRejectCommand
		ctx                  context.Context
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()

		specsInboxDir = filepath.Join(tempDir, "specs")
		specsInProgressDir = filepath.Join(tempDir, "specs", "in-progress")
		specsRejectedDir = filepath.Join(tempDir, "specs", "rejected")
		promptsInboxDir = filepath.Join(tempDir, "prompts")
		promptsInProgressDir = filepath.Join(tempDir, "prompts", "in-progress")
		promptsCompletedDir = filepath.Join(tempDir, "prompts", "completed")
		promptsRejectedDir = filepath.Join(tempDir, "prompts", "rejected")

		Expect(os.MkdirAll(specsInboxDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(specsInProgressDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(promptsInboxDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(promptsInProgressDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(promptsCompletedDir, 0750)).To(Succeed())

		specRejectCmd = cmd.NewSpecRejectCommand(
			specsInboxDir,
			specsInProgressDir,
			specsRejectedDir,
			promptsInboxDir,
			promptsInProgressDir,
			promptsCompletedDir,
			promptsRejectedDir,
			libtime.NewCurrentDateTime(),
		)
		ctx = context.Background()
	})

	Describe("Missing reason flag", func() {
		It("returns error when --reason is missing", func() {
			err := specRejectCmd.Run(ctx, []string{"some-spec.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--reason is required"))
		})
	})

	Describe("Spec not found", func() {
		It("returns error when spec does not exist", func() {
			err := specRejectCmd.Run(ctx, []string{"999-nonexistent.md", "--reason", "x"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec not found"))
		})
	})

	Describe("Reject spec from draft, no linked prompts", func() {
		It("moves draft spec from inbox to specs rejected with correct frontmatter", func() {
			specFile := filepath.Join(specsInboxDir, "001-my-spec.md")
			Expect(
				os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# My Spec"), 0600),
			).To(Succeed())

			err := specRejectCmd.Run(ctx, []string{"001-my-spec.md", "--reason", "not needed"})
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(specFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			dest := filepath.Join(specsRejectedDir, "001-my-spec.md")
			content, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: rejected"))
			Expect(string(content)).To(ContainSubstring("rejected_reason: not needed"))
			Expect(string(content)).To(ContainSubstring("rejected:"))
		})
	})

	Describe("Reject spec from approved, no linked prompts", func() {
		It("moves approved spec from in-progress to specs rejected", func() {
			specFile := filepath.Join(specsInProgressDir, "002-approved-spec.md")
			Expect(
				os.WriteFile(specFile, []byte("---\nstatus: approved\n---\n# Approved Spec"), 0600),
			).To(Succeed())

			err := specRejectCmd.Run(
				ctx,
				[]string{"002-approved-spec.md", "--reason", "changed direction"},
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(specFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			dest := filepath.Join(specsRejectedDir, "002-approved-spec.md")
			content, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: rejected"))
		})
	})

	Describe("Reject spec with cascade", func() {
		It("rejects spec and linked prompt, both moved to their rejected dirs", func() {
			specFile := filepath.Join(specsInboxDir, "003-cascade-spec.md")
			Expect(
				os.WriteFile(specFile, []byte("---\nstatus: approved\n---\n# Cascade Spec"), 0600),
			).To(Succeed())

			promptFile := filepath.Join(promptsInboxDir, "010-linked-prompt.md")
			Expect(
				os.WriteFile(
					promptFile,
					[]byte("---\nstatus: draft\nspec: [\"003\"]\n---\n# Linked Prompt"),
					0600,
				),
			).To(Succeed())

			err := specRejectCmd.Run(ctx, []string{"003-cascade-spec.md", "--reason", "cancelled"})
			Expect(err).NotTo(HaveOccurred())

			// Spec should be gone from inbox
			_, err = os.Stat(specFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			// Spec should be in specs rejected
			specDest := filepath.Join(specsRejectedDir, "003-cascade-spec.md")
			specContent, err := os.ReadFile(specDest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(specContent)).To(ContainSubstring("status: rejected"))
			Expect(string(specContent)).To(ContainSubstring("rejected_reason: cancelled"))

			// Prompt should be gone from inbox
			_, err = os.Stat(promptFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			// Prompt should be in prompts rejected
			promptDest := filepath.Join(promptsRejectedDir, "010-linked-prompt.md")
			promptContent, err := os.ReadFile(promptDest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(promptContent)).To(ContainSubstring("status: rejected"))
			Expect(string(promptContent)).To(ContainSubstring("rejected_reason: cancelled"))
		})
	})

	Describe("Preflight failure", func() {
		It("returns error naming the non-rejectable prompt, neither file is moved", func() {
			specFile := filepath.Join(specsInboxDir, "004-preflight-spec.md")
			Expect(
				os.WriteFile(
					specFile,
					[]byte("---\nstatus: approved\n---\n# Preflight Spec"),
					0600,
				),
			).To(Succeed())

			promptFile := filepath.Join(promptsInProgressDir, "020-executing-prompt.md")
			Expect(
				os.WriteFile(
					promptFile,
					[]byte("---\nstatus: executing\nspec: [\"004\"]\n---\n# Executing Prompt"),
					0600,
				),
			).To(Succeed())

			err := specRejectCmd.Run(ctx, []string{"004-preflight-spec.md", "--reason", "stop"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("020-executing-prompt.md"))
			Expect(err.Error()).To(ContainSubstring("executing"))

			// Neither file should have moved
			_, statErr := os.Stat(specFile)
			Expect(statErr).NotTo(HaveOccurred())

			_, statErr = os.Stat(promptFile)
			Expect(statErr).NotTo(HaveOccurred())
		})
	})

	Describe("Reject already-rejected spec", func() {
		It("returns error containing 'already rejected'", func() {
			specFile := filepath.Join(specsInboxDir, "005-already.md")
			Expect(
				os.WriteFile(specFile, []byte("---\nstatus: rejected\n---\n# Already"), 0600),
			).To(Succeed())

			err := specRejectCmd.Run(ctx, []string{"005-already.md", "--reason", "again"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already rejected"))
		})
	})

	Describe("Reject non-rejectable spec", func() {
		It("returns error containing 'cannot reject' for verifying status", func() {
			specFile := filepath.Join(specsInProgressDir, "006-verifying.md")
			Expect(
				os.WriteFile(specFile, []byte("---\nstatus: verifying\n---\n# Verifying"), 0600),
			).To(Succeed())

			err := specRejectCmd.Run(ctx, []string{"006-verifying.md", "--reason", "stop"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot reject"))
		})
	})

	Describe("Mid-cascade FS error", func() {
		It("stops cascade on first FS error, spec is NOT moved", func() {
			specFile := filepath.Join(specsInboxDir, "007-mid-cascade.md")
			Expect(
				os.WriteFile(specFile, []byte("---\nstatus: approved\n---\n# Mid Cascade"), 0600),
			).To(Succeed())

			// Two linked prompts both rejectable
			promptFile1 := filepath.Join(promptsInboxDir, "030-first.md")
			Expect(
				os.WriteFile(
					promptFile1,
					[]byte("---\nstatus: draft\nspec: [\"007\"]\n---\n# First"),
					0600,
				),
			).To(Succeed())

			promptFile2 := filepath.Join(promptsInboxDir, "031-second.md")
			Expect(
				os.WriteFile(
					promptFile2,
					[]byte("---\nstatus: draft\nspec: [\"007\"]\n---\n# Second"),
					0600,
				),
			).To(Succeed())

			// Block prompts/rejected by creating a file there (prevents MkdirAll)
			Expect(os.WriteFile(promptsRejectedDir, []byte("block"), 0600)).To(Succeed())

			err := specRejectCmd.Run(ctx, []string{"007-mid-cascade.md", "--reason", "cancel"})
			Expect(err).To(HaveOccurred())

			// Spec should NOT have moved (commit ordering: spec moves last)
			_, statErr := os.Stat(specFile)
			Expect(statErr).NotTo(HaveOccurred())
		})
	})

	Describe("No positional arg", func() {
		It("returns error with usage message", func() {
			err := specRejectCmd.Run(ctx, []string{"--reason", "something"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("usage: dark-factory spec reject"))
		})
	})
})
