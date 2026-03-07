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
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("PromptVerifyCommand", func() {
	var (
		tempDir           string
		queueDir          string
		completedDir      string
		mockPromptManager *mocks.Manager
		mockReleaser      *mocks.Releaser
		mockBrancher      *mocks.Brancher
		mockPRCreator     *mocks.PRCreator
		ctx               context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "verify-test-*")
		Expect(err).NotTo(HaveOccurred())

		queueDir = filepath.Join(tempDir, "in-progress")
		completedDir = filepath.Join(tempDir, "completed")

		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(completedDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		mockPromptManager = &mocks.Manager{}
		mockReleaser = &mocks.Releaser{}
		mockBrancher = &mocks.Brancher{}
		mockPRCreator = &mocks.PRCreator{}

		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	makeCmd := func(workflow config.Workflow) cmd.PromptVerifyCommand {
		return cmd.NewPromptVerifyCommand(
			queueDir,
			completedDir,
			mockPromptManager,
			mockReleaser,
			workflow,
			mockBrancher,
			mockPRCreator,
		)
	}

	Context("no args", func() {
		It("returns usage error", func() {
			err := makeCmd(config.WorkflowDirect).Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("usage: dark-factory prompt verify"))
		})
	})

	Context("prompt not found", func() {
		It("returns error", func() {
			err := makeCmd(config.WorkflowDirect).Run(ctx, []string{"999-nonexistent"})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("prompt in approved state", func() {
		It("returns not in pending verification error", func() {
			testFile := filepath.Join(queueDir, "080-test.md")
			err := os.WriteFile(testFile, []byte("---\nstatus: approved\n---\n# Test\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = makeCmd(config.WorkflowDirect).Run(ctx, []string{"080-test.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not in pending verification state"))
			Expect(err.Error()).To(ContainSubstring("approved"))
		})
	})

	Context("prompt in failed state", func() {
		It("returns not in pending verification error", func() {
			testFile := filepath.Join(queueDir, "080-test.md")
			err := os.WriteFile(testFile, []byte("---\nstatus: failed\n---\n# Test\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = makeCmd(config.WorkflowDirect).Run(ctx, []string{"080-test.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not in pending verification state"))
			Expect(err.Error()).To(ContainSubstring("failed"))
		})
	})

	Context("prompt in pending_verification state, workflow direct, no CHANGELOG", func() {
		It("calls MoveToCompleted, CommitCompletedFile, CommitOnly and prints verified", func() {
			testFile := filepath.Join(queueDir, "080-test.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: pending_verification\n---\n# My Test Prompt\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			mockPromptManager.MoveToCompletedReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(false)
			mockReleaser.CommitOnlyReturns(nil)

			err = makeCmd(config.WorkflowDirect).Run(ctx, []string{"080-test.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockPromptManager.MoveToCompletedCallCount()).To(Equal(1))
			Expect(mockReleaser.CommitCompletedFileCallCount()).To(Equal(1))
			Expect(mockReleaser.CommitOnlyCallCount()).To(Equal(1))
			Expect(mockReleaser.CommitAndReleaseCallCount()).To(Equal(0))
		})
	})

	Context("prompt in pending_verification state, workflow direct, CHANGELOG with feat", func() {
		It("calls CommitAndRelease with MinorBump", func() {
			testFile := filepath.Join(queueDir, "080-test.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: pending_verification\n---\n# My Test Prompt\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Create a CHANGELOG.md in tempDir and change to it
			changelogContent := "# Changelog\n\n## Unreleased\n\n- feat: add something new\n\n## v1.0.0\n\n- fix: old fix\n"
			origDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(
				filepath.Join(tempDir, "CHANGELOG.md"),
				[]byte(changelogContent),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Chdir(origDir) }()

			mockPromptManager.MoveToCompletedReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(true)
			mockReleaser.CommitAndReleaseReturns(nil)

			err = makeCmd(config.WorkflowDirect).Run(ctx, []string{"080-test.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockReleaser.CommitAndReleaseCallCount()).To(Equal(1))
			_, bump := mockReleaser.CommitAndReleaseArgsForCall(0)
			Expect(bump).To(Equal(git.MinorBump))
			Expect(mockReleaser.CommitOnlyCallCount()).To(Equal(0))
		})
	})

	Context("prompt in pending_verification state, workflow pr", func() {
		It("calls MoveToCompleted, CommitCompletedFile, CommitOnly, Push, Create", func() {
			testFile := filepath.Join(queueDir, "080-test.md")
			err := os.WriteFile(
				testFile,
				[]byte(
					"---\nstatus: pending_verification\nbranch: dark-factory/080-test\n---\n# My Test Prompt\n",
				),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			mockPromptManager.MoveToCompletedReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/owner/repo/pull/1", nil)

			err = makeCmd(config.WorkflowPR).Run(ctx, []string{"080-test.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockPromptManager.MoveToCompletedCallCount()).To(Equal(1))
			Expect(mockReleaser.CommitCompletedFileCallCount()).To(Equal(1))
			Expect(mockReleaser.CommitOnlyCallCount()).To(Equal(1))
			Expect(mockBrancher.PushCallCount()).To(Equal(1))
			Expect(mockPRCreator.CreateCallCount()).To(Equal(1))
		})
	})

	Context("prompt with no branch in frontmatter, workflow pr", func() {
		It("derives branch name from filename", func() {
			testFile := filepath.Join(queueDir, "080-my-feature.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: pending_verification\n---\n# My Feature\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			mockPromptManager.MoveToCompletedReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/owner/repo/pull/2", nil)

			err = makeCmd(config.WorkflowPR).Run(ctx, []string{"080-my-feature.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBrancher.PushCallCount()).To(Equal(1))
			_, pushedBranch := mockBrancher.PushArgsForCall(0)
			Expect(pushedBranch).To(Equal("dark-factory/080-my-feature"))
		})
	})
})
