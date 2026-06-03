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
	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("RejectCommand", func() {
	var (
		tempDir       string
		inboxDir      string
		inProgressDir string
		rejectedDir   string
		rejectCmd     cmd.RejectCommand
		ctx           context.Context
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()

		inboxDir = filepath.Join(tempDir, "prompts")
		inProgressDir = filepath.Join(tempDir, "prompts", "in-progress")
		rejectedDir = filepath.Join(tempDir, "prompts", "rejected")

		Expect(os.MkdirAll(inboxDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())

		rejectCmd = cmd.NewRejectCommand(
			inboxDir,
			inProgressDir,
			rejectedDir,
			prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime()),
		)
		ctx = context.Background()
	})

	Describe("Missing reason flag", func() {
		It("returns error when --reason is missing", func() {
			err := rejectCmd.Run(ctx, []string{"some-prompt.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--reason is required"))
		})

		It("returns error when --reason has no value", func() {
			err := rejectCmd.Run(ctx, []string{"some-prompt.md", "--reason"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--reason requires a value"))
		})
	})

	Describe("Prompt not found", func() {
		It("returns error when prompt does not exist", func() {
			err := rejectCmd.Run(ctx, []string{"999-missing.md", "--reason", "x"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prompt not found"))
		})
	})

	Describe("Reject from draft (inbox)", func() {
		It("moves draft prompt to rejected dir with correct frontmatter", func() {
			promptFile := filepath.Join(inboxDir, "001-fix-something.md")
			Expect(
				os.WriteFile(promptFile, []byte("---\nstatus: draft\n---\n# Fix Something"), 0600),
			).To(Succeed())

			err := rejectCmd.Run(ctx, []string{"001-fix-something.md", "--reason", "not needed"})
			Expect(err).NotTo(HaveOccurred())

			// Original file should be gone
			_, err = os.Stat(promptFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			// File should be in rejected dir
			dest := filepath.Join(rejectedDir, "001-fix-something.md")
			_, err = os.Stat(dest)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: rejected"))
			Expect(string(content)).To(ContainSubstring("rejected_reason: not needed"))
			Expect(string(content)).To(ContainSubstring("rejected:"))
		})
	})

	Describe("Reject from approved (in-progress)", func() {
		It("moves approved prompt from in-progress to rejected dir", func() {
			promptFile := filepath.Join(inProgressDir, "002-approved-task.md")
			Expect(
				os.WriteFile(
					promptFile,
					[]byte("---\nstatus: approved\n---\n# Approved Task"),
					0600,
				),
			).To(Succeed())

			err := rejectCmd.Run(
				ctx,
				[]string{"002-approved-task.md", "--reason", "changed direction"},
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(promptFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			dest := filepath.Join(rejectedDir, "002-approved-task.md")
			content, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: rejected"))
			Expect(string(content)).To(ContainSubstring("rejected_reason: changed direction"))
		})
	})

	Describe("Reject already-rejected item", func() {
		It("returns error containing 'already rejected'", func() {
			promptFile := filepath.Join(inboxDir, "003-already.md")
			Expect(
				os.WriteFile(promptFile, []byte("---\nstatus: rejected\n---\n# Already"), 0600),
			).To(Succeed())

			err := rejectCmd.Run(ctx, []string{"003-already.md", "--reason", "again"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already rejected"))
		})
	})

	Describe("Reject executing prompt", func() {
		It("returns error containing 'cannot reject'", func() {
			promptFile := filepath.Join(inProgressDir, "004-executing.md")
			Expect(
				os.WriteFile(promptFile, []byte("---\nstatus: executing\n---\n# Executing"), 0600),
			).To(Succeed())

			err := rejectCmd.Run(ctx, []string{"004-executing.md", "--reason", "stop"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot reject"))
		})
	})

	Describe("Reject completed prompt", func() {
		It("returns error containing 'cannot reject'", func() {
			promptFile := filepath.Join(inboxDir, "005-completed.md")
			Expect(
				os.WriteFile(promptFile, []byte("---\nstatus: completed\n---\n# Completed"), 0600),
			).To(Succeed())

			err := rejectCmd.Run(ctx, []string{"005-completed.md", "--reason", "too late"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot reject"))
		})
	})

	Describe("Reject idea prompt", func() {
		It("successfully rejects an idea prompt", func() {
			promptFile := filepath.Join(inboxDir, "006-idea.md")
			Expect(
				os.WriteFile(promptFile, []byte("---\nstatus: idea\n---\n# Idea"), 0600),
			).To(Succeed())

			err := rejectCmd.Run(ctx, []string{"006-idea.md", "--reason", "not useful"})
			Expect(err).NotTo(HaveOccurred())

			dest := filepath.Join(rejectedDir, "006-idea.md")
			content, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: rejected"))
		})
	})

	Describe("No positional arg", func() {
		It("returns error with usage message when only --reason is given", func() {
			err := rejectCmd.Run(ctx, []string{"--reason", "something"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("usage: dark-factory prompt reject"))
		})
	})

	Describe("Reject from failed state", func() {
		It(
			"moves failed prompt from in-progress to rejected and writes originalStatus: failed",
			func() {
				promptFile := filepath.Join(inProgressDir, "226-spec-056-foo-failed.md")
				Expect(os.WriteFile(
					promptFile,
					[]byte("---\nstatus: failed\n---\n# Failed prompt"),
					0600,
				)).To(Succeed())

				err := rejectCmd.Run(
					ctx,
					[]string{
						"226-spec-056-foo-failed.md",
						"--reason",
						"orphan from sibling worktree",
					},
				)
				Expect(err).NotTo(HaveOccurred())

				// Source gone
				_, err = os.Stat(promptFile)
				Expect(os.IsNotExist(err)).To(BeTrue())

				// File present in rejected/ — assert via typed frontmatter, not string-match
				dest := filepath.Join(rejectedDir, "226-spec-056-foo-failed.md")
				pm := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
				pf, err := pm.Load(ctx, dest)
				Expect(err).NotTo(HaveOccurred())
				Expect(pf.Frontmatter.Status).To(Equal("rejected"))
				Expect(pf.Frontmatter.OriginalStatus).To(Equal("failed"))
				Expect(pf.Frontmatter.RejectedReason).To(Equal("orphan from sibling worktree"))
			},
		)
	})

	Describe("Pre-execution reject leaves originalStatus empty", func() {
		It("rejects a draft prompt without writing originalStatus field", func() {
			promptFile := filepath.Join(inboxDir, "004-draft-pre.md")
			Expect(os.WriteFile(
				promptFile,
				[]byte("---\nstatus: draft\n---\n# Draft"),
				0600,
			)).To(Succeed())

			err := rejectCmd.Run(ctx, []string{"004-draft-pre.md", "--reason", "noop"})
			Expect(err).NotTo(HaveOccurred())

			dest := filepath.Join(rejectedDir, "004-draft-pre.md")
			pm := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
			pf, err := pm.Load(ctx, dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(pf.Frontmatter.Status).To(Equal("rejected"))
			Expect(pf.Frontmatter.RejectedReason).To(Equal("noop"))
			Expect(pf.Frontmatter.OriginalStatus).To(Equal(""))
		})
	})

	Describe("Hostile reason round-trip", func() {
		It("round-trips a hostile reason containing colon and newline through frontmatter", func() {
			promptFile := filepath.Join(inProgressDir, "226-hostile.md")
			Expect(os.WriteFile(
				promptFile,
				[]byte("---\nstatus: failed\n---\n# Hostile"),
				0600,
			)).To(Succeed())

			err := rejectCmd.Run(ctx, []string{"226-hostile.md", "--reason", "a: b\nc"})
			Expect(err).NotTo(HaveOccurred())

			// Read it back via a fresh PromptFile
			dest := filepath.Join(rejectedDir, "226-hostile.md")
			pm := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
			pf, err := pm.Load(ctx, dest)
			Expect(err).NotTo(HaveOccurred())

			Expect(pf.Frontmatter.RejectedReason).To(Equal("a: b\nc"))
			Expect(pf.Frontmatter.OriginalStatus).To(Equal("failed"))
			Expect(pf.Frontmatter.Status).To(Equal("rejected"))
		})
	})

	Describe("Re-run after partial move", func() {
		It("completes frontmatter rewrite on re-run after partial move to rejected/", func() {
			promptFile := filepath.Join(rejectedDir, "226-partial.md")
			// Simulate the partial-move state: file is in rejected/ but frontmatter still says failed
			Expect(os.MkdirAll(rejectedDir, 0750)).To(Succeed())
			Expect(os.WriteFile(
				promptFile,
				[]byte("---\nstatus: failed\n---\n# Partial"),
				0600,
			)).To(Succeed())

			err := rejectCmd.Run(ctx, []string{"226-partial.md", "--reason", "complete it"})
			Expect(err).NotTo(HaveOccurred())

			pm := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
			pf, err := pm.Load(ctx, promptFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(pf.Frontmatter.Status).To(Equal("rejected"))
			Expect(pf.Frontmatter.OriginalStatus).To(Equal("failed"))
			Expect(pf.Frontmatter.RejectedReason).To(Equal("complete it"))
		})

		It("returns 'already rejected' on a re-run after full rewrite", func() {
			promptFile := filepath.Join(rejectedDir, "226-done.md")
			Expect(os.MkdirAll(rejectedDir, 0750)).To(Succeed())
			Expect(os.WriteFile(
				promptFile,
				[]byte(
					"---\nstatus: rejected\noriginalStatus: failed\nrejected_reason: x\n---\n# Done",
				),
				0600,
			)).To(Succeed())

			err := rejectCmd.Run(ctx, []string{"226-done.md", "--reason", "again"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already rejected"))
		})
	})

	Describe("Reject idea does not write originalStatus", func() {
		It("rejects an idea prompt without writing originalStatus", func() {
			promptFile := filepath.Join(inboxDir, "005-idea.md")
			Expect(
				os.WriteFile(promptFile, []byte("---\nstatus: idea\n---\n# Idea"), 0600),
			).To(Succeed())

			err := rejectCmd.Run(ctx, []string{"005-idea.md", "--reason", "no"})
			Expect(err).NotTo(HaveOccurred())

			dest := filepath.Join(rejectedDir, "005-idea.md")
			pm := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
			pf, err := pm.Load(ctx, dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(pf.Frontmatter.Status).To(Equal("rejected"))
			Expect(pf.Frontmatter.OriginalStatus).To(Equal(""))
		})
	})
})
