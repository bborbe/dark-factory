// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Internal git helpers", func() {
	Describe("processUnreleasedSection", func() {
		It("returns unchanged when no Unreleased section exists", func() {
			lines := []string{
				"# CHANGELOG",
				"",
				"## v1.0.0",
				"- Some change",
			}

			result, found := processUnreleasedSection(lines, "v1.1.0")
			Expect(found).To(BeFalse())
			Expect(result).To(Equal(lines))
		})

		It("renames Unreleased to version when found", func() {
			lines := []string{
				"# CHANGELOG",
				"",
				"## Unreleased",
				"- New feature",
				"- Bug fix",
			}

			result, found := processUnreleasedSection(lines, "v1.1.0")
			Expect(found).To(BeTrue())
			Expect(result).To(Equal([]string{
				"# CHANGELOG",
				"",
				"## v1.1.0",
				"- New feature",
				"- Bug fix",
			}))
		})

		It("only renames first Unreleased section", func() {
			lines := []string{
				"# CHANGELOG",
				"",
				"## Unreleased",
				"- New feature",
				"",
				"## v1.0.0",
				"- Old change",
			}

			result, found := processUnreleasedSection(lines, "v1.1.0")
			Expect(found).To(BeTrue())
			Expect(result).To(Equal([]string{
				"# CHANGELOG",
				"",
				"## v1.1.0",
				"- New feature",
				"",
				"## v1.0.0",
				"- Old change",
			}))
		})

		It("handles Unreleased at end of file", func() {
			lines := []string{
				"# CHANGELOG",
				"",
				"## Unreleased",
				"- Latest change",
			}

			result, found := processUnreleasedSection(lines, "v2.0.0")
			Expect(found).To(BeTrue())
			Expect(result[2]).To(Equal("## v2.0.0"))
		})

		It("preserves all content after Unreleased", func() {
			lines := []string{
				"# CHANGELOG",
				"## Unreleased",
				"- Change 1",
				"- Change 2",
				"",
				"## v1.0.0",
				"- Old change",
				"",
				"## v0.9.0",
				"- Very old change",
			}

			result, found := processUnreleasedSection(lines, "v1.1.0")
			Expect(found).To(BeTrue())
			Expect(len(result)).To(Equal(len(lines)))
			Expect(result[1]).To(Equal("## v1.1.0"))
			Expect(result[5]).To(Equal("## v1.0.0"))
			Expect(result[8]).To(Equal("## v0.9.0"))
		})

		It("handles empty Unreleased section", func() {
			lines := []string{
				"# CHANGELOG",
				"",
				"## Unreleased",
				"",
				"## v1.0.0",
				"- Some change",
			}

			result, found := processUnreleasedSection(lines, "v1.1.0")
			Expect(found).To(BeTrue())
			Expect(result[2]).To(Equal("## v1.1.0"))
		})
	})

	Describe("CommitAndRelease", func() {
		It("returns error outside a git repo", func() {
			ctx := context.Background()
			tmpDir, err := os.MkdirTemp("", "commitandrelease-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			origDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Chdir(origDir) }()

			err = os.Chdir(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			err = CommitAndRelease(ctx, PatchBump)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("gitCommit", func() {
		It("returns error outside a git repo", func() {
			ctx := context.Background()
			tmpDir, err := os.MkdirTemp("", "gitcommit-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			origDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Chdir(origDir) }()

			err = os.Chdir(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			err = gitCommit(ctx, "test commit")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CommitCompletedFile", func() {
		It("returns error outside a git repo", func() {
			ctx := context.Background()
			tmpDir, err := os.MkdirTemp("", "commitcompleted-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			origDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Chdir(origDir) }()

			err = os.Chdir(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			err = CommitCompletedFile(ctx, filepath.Join(tmpDir, "somefile.md"))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("gitAddAll", func() {
		It("returns error outside a git repo", func() {
			ctx := context.Background()
			tmpDir, err := os.MkdirTemp("", "gitaddall-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			origDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Chdir(origDir) }()

			err = os.Chdir(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			err = gitAddAll(ctx)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("gitTag", func() {
		It("returns error for invalid tag format", func() {
			ctx := context.Background()
			err := gitTag(ctx, "not-a-semver")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("gitPushTag", func() {
		It("returns error for invalid tag format", func() {
			ctx := context.Background()
			err := gitPushTag(ctx, "not-a-semver")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("MoveFile", func() {
		var (
			ctx     context.Context
			tempDir string
		)

		BeforeEach(func() {
			ctx = context.Background()
			var err error
			tempDir, err = os.MkdirTemp("", "git-movefile-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		Context("non-git directory", func() {
			It("falls back to os.Rename", func() {
				oldPath := filepath.Join(tempDir, "old.txt")
				newPath := filepath.Join(tempDir, "new.txt")

				err := os.WriteFile(oldPath, []byte("test content"), 0600)
				Expect(err).NotTo(HaveOccurred())

				err = MoveFile(ctx, oldPath, newPath)
				Expect(err).NotTo(HaveOccurred())

				// Old file should not exist
				_, err = os.Stat(oldPath)
				Expect(os.IsNotExist(err)).To(BeTrue())

				// New file should exist with same content
				content, err := os.ReadFile(newPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal("test content"))
			})

			It("returns error when source file does not exist", func() {
				oldPath := filepath.Join(tempDir, "nonexistent.txt")
				newPath := filepath.Join(tempDir, "new.txt")

				err := MoveFile(ctx, oldPath, newPath)
				Expect(err).To(HaveOccurred())
			})

			It("handles subdirectories", func() {
				subdir := filepath.Join(tempDir, "subdir")
				err := os.MkdirAll(subdir, 0750)
				Expect(err).NotTo(HaveOccurred())

				oldPath := filepath.Join(tempDir, "file.txt")
				newPath := filepath.Join(subdir, "file.txt")

				err = os.WriteFile(oldPath, []byte("content"), 0600)
				Expect(err).NotTo(HaveOccurred())

				err = MoveFile(ctx, oldPath, newPath)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(newPath)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
