// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("Prompt", func() {
	var (
		ctx     context.Context
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "prompt-test-*")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("ListQueued", func() {
		Context("with explicit status: queued", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "queued")
				createPromptFile(tempDir, "002-second.md", "queued")
			})

			It("returns prompts sorted alphabetically", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(2))
				Expect(filepath.Base(prompts[0].Path)).To(Equal("001-first.md"))
				Expect(filepath.Base(prompts[1].Path)).To(Equal("002-second.md"))
			})
		})

		Context("with no frontmatter at all", func() {
			BeforeEach(func() {
				// Plain markdown file with no frontmatter
				content := "# Test Prompt\n\nContent here.\n"
				path := filepath.Join(tempDir, "001-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("picks up the file", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(1))
				Expect(filepath.Base(prompts[0].Path)).To(Equal("001-plain.md"))
			})
		})

		Context("with frontmatter but no status field", func() {
			BeforeEach(func() {
				// Frontmatter with other fields but no status
				content := "---\nauthor: alice\n---\n\n# Test Prompt\n\nContent here.\n"
				path := filepath.Join(tempDir, "001-no-status.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("picks up the file", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(1))
				Expect(filepath.Base(prompts[0].Path)).To(Equal("001-no-status.md"))
			})
		})

		Context("with skip statuses", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-executing.md", "executing")
				createPromptFile(tempDir, "002-completed.md", "completed")
				createPromptFile(tempDir, "003-failed.md", "failed")
			})

			It("does not return files with skip status", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(0))
			})
		})

		Context("with mixed files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "queued")
				createPromptFile(tempDir, "002-completed.md", "completed")
				// Plain file with no frontmatter
				content := "# Plain Prompt\n\nContent here.\n"
				path := filepath.Join(tempDir, "003-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
				createPromptFile(tempDir, "004-executing.md", "executing")
			})

			It("returns queued and plain files only", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(2))
				Expect(filepath.Base(prompts[0].Path)).To(Equal("001-queued.md"))
				Expect(filepath.Base(prompts[1].Path)).To(Equal("003-plain.md"))
			})
		})

		Context("with non-markdown files", func() {
			BeforeEach(func() {
				// Create a non-markdown file
				err := os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("test"), 0600)
				Expect(err).To(BeNil())
				createPromptFile(tempDir, "001-first.md", "queued")
			})

			It("ignores non-markdown files", func() {
				prompts, err := prompt.ListQueued(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(1))
			})
		})
	})

	Describe("SetStatus", func() {
		Context("with existing frontmatter", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "queued")
			})

			It("updates status field", func() {
				err := prompt.SetStatus(ctx, path, "executing")
				Expect(err).To(BeNil())

				content, err := os.ReadFile(path)
				Expect(err).To(BeNil())
				Expect(string(content)).To(ContainSubstring("status: executing"))
			})
		})

		Context("without frontmatter", func() {
			var path string

			BeforeEach(func() {
				// Plain markdown file with no frontmatter
				content := "# Test Prompt\n\nContent here.\n"
				path = filepath.Join(tempDir, "001-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("adds frontmatter with status", func() {
				err := prompt.SetStatus(ctx, path, "executing")
				Expect(err).To(BeNil())

				content, err := os.ReadFile(path)
				Expect(err).To(BeNil())
				contentStr := string(content)
				Expect(contentStr).To(ContainSubstring("---\n"))
				Expect(contentStr).To(ContainSubstring("status: executing"))
				Expect(contentStr).To(ContainSubstring("# Test Prompt"))
			})
		})
	})

	Describe("SetContainer", func() {
		Context("with existing frontmatter", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "queued")
			})

			It("adds container field", func() {
				err := prompt.SetContainer(ctx, path, "dark-factory-001-test")
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Container).To(Equal("dark-factory-001-test"))
				Expect(fm.Status).To(Equal("queued")) // Status should be preserved
			})
		})

		Context("without frontmatter", func() {
			var path string

			BeforeEach(func() {
				// Plain markdown file with no frontmatter
				content := "# Test Prompt\n\nContent here.\n"
				path = filepath.Join(tempDir, "001-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("adds frontmatter with container field", func() {
				err := prompt.SetContainer(ctx, path, "dark-factory-001-plain")
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Container).To(Equal("dark-factory-001-plain"))
			})
		})

		Context("with existing container field", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued
container: old-container
---

# Test Prompt

Content here.
`
				path = filepath.Join(tempDir, "001-test.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("updates container field", func() {
				err := prompt.SetContainer(ctx, path, "new-container")
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Container).To(Equal("new-container"))
				Expect(fm.Status).To(Equal("queued")) // Status should be preserved
			})
		})
	})

	Describe("Title", func() {
		Context("with frontmatter", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued
---

# Implement Feature X

This is the content.
`
				path = filepath.Join(tempDir, "001-test.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("extracts first heading", func() {
				title, err := prompt.Title(ctx, path)
				Expect(err).To(BeNil())
				Expect(title).To(Equal("Implement Feature X"))
			})
		})

		Context("without frontmatter", func() {
			var path string

			BeforeEach(func() {
				content := `# Implement Feature Y

This is the content.
`
				path = filepath.Join(tempDir, "001-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("extracts first heading from start of file", func() {
				title, err := prompt.Title(ctx, path)
				Expect(err).To(BeNil())
				Expect(title).To(Equal("Implement Feature Y"))
			})
		})

		Context("without heading", func() {
			var path string

			BeforeEach(func() {
				content := "just some plain text without heading\n"
				path = filepath.Join(tempDir, "004-test.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("returns filename without extension", func() {
				title, err := prompt.Title(ctx, path)
				Expect(err).To(BeNil())
				Expect(title).To(Equal("004-test"))
			})
		})
	})

	Describe("Content", func() {
		Context("with content", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "queued")
			})

			It("returns content without frontmatter", func() {
				content, err := prompt.Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).NotTo(ContainSubstring("status: queued"))
				Expect(content).To(ContainSubstring("# Test Prompt"))
			})
		})

		Context("with empty file", func() {
			var path string

			BeforeEach(func() {
				path = filepath.Join(tempDir, "empty.md")
				err := os.WriteFile(path, []byte(""), 0600)
				Expect(err).To(BeNil())
			})

			It("returns ErrEmptyPrompt", func() {
				_, err := prompt.Content(ctx, path)
				Expect(err).To(Equal(prompt.ErrEmptyPrompt))
			})
		})

		Context("with whitespace-only file", func() {
			var path string

			BeforeEach(func() {
				path = filepath.Join(tempDir, "whitespace.md")
				err := os.WriteFile(path, []byte("   \n\t\n  \n"), 0600)
				Expect(err).To(BeNil())
			})

			It("returns ErrEmptyPrompt", func() {
				_, err := prompt.Content(ctx, path)
				Expect(err).To(Equal(prompt.ErrEmptyPrompt))
			})
		})
	})

	Describe("MoveToCompleted", func() {
		var path string

		BeforeEach(func() {
			path = createPromptFile(tempDir, "001-test.md", "queued")
		})

		It("moves file to completed subdirectory", func() {
			err := prompt.MoveToCompleted(ctx, path)
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
			err := prompt.MoveToCompleted(ctx, path)
			Expect(err).To(BeNil())

			// Read completed file and verify status
			completedPath := filepath.Join(tempDir, "completed", "001-test.md")
			fm, err := prompt.ReadFrontmatter(ctx, completedPath)
			Expect(err).To(BeNil())
			Expect(fm.Status).To(Equal("completed"))
		})
	})

	Describe("HasExecuting", func() {
		Context("with executing prompt", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "queued")
				createPromptFile(tempDir, "002-executing.md", "executing")
				createPromptFile(tempDir, "003-completed.md", "completed")
			})

			It("returns true", func() {
				result := prompt.HasExecuting(ctx, tempDir)
				Expect(result).To(BeTrue())
			})
		})

		Context("with multiple executing prompts", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-executing.md", "executing")
				createPromptFile(tempDir, "002-executing.md", "executing")
			})

			It("returns true", func() {
				result := prompt.HasExecuting(ctx, tempDir)
				Expect(result).To(BeTrue())
			})
		})

		Context("without executing prompt", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "queued")
				createPromptFile(tempDir, "002-completed.md", "completed")
				createPromptFile(tempDir, "003-failed.md", "failed")
			})

			It("returns false", func() {
				result := prompt.HasExecuting(ctx, tempDir)
				Expect(result).To(BeFalse())
			})
		})

		Context("with empty directory", func() {
			It("returns false", func() {
				result := prompt.HasExecuting(ctx, tempDir)
				Expect(result).To(BeFalse())
			})
		})

		Context("with non-markdown files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "queued")
				// Create a non-markdown file
				err := os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("test"), 0600)
				Expect(err).To(BeNil())
			})

			It("ignores non-markdown files and returns false", func() {
				result := prompt.HasExecuting(ctx, tempDir)
				Expect(result).To(BeFalse())
			})
		})

		Context("with invalid frontmatter", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "queued")
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
				result := prompt.HasExecuting(ctx, tempDir)
				Expect(result).To(BeFalse())
			})
		})

		Context("with nonexistent directory", func() {
			It("returns false", func() {
				result := prompt.HasExecuting(ctx, "/nonexistent/path")
				Expect(result).To(BeFalse())
			})
		})
	})

	Describe("ResetExecuting", func() {
		Context("with mixed statuses", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "queued")
				createPromptFile(tempDir, "002-executing.md", "executing")
				createPromptFile(tempDir, "003-completed.md", "completed")
				createPromptFile(tempDir, "004-executing.md", "executing")
				createPromptFile(tempDir, "005-failed.md", "failed")
			})

			It("resets only executing prompts to queued", func() {
				err := prompt.ResetExecuting(ctx, tempDir)
				Expect(err).To(BeNil())

				// Check that executing prompts are now queued
				fm, err := prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "002-executing.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("queued"))

				fm, err = prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "004-executing.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("queued"))

				// Check that other statuses are unchanged
				fm, err = prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "001-queued.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("queued"))

				fm, err = prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "003-completed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))

				fm, err = prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "005-failed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("failed"))
			})
		})

		Context("with no executing prompts", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "queued")
				createPromptFile(tempDir, "002-completed.md", "completed")
			})

			It("does nothing", func() {
				err := prompt.ResetExecuting(ctx, tempDir)
				Expect(err).To(BeNil())

				// Verify statuses are unchanged
				fm, err := prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "001-queued.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("queued"))

				fm, err = prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "002-completed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
			})
		})

		Context("with empty directory", func() {
			It("does nothing", func() {
				err := prompt.ResetExecuting(ctx, tempDir)
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("splitFrontmatter edge cases", func() {
		Context("with inline --- in content", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued
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
				fm, err := prompt.ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("queued"))

				content, err := prompt.Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("# Test Prompt"))
				Expect(content).To(ContainSubstring("This content has --- inline"))
				Expect(content).NotTo(ContainSubstring("status: queued"))
			})
		})

		Context("with --- at EOF without newline", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued
---`
				path = filepath.Join(tempDir, "002-eof.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("correctly parses frontmatter", func() {
				fm, err := prompt.ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("queued"))
			})

			It("returns empty content error", func() {
				_, err := prompt.Content(ctx, path)
				Expect(err).To(Equal(prompt.ErrEmptyPrompt))
			})
		})

		Context("with only opening --- and no closing", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued

# This is not valid frontmatter
Content here.
`
				path = filepath.Join(tempDir, "003-unclosed.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("treats entire file as content (no frontmatter)", func() {
				fm, err := prompt.ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("")) // No frontmatter parsed

				content, err := prompt.Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("---"))
				Expect(content).To(ContainSubstring("status: queued"))
			})
		})
	})

	Describe("NormalizeFilenames", func() {
		Context("with file missing numeric prefix", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "queued")
				createPromptFile(tempDir, "002-second.md", "queued")
				createPromptFile(tempDir, "fix-something.md", "queued")
			})

			It("assigns next available number", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
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
				createPromptFile(tempDir, "009-foo.md", "queued")
				createPromptFile(tempDir, "009-bar.md", "queued")
			})

			It("renames later file to next available number", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
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
				createPromptFile(tempDir, "9-foo.md", "queued")
			})

			It("normalizes to zero-padded 3-digit format", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
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
				createPromptFile(tempDir, "42-answer.md", "queued")
			})

			It("normalizes to zero-padded 3-digit format", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("42-answer.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("042-answer.md"))
			})
		})

		Context("with already-valid files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "queued")
				createPromptFile(tempDir, "002-second.md", "queued")
				createPromptFile(tempDir, "003-third.md", "queued")
			})

			It("does not rename any files", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
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
				createPromptFile(tempDir, "001-valid.md", "queued")
				createPromptFile(tempDir, "9-wrong-format.md", "queued")
				createPromptFile(tempDir, "no-number.md", "queued")
			})

			It("renames only invalid files", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
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
				createPromptFile(tempDir, "001-valid.md", "queued")
				createPromptFile(completedDir, "wrong-name.md", "completed")
			})

			It("does not rename files in subdirectories", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
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
				createPromptFile(tempDir, "new-feature.md", "queued")
			})

			It("assigns next number above completed/ maximum", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].OldPath)).To(Equal("new-feature.md"))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("004-new-feature.md"))
			})
		})

		Context("with non-markdown files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-valid.md", "queued")
				// Create non-markdown file without number
				err := os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("test"), 0600)
				Expect(err).To(BeNil())
			})

			It("ignores non-markdown files", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))

				// Verify non-markdown file is unchanged
				_, err = os.Stat(filepath.Join(tempDir, "readme.txt"))
				Expect(err).To(BeNil())
			})
		})

		Context("with empty directory", func() {
			It("returns no renames", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(0))
			})
		})

		Context("with gaps in numbering", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "queued")
				createPromptFile(tempDir, "005-fifth.md", "queued")
				createPromptFile(tempDir, "010-tenth.md", "queued")
				createPromptFile(tempDir, "new-file.md", "queued")
			})

			It("assigns smallest available number", func() {
				renames, err := prompt.NormalizeFilenames(ctx, tempDir)
				Expect(err).To(BeNil())
				Expect(renames).To(HaveLen(1))
				Expect(filepath.Base(renames[0].NewPath)).To(Equal("002-new-file.md"))
			})
		})
	})
})

// Helper function to create a prompt file with given status
func createPromptFile(dir, filename, status string) string {
	content := "---\n"
	if status != "" {
		content += "status: " + status + "\n"
	}
	content += "---\n\n# Test Prompt\n\nContent here.\n"

	path := filepath.Join(dir, filename)
	err := os.WriteFile(path, []byte(content), 0600)
	if err != nil {
		panic(err)
	}
	return path
}
