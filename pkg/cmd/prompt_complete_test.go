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
	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("PromptCompleteCommand", func() {
	var (
		tempDir       string
		queueDir      string
		completedDir  string
		promptManager *mocks.Manager
		releaser      *mocks.Releaser
		brancher      *mocks.Brancher
		prCreator     *mocks.PRCreator
		ctx           context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "complete-test-*")
		Expect(err).NotTo(HaveOccurred())

		queueDir = filepath.Join(tempDir, "in-progress")
		completedDir = filepath.Join(tempDir, "completed")

		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(completedDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		promptManager = &mocks.Manager{}
		releaser = &mocks.Releaser{}
		brancher = &mocks.Brancher{}
		prCreator = &mocks.PRCreator{}

		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	makeCmd := func(pr bool) cmd.PromptCompleteCommand {
		return cmd.NewPromptCompleteCommand(
			queueDir,
			completedDir,
			promptManager,
			releaser,
			pr,
			brancher,
			prCreator,
			libtime.NewCurrentDateTime(),
		)
	}

	Context("no args", func() {
		It("returns usage error", func() {
			err := makeCmd(false).Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("usage: dark-factory prompt complete"))
		})
	})

	Context("prompt not found", func() {
		It("returns error", func() {
			err := makeCmd(false).Run(ctx, []string{"999-nonexistent"})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("prompt in approved state", func() {
		It("returns cannot be completed error", func() {
			testFile := filepath.Join(queueDir, "080-test.md")
			err := os.WriteFile(testFile, []byte("---\nstatus: approved\n---\n# Test\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = makeCmd(false).Run(ctx, []string{"080-test.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prompt cannot be completed"))
			Expect(err.Error()).To(ContainSubstring("approved"))
		})
	})

	Context("prompt in failed state", func() {
		It("succeeds and calls MoveToCompleted, CommitCompletedFile, CommitOnly", func() {
			testFile := filepath.Join(queueDir, "080-test.md")
			err := os.WriteFile(testFile, []byte("---\nstatus: failed\n---\n# Test\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			promptManager.MoveToCompletedReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			err = makeCmd(false).Run(ctx, []string{"080-test.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(promptManager.MoveToCompletedCallCount()).To(Equal(1))
			Expect(releaser.CommitCompletedFileCallCount()).To(Equal(1))
			Expect(releaser.CommitOnlyCallCount()).To(Equal(1))
		})
	})

	Context("prompt in executing state", func() {
		It("succeeds and calls MoveToCompleted, CommitCompletedFile, CommitOnly", func() {
			testFile := filepath.Join(queueDir, "080-test.md")
			err := os.WriteFile(testFile, []byte("---\nstatus: executing\n---\n# Test\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			promptManager.MoveToCompletedReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			err = makeCmd(false).Run(ctx, []string{"080-test.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(promptManager.MoveToCompletedCallCount()).To(Equal(1))
			Expect(releaser.CommitCompletedFileCallCount()).To(Equal(1))
			Expect(releaser.CommitOnlyCallCount()).To(Equal(1))
		})
	})

	Context("prompt in in_review state", func() {
		It("succeeds and calls MoveToCompleted, CommitCompletedFile, CommitOnly", func() {
			testFile := filepath.Join(queueDir, "080-test.md")
			err := os.WriteFile(testFile, []byte("---\nstatus: in_review\n---\n# Test\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			promptManager.MoveToCompletedReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			err = makeCmd(false).Run(ctx, []string{"080-test.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(promptManager.MoveToCompletedCallCount()).To(Equal(1))
			Expect(releaser.CommitCompletedFileCallCount()).To(Equal(1))
			Expect(releaser.CommitOnlyCallCount()).To(Equal(1))
		})
	})

	Context("prompt in pending_verification state, workflow direct, no CHANGELOG", func() {
		It("calls MoveToCompleted, CommitCompletedFile, CommitOnly and prints completed", func() {
			testFile := filepath.Join(queueDir, "080-test.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: pending_verification\n---\n# My Test Prompt\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			promptManager.MoveToCompletedReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			err = makeCmd(false).Run(ctx, []string{"080-test.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(promptManager.MoveToCompletedCallCount()).To(Equal(1))
			Expect(releaser.CommitCompletedFileCallCount()).To(Equal(1))
			Expect(releaser.CommitOnlyCallCount()).To(Equal(1))
			Expect(releaser.CommitAndReleaseCallCount()).To(Equal(0))
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

			promptManager.MoveToCompletedReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(true)
			releaser.CommitAndReleaseReturns(nil)

			err = makeCmd(false).Run(ctx, []string{"080-test.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(releaser.CommitAndReleaseCallCount()).To(Equal(1))
			_, bump := releaser.CommitAndReleaseArgsForCall(0)
			Expect(bump).To(Equal(git.MinorBump))
			Expect(releaser.CommitOnlyCallCount()).To(Equal(0))
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

			promptManager.MoveToCompletedReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/owner/repo/pull/1", nil)

			err = makeCmd(true).Run(ctx, []string{"080-test.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(promptManager.MoveToCompletedCallCount()).To(Equal(1))
			Expect(releaser.CommitCompletedFileCallCount()).To(Equal(1))
			Expect(releaser.CommitOnlyCallCount()).To(Equal(1))
			Expect(brancher.PushCallCount()).To(Equal(1))
			Expect(prCreator.CreateCallCount()).To(Equal(1))
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

			promptManager.MoveToCompletedReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/owner/repo/pull/2", nil)

			err = makeCmd(true).Run(ctx, []string{"080-my-feature.md"})
			Expect(err).NotTo(HaveOccurred())

			Expect(brancher.PushCallCount()).To(Equal(1))
			_, pushedBranch := brancher.PushArgsForCall(0)
			Expect(pushedBranch).To(Equal("dark-factory/080-my-feature"))
		})
	})
})
