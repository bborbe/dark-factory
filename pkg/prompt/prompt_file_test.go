// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("Prompt", func() {
	var (
		ctx     context.Context
		tempDir string
		mover   prompt.FileMover
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "prompt-test-*")
		Expect(err).To(BeNil())
		mover = &simpleMover{}
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("MoveToCompleted", func() {
		var path string

		BeforeEach(func() {
			path = createPromptFile(tempDir, "001-test.md", "approved")
		})

		It("moves file to completed subdirectory", func() {
			completedDir := filepath.Join(tempDir, "completed")
			err := prompt.NewManager("", "", completedDir, mover, libtime.NewCurrentDateTime()).
				MoveToCompleted(ctx, path)
			Expect(err).To(BeNil())

			// Original file should not exist
			_, err = os.Stat(path)
			Expect(os.IsNotExist(err)).To(BeTrue())

			// File should exist in completed/
			completedPath := filepath.Join(tempDir, "completed", "001-test.md")
			_, err = os.Stat(completedPath)
			Expect(err).To(BeNil())
		})

		It("sets status to completed before moving", func() {
			completedDir := filepath.Join(tempDir, "completed")
			err := prompt.NewManager("", "", completedDir, mover, libtime.NewCurrentDateTime()).
				MoveToCompleted(ctx, path)
			Expect(err).To(BeNil())

			// Read completed file and verify status
			completedPath := filepath.Join(tempDir, "completed", "001-test.md")
			fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				ReadFrontmatter(ctx, completedPath)
			Expect(err).To(BeNil())
			Expect(fm.Status).To(Equal("completed"))
		})
	})

	Describe("HasExecuting", func() {
		Context("with executing prompt", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-executing.md", "executing")
				createPromptFile(tempDir, "003-completed.md", "completed")
			})

			It("returns true", func() {
				result := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					HasExecuting(ctx)
				Expect(result).To(BeTrue())
			})
		})

		Context("with multiple executing prompts", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-executing.md", "executing")
				createPromptFile(tempDir, "002-executing.md", "executing")
			})

			It("returns true", func() {
				result := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					HasExecuting(ctx)
				Expect(result).To(BeTrue())
			})
		})

		Context("without executing prompt", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-completed.md", "completed")
				createPromptFile(tempDir, "003-failed.md", "failed")
			})

			It("returns false", func() {
				result := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					HasExecuting(ctx)
				Expect(result).To(BeFalse())
			})
		})

		Context("with empty directory", func() {
			It("returns false", func() {
				result := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					HasExecuting(ctx)
				Expect(result).To(BeFalse())
			})
		})

		Context("with non-markdown files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				// Create a non-markdown file
				err := os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("test"), 0600)
				Expect(err).To(BeNil())
			})

			It("ignores non-markdown files and returns false", func() {
				result := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					HasExecuting(ctx)
				Expect(result).To(BeFalse())
			})
		})

		Context("with invalid frontmatter", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				// Create file with invalid frontmatter
				invalidContent := "---\ninvalid yaml content ][[\n---\n# Test\n"
				err := os.WriteFile(
					filepath.Join(tempDir, "002-invalid.md"),
					[]byte(invalidContent),
					0600,
				)
				Expect(err).To(BeNil())
			})

			It("skips files with errors and returns false", func() {
				result := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					HasExecuting(ctx)
				Expect(result).To(BeFalse())
			})
		})

		Context("with nonexistent directory", func() {
			It("returns false", func() {
				result := prompt.NewManager("", "/nonexistent/path", "", nil, libtime.NewCurrentDateTime()).
					HasExecuting(ctx)
				Expect(result).To(BeFalse())
			})
		})
	})

	Describe("ResetExecuting", func() {
		Context("with mixed statuses", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-executing.md", "executing")
				createPromptFile(tempDir, "003-completed.md", "completed")
				createPromptFile(tempDir, "004-executing.md", "executing")
				createPromptFile(tempDir, "005-failed.md", "failed")
			})

			It("resets only executing prompts to queued", func() {
				err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ResetExecuting(ctx)
				Expect(err).To(BeNil())

				// Check that executing prompts are now queued
				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "002-executing.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "004-executing.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				// Check that other statuses are unchanged
				fm, err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "001-queued.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "003-completed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))

				fm, err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "005-failed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("failed"))
			})
		})

		Context("with no executing prompts", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-completed.md", "completed")
			})

			It("does nothing", func() {
				err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ResetExecuting(ctx)
				Expect(err).To(BeNil())

				// Verify statuses are unchanged
				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "001-queued.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "002-completed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
			})
		})

		Context("with empty directory", func() {
			It("does nothing", func() {
				err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ResetExecuting(ctx)
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("ResetFailed", func() {
		Context("with mixed statuses", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-executing.md", "executing")
				createPromptFile(tempDir, "003-completed.md", "completed")
				createPromptFile(tempDir, "004-failed.md", "failed")
				createPromptFile(tempDir, "005-failed.md", "failed")
			})

			It("resets only failed prompts to queued", func() {
				err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ResetFailed(ctx)
				Expect(err).To(BeNil())

				// Check that failed prompts are now queued
				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "004-failed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "005-failed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				// Check that other statuses are unchanged
				fm, err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "001-queued.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "002-executing.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("executing"))

				fm, err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "003-completed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
			})
		})

		Context("with no failed prompts", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-completed.md", "completed")
			})

			It("does nothing", func() {
				err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ResetFailed(ctx)
				Expect(err).To(BeNil())

				// Verify statuses are unchanged
				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "001-queued.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				fm, err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, filepath.Join(tempDir, "002-completed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
			})
		})

		Context("with empty directory", func() {
			It("does nothing", func() {
				err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ResetFailed(ctx)
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("splitFrontmatter edge cases", func() {
		Context("with inline --- in content", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
---

# Test Prompt

This content has --- inline which should not be confused with frontmatter.

More content here.
`
				path = filepath.Join(tempDir, "001-inline.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("correctly extracts frontmatter and content", func() {
				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))

				content, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("# Test Prompt"))
				Expect(content).To(ContainSubstring("This content has --- inline"))
				Expect(content).NotTo(ContainSubstring("status: approved"))
			})
		})

		Context("with --- at EOF without newline", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
---`
				path = filepath.Join(tempDir, "002-eof.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("correctly parses frontmatter", func() {
				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("approved"))
			})

			It("returns empty content error", func() {
				_, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(Equal(prompt.ErrEmptyPrompt))
			})
		})

		Context("with in_review status in frontmatter", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: in_review
pr-url: https://github.com/example/repo/pull/42
---
# Some Prompt
Content here.
`
				path = filepath.Join(tempDir, "004-in-review.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("parses in_review status from frontmatter", func() {
				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal(string(prompt.InReviewPromptStatus)))
			})
		})

		Context("with only opening --- and no closing", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved

# This is not valid frontmatter
Content here.
`
				path = filepath.Join(tempDir, "003-unclosed.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("treats entire file as content (no frontmatter)", func() {
				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("")) // No frontmatter parsed

				content, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("---"))
				Expect(content).To(ContainSubstring("status: approved"))
			})
		})
	})

	Describe("setField does not accumulate blank lines", func() {
		It("preserves body without adding extra newlines across multiple calls", func() {
			path := filepath.Join(tempDir, "001-no-growth.md")
			original := "# My Prompt\n\nDo the thing.\n"
			err := os.WriteFile(path, []byte(original), 0600)
			Expect(err).To(BeNil())

			// Simulate full lifecycle: created, queued, container, version, executing, completed
			err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				SetStatus(ctx, path, "approved")
			Expect(err).To(BeNil())
			err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				SetContainer(ctx, path, "test-container")
			Expect(err).To(BeNil())
			err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				SetVersion(ctx, path, "v1.0.0")
			Expect(err).To(BeNil())
			err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				SetStatus(ctx, path, "executing")
			Expect(err).To(BeNil())
			err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				SetStatus(ctx, path, "completed")
			Expect(err).To(BeNil())

			// Read content — should have no leading blank lines
			content, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Content(ctx, path)
			Expect(err).To(BeNil())
			Expect(content).To(HavePrefix("# My Prompt"))
		})

		It("body size stays constant across 20 setField cycles", func() {
			path := filepath.Join(tempDir, "001-stable.md")
			original := "---\nstatus: approved\n---\n# Prompt\n\nContent here.\n"
			err := os.WriteFile(path, []byte(original), 0600)
			Expect(err).To(BeNil())

			// Read initial file size
			initialData, err := os.ReadFile(path)
			Expect(err).To(BeNil())
			initialSize := len(initialData)

			// Run 20 setField cycles (simulates retries)
			for i := 0; i < 20; i++ {
				err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetStatus(ctx, path, "executing")
				Expect(err).To(BeNil())
				err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetStatus(ctx, path, "approved")
				Expect(err).To(BeNil())
			}

			// File should not have grown significantly (only timestamp changes)
			finalData, err := os.ReadFile(path)
			Expect(err).To(BeNil())

			// Allow for timestamp field additions but not blank line growth
			// Timestamps add ~80 bytes max, blank line growth would add 20+ bytes per cycle
			Expect(len(finalData)).To(BeNumerically("<", initialSize+200))

			// Content should still start correctly
			content, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Content(ctx, path)
			Expect(err).To(BeNil())
			Expect(content).To(HavePrefix("# Prompt"))
		})
	})

	Describe("StripNumberPrefix", func() {
		DescribeTable("strips numeric prefixes",
			func(input string, expected string) {
				Expect(prompt.StripNumberPrefix(input)).To(Equal(expected))
			},
			Entry("3-digit prefix", "200-foo.md", "foo.md"),
			Entry("1-digit prefix", "1-bar.md", "bar.md"),
			Entry("no prefix", "foo.md", "foo.md"),
			Entry("valid 3-digit prefix", "001-baz.md", "baz.md"),
			Entry("large number", "9999-test.md", "test.md"),
		)
	})

	Describe("NormalizeFilenames", func() {
		Context("with file missing numeric prefix", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "approved")
				createPromptFile(tempDir, "002-second.md", "approved")
				createPromptFile(tempDir, "fix-something.md", "approved")
			})

			It("assigns next available number", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("fix-something.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("003-fix-something.md"))

				// Verify file was actually renamed
				_, err = os.Stat(filepath.Join(tempDir, "003-fix-something.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "fix-something.md"))
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("with duplicate number", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "009-foo.md", "approved")
				createPromptFile(tempDir, "009-bar.md", "approved")
			})

			It("renames later file to next available number", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				// First file alphabetically (009-bar.md) is kept, second (009-foo.md) is renamed
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("009-foo.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("001-foo.md"))

				// Verify files exist
				_, err = os.Stat(filepath.Join(tempDir, "009-bar.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "001-foo.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "009-foo.md"))
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("with wrong format (single digit)", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "9-foo.md", "approved")
			})

			It("normalizes to zero-padded 3-digit format", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("9-foo.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("009-foo.md"))

				// Verify file was renamed
				_, err = os.Stat(filepath.Join(tempDir, "009-foo.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "9-foo.md"))
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("with wrong format (two digits)", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "42-answer.md", "approved")
			})

			It("normalizes to zero-padded 3-digit format", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("42-answer.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("042-answer.md"))
			})
		})

		Context("with already-valid files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "approved")
				createPromptFile(tempDir, "002-second.md", "approved")
				createPromptFile(tempDir, "003-third.md", "approved")
			})

			It("does not rename any files", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))

				// Verify files still exist with same names
				_, err = os.Stat(filepath.Join(tempDir, "001-first.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "002-second.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "003-third.md"))
				Expect(err).To(BeNil())
			})
		})

		Context("with mixed valid and invalid files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-valid.md", "approved")
				createPromptFile(tempDir, "9-wrong-format.md", "approved")
				createPromptFile(tempDir, "no-number.md", "approved")
			})

			It("renames only invalid files", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(2))

				// Verify valid file is unchanged
				_, err = os.Stat(filepath.Join(tempDir, "001-valid.md"))
				Expect(err).To(BeNil())

				// Verify invalid files were renamed
				_, err = os.Stat(filepath.Join(tempDir, "009-wrong-format.md"))
				Expect(err).To(BeNil())
				_, err = os.Stat(filepath.Join(tempDir, "002-no-number.md"))
				Expect(err).To(BeNil())
			})
		})

		Context("with completed subdirectory", func() {
			BeforeEach(func() {
				// Create completed subdirectory
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())

				// Add files to both root and completed
				createPromptFile(tempDir, "001-valid.md", "approved")
				createPromptFile(completedDir, "wrong-name.md", "completed")
			})

			It("does not rename files in subdirectories", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))

				// Verify completed file is unchanged
				_, err = os.Stat(filepath.Join(tempDir, "completed", "wrong-name.md"))
				Expect(err).To(BeNil())
			})
		})

		Context("with numbers used in completed/ subdirectory", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())

				// completed/ has 001-003
				createPromptFile(completedDir, "001-done.md", "completed")
				createPromptFile(completedDir, "002-done.md", "completed")
				createPromptFile(completedDir, "003-done.md", "completed")

				// root has no numbered files, only an unnumbered one
				createPromptFile(tempDir, "new-feature.md", "approved")
			})

			It("assigns next number above completed/ maximum", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("new-feature.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("004-new-feature.md"))
			})
		})

		Context("with completed/ files lacking frontmatter", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())

				// completed/ has files without frontmatter
				path1 := filepath.Join(completedDir, "001-no-fm.md")
				err = os.WriteFile(path1, []byte("# Old prompt\n"), 0600)
				Expect(err).To(BeNil())
				// And one with frontmatter
				createPromptFile(completedDir, "002-with-fm.md", "completed")
				// And one with empty frontmatter
				path3 := filepath.Join(completedDir, "003-empty-fm.md")
				err = os.WriteFile(path3, []byte("---\n---\n# Empty\n"), 0600)
				Expect(err).To(BeNil())

				// root has unnumbered file
				createPromptFile(tempDir, "new-task.md", "approved")
			})

			It("scans completed/ without errors and avoids used numbers", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				// Should assign 004 (not 001-003 which are used in completed/)
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("new-task.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("004-new-task.md"))
			})
		})

		Context("with non-markdown files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-valid.md", "approved")
				// Create non-markdown file without number
				err := os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("test"), 0600)
				Expect(err).To(BeNil())
			})

			It("ignores non-markdown files", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))

				// Verify non-markdown file is unchanged
				_, err = os.Stat(filepath.Join(tempDir, "readme.txt"))
				Expect(err).To(BeNil())
			})
		})

		Context("with empty directory", func() {
			It("returns no renames", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))
			})
		})

		Context("with gaps in numbering", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "approved")
				createPromptFile(tempDir, "005-fifth.md", "approved")
				createPromptFile(tempDir, "010-tenth.md", "approved")
				createPromptFile(tempDir, "new-file.md", "approved")
			})

			It("assigns smallest available number", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("002-new-file.md"))
			})
		})

		Context("with wrong-format file conflicting with completed/ number", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())

				createPromptFile(completedDir, "001-core-pipeline.md", "completed")
				createPromptFile(tempDir, "01-foo.md", "approved")
			})

			It("renames to next available number instead of conflicting 001", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("01-foo.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("002-foo.md"))
			})
		})

		Context("with wrong-format file conflicting with many completed/ numbers", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())

				for i := 1; i <= 94; i++ {
					name := fmt.Sprintf("%03d-done.md", i)
					createPromptFile(completedDir, name, "completed")
				}
				createPromptFile(tempDir, "01-foo.md", "approved")
			})

			It("renames to first number above completed/ maximum", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("01-foo.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("095-foo.md"))
			})
		})

		Context("with wrong-format file and no completed files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "01-foo.md", "approved")
			})

			It("reformats to 3-digit prefix keeping same number", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("01-foo.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("001-foo.md"))
			})
		})

		Context("with already-correct 3-digit file in queue", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "095-foo.md", "approved")
			})

			It("does not rename the file", func() {
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))
			})
		})

		Context("inbox files are ignored for numbering", func() {
			It("does not reserve numbers from inbox files", func() {
				inboxDir := filepath.Join(tempDir, "inbox")
				err := os.MkdirAll(inboxDir, 0750)
				Expect(err).To(BeNil())

				// inbox has 001-003 — these should be ignored
				createPromptFile(inboxDir, "001-draft.md", "approved")
				createPromptFile(inboxDir, "002-draft.md", "approved")
				createPromptFile(inboxDir, "003-draft.md", "approved")

				// in-progress has one unnumbered file
				createPromptFile(tempDir, "new-feature.md", "approved")

				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NewManager("", "", completedDir, mover, nil).
					NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("new-feature.md"))
				// Should assign 001 — inbox numbers are not reserved
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("001-new-feature.md"))
			})
		})
	})

	Describe("PromptFile.SetPRURL", func() {
		It("sets the pr-url field in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: completed\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())

			pf.SetPRURL("https://github.com/user/repo/pull/42")
			Expect(pf.Frontmatter.PRURL).To(Equal("https://github.com/user/repo/pull/42"))
		})

		It("preserves pr-url after save and load roundtrip", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: completed\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			// Load, set PR URL, and save
			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			pf.SetPRURL("https://github.com/user/repo/pull/99")
			err = pf.Save(ctx)
			Expect(err).To(BeNil())

			// Load again and verify
			pf2, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			Expect(pf2.Frontmatter.PRURL).To(Equal("https://github.com/user/repo/pull/99"))
		})

		It("loads files without pr-url field", func() {
			path := filepath.Join(tempDir, "001-old-prompt.md")
			content := "---\nstatus: completed\nsummary: Old prompt\n---\n\n# Test\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			Expect(pf.Frontmatter.PRURL).To(Equal(""))
		})
	})

	Describe("SetPRURL", func() {
		It("sets pr-url field in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: completed\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				SetPRURL(ctx, path, "https://github.com/user/repo/pull/123")
			Expect(err).To(BeNil())

			// Verify the file was updated
			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			Expect(pf.Frontmatter.PRURL).To(Equal("https://github.com/user/repo/pull/123"))
		})

		It("adds frontmatter if file has none", func() {
			path := filepath.Join(tempDir, "001-no-frontmatter.md")
			content := "# Test Prompt\n\nContent here.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				SetPRURL(ctx, path, "https://github.com/user/repo/pull/1")
			Expect(err).To(BeNil())

			// Verify frontmatter was added with pr-url
			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			Expect(pf.Frontmatter.PRURL).To(Equal("https://github.com/user/repo/pull/1"))
		})
	})

	Describe("PromptFile.PRURL", func() {
		It("returns pr-url value when set in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: completed\npr-url: https://github.com/user/repo/pull/42\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			Expect(pf.PRURL()).To(Equal("https://github.com/user/repo/pull/42"))
		})

		It("returns empty string when pr-url is not set", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: completed\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			Expect(pf.PRURL()).To(Equal(""))
		})
	})

	Describe("PromptFile.MarkFailed", func() {
		It("sets status to failed with timestamp", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: executing\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())

			pf.MarkFailed()
			Expect(pf.Frontmatter.Status).To(Equal("failed"))
			Expect(pf.Frontmatter.Completed).NotTo(BeEmpty())
		})
	})

	Describe("PromptFile.SetBranch", func() {
		It("sets the branch field in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())

			pf.SetBranch("dark-factory/042-add-feature")
			Expect(pf.Frontmatter.Branch).To(Equal("dark-factory/042-add-feature"))
		})
	})

	Describe("PromptFile.Branch", func() {
		It("returns branch value when branch field is set in frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\nbranch: dark-factory/042-add-feature\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			Expect(pf.Branch()).To(Equal("dark-factory/042-add-feature"))
		})

		It("returns empty string when branch field is not set", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			Expect(pf.Branch()).To(Equal(""))
		})
	})

	Describe("SetBranch", func() {
		It("writes the branch value into frontmatter", func() {
			path := filepath.Join(tempDir, "001-test.md")
			content := "---\nstatus: approved\n---\n\n# Test\n\nContent.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				SetBranch(ctx, path, "dark-factory/042-add-feature")
			Expect(err).To(BeNil())

			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			Expect(pf.Branch()).To(Equal("dark-factory/042-add-feature"))
		})

		It("adds frontmatter if file has none", func() {
			path := filepath.Join(tempDir, "001-no-frontmatter.md")
			content := "# Test Prompt\n\nContent here.\n"
			err := os.WriteFile(path, []byte(content), 0600)
			Expect(err).To(BeNil())

			err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				SetBranch(ctx, path, "dark-factory/042-add-feature")
			Expect(err).To(BeNil())

			pf, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
				Load(ctx, path)
			Expect(err).To(BeNil())
			Expect(pf.Branch()).To(Equal("dark-factory/042-add-feature"))
		})
	})
})
