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

// simpleMover is a test implementation that uses os.Rename
type simpleMover struct{}

func (s *simpleMover) MoveFile(ctx context.Context, oldPath string, newPath string) error {
	return os.Rename(oldPath, newPath)
}

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

	Describe("Status.Validate", func() {
		Context("with valid statuses", func() {
			It("accepts queued", func() {
				err := prompt.StatusQueued.Validate(ctx)
				Expect(err).To(BeNil())
			})

			It("accepts executing", func() {
				err := prompt.StatusExecuting.Validate(ctx)
				Expect(err).To(BeNil())
			})

			It("accepts completed", func() {
				err := prompt.StatusCompleted.Validate(ctx)
				Expect(err).To(BeNil())
			})

			It("accepts failed", func() {
				err := prompt.StatusFailed.Validate(ctx)
				Expect(err).To(BeNil())
			})
		})

		Context("with invalid status", func() {
			It("rejects unknown status", func() {
				err := prompt.Status("invalid").Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("status(invalid) is invalid"))
			})

			It("rejects empty status", func() {
				err := prompt.Status("").Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("status() is invalid"))
			})
		})
	})

	Describe("Prompt.Validate", func() {
		Context("with valid prompt", func() {
			It("accepts valid prompt with numbered filename", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.StatusQueued,
				}
				err := p.Validate(ctx)
				Expect(err).To(BeNil())
			})
		})

		Context("with invalid prompts", func() {
			It("rejects empty path", func() {
				p := prompt.Prompt{
					Path:   "",
					Status: prompt.StatusQueued,
				}
				err := p.Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("path"))
			})

			It("rejects invalid status", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.Status("invalid"),
				}
				err := p.Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("status"))
			})

			It("rejects filename without number prefix", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "test.md"),
					Status: prompt.StatusQueued,
				}
				err := p.Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("missing NNN- prefix"))
			})

			It("rejects filename with wrong format (single digit)", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "1-test.md"),
					Status: prompt.StatusQueued,
				}
				err := p.Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("missing NNN- prefix"))
			})

			It("rejects filename with wrong format (two digits)", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "42-test.md"),
					Status: prompt.StatusQueued,
				}
				err := p.Validate(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("missing NNN- prefix"))
			})
		})
	})

	Describe("Prompt.ValidateForExecution", func() {
		Context("with queued status", func() {
			It("accepts prompt ready for execution", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.StatusQueued,
				}
				err := p.ValidateForExecution(ctx)
				Expect(err).To(BeNil())
			})
		})

		Context("with non-queued status", func() {
			It("rejects executing prompt", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.StatusExecuting,
				}
				err := p.ValidateForExecution(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("expected status queued"))
			})

			It("rejects completed prompt", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.StatusCompleted,
				}
				err := p.ValidateForExecution(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("expected status queued"))
			})

			It("rejects failed prompt", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "001-test.md"),
					Status: prompt.StatusFailed,
				}
				err := p.ValidateForExecution(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("expected status queued"))
			})
		})

		Context("with invalid prompt", func() {
			It("rejects prompt without number prefix", func() {
				p := prompt.Prompt{
					Path:   filepath.Join(tempDir, "test.md"),
					Status: prompt.StatusQueued,
				}
				err := p.ValidateForExecution(ctx)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("missing NNN- prefix"))
			})
		})
	})

	Describe("Prompt.Number", func() {
		It("extracts number from valid filename", func() {
			p := prompt.Prompt{
				Path: filepath.Join(tempDir, "001-test.md"),
			}
			Expect(p.Number()).To(Equal(1))
		})

		It("extracts larger number", func() {
			p := prompt.Prompt{
				Path: filepath.Join(tempDir, "042-test.md"),
			}
			Expect(p.Number()).To(Equal(42))
		})

		It("returns -1 for filename without number", func() {
			p := prompt.Prompt{
				Path: filepath.Join(tempDir, "test.md"),
			}
			Expect(p.Number()).To(Equal(-1))
		})

		It("returns -1 for invalid number format", func() {
			p := prompt.Prompt{
				Path: filepath.Join(tempDir, "1-test.md"),
			}
			Expect(p.Number()).To(Equal(-1))
		})
	})

	Describe("AllPreviousCompleted", func() {
		Context("with no previous prompts", func() {
			It("returns true for n=1", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 1)
				Expect(result).To(BeTrue())
			})
		})

		Context("with all previous prompts completed", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
				createPromptFile(completedDir, "001-first.md", "completed")
				createPromptFile(completedDir, "002-second.md", "completed")
				createPromptFile(completedDir, "003-third.md", "completed")
			})

			It("returns true for n=4", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 4)
				Expect(result).To(BeTrue())
			})

			It("returns true for n=2", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 2)
				Expect(result).To(BeTrue())
			})
		})

		Context("with gap in completed prompts", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
				createPromptFile(completedDir, "001-first.md", "completed")
				createPromptFile(completedDir, "003-third.md", "completed")
				// Missing 002
			})

			It("returns false for n=3", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 3)
				Expect(result).To(BeFalse())
			})

			It("returns false for n=4", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 4)
				Expect(result).To(BeFalse())
			})

			It("returns true for n=2", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 2)
				Expect(result).To(BeTrue())
			})
		})

		Context("with no completed directory", func() {
			It("returns false for n=2", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 2)
				Expect(result).To(BeFalse())
			})

			It("returns true for n=1", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 1)
				Expect(result).To(BeTrue())
			})
		})

		Context("with empty completed directory", func() {
			BeforeEach(func() {
				completedDir := filepath.Join(tempDir, "completed")
				err := os.MkdirAll(completedDir, 0750)
				Expect(err).To(BeNil())
			})

			It("returns false for n=2", func() {
				result := prompt.AllPreviousCompleted(ctx, filepath.Join(tempDir, "completed"), 2)
				Expect(result).To(BeFalse())
			})
		})
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

	Describe("SetVersion", func() {
		Context("with existing frontmatter", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "queued")
			})

			It("adds version field", func() {
				err := prompt.SetVersion(ctx, path, "v0.2.37")
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.DarkFactoryVersion).To(Equal("v0.2.37"))
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

			It("adds frontmatter with version field", func() {
				err := prompt.SetVersion(ctx, path, "v0.1.0")
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.DarkFactoryVersion).To(Equal("v0.1.0"))
			})
		})

		Context("with existing version field", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued
dark-factory-version: v0.1.0
---

# Test Prompt

Content here.
`
				path = filepath.Join(tempDir, "001-test.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("updates version field", func() {
				err := prompt.SetVersion(ctx, path, "v0.2.0")
				Expect(err).To(BeNil())

				fm, err := prompt.ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.DarkFactoryVersion).To(Equal("v0.2.0"))
				Expect(fm.Status).To(Equal("queued")) // Status should be preserved
			})
		})

		Context("version persists through move to completed", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "queued")
			})

			It("preserves version when moved to completed", func() {
				// Set container and version
				err := prompt.SetContainer(ctx, path, "dark-factory-001-test")
				Expect(err).To(BeNil())

				err = prompt.SetVersion(ctx, path, "v0.5.0")
				Expect(err).To(BeNil())

				// Move to completed
				completedDir := filepath.Join(tempDir, "completed")
				err = prompt.MoveToCompleted(ctx, path, completedDir, mover)
				Expect(err).To(BeNil())

				// Verify version is preserved in completed file
				completedPath := filepath.Join(tempDir, "completed", "001-test.md")
				fm, err := prompt.ReadFrontmatter(ctx, completedPath)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
				Expect(fm.Container).To(Equal("dark-factory-001-test"))
				Expect(fm.DarkFactoryVersion).To(Equal("v0.5.0"))
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

		Context("with duplicate empty frontmatter", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued
---



---
---

# Actual prompt title

Prompt content here.
`
				path = filepath.Join(tempDir, "001-duplicate.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("strips the empty frontmatter block from content", func() {
				content, err := prompt.Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).NotTo(ContainSubstring("status: queued"))
				Expect(content).NotTo(HavePrefix("---"))
				Expect(content).To(ContainSubstring("# Actual prompt title"))
				Expect(content).To(ContainSubstring("Prompt content here."))
			})
		})

		Context("with empty frontmatter block containing only whitespace", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued
---

---


---

# Test Prompt

Content here.
`
				path = filepath.Join(tempDir, "001-whitespace-fm.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("strips the whitespace-only frontmatter block", func() {
				content, err := prompt.Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).NotTo(ContainSubstring("status: queued"))
				Expect(content).NotTo(HavePrefix("---"))
				Expect(content).To(ContainSubstring("# Test Prompt"))
			})
		})

		Context("with multiple consecutive empty frontmatter blocks", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued
---

---
---

---
---

# Test Prompt

Content here.
`
				path = filepath.Join(tempDir, "001-multiple.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("strips all empty frontmatter blocks", func() {
				content, err := prompt.Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).NotTo(ContainSubstring("status: queued"))
				Expect(content).NotTo(HavePrefix("---"))
				Expect(content).To(ContainSubstring("# Test Prompt"))
			})
		})

		Context("with non-empty duplicate frontmatter", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued
---

---
title: Some Title
---

# Test Prompt

Content here.
`
				path = filepath.Join(tempDir, "001-nonempty.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("preserves non-empty frontmatter in content", func() {
				content, err := prompt.Content(ctx, path)
				Expect(err).To(BeNil())
				Expect(content).NotTo(ContainSubstring("status: queued"))
				Expect(content).To(ContainSubstring("---"))
				Expect(content).To(ContainSubstring("title: Some Title"))
				Expect(content).To(ContainSubstring("# Test Prompt"))
			})
		})

		Context("with empty frontmatter at EOF", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: queued
---

---
---`
				path = filepath.Join(tempDir, "001-eof.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
			})

			It("strips empty frontmatter and returns empty prompt error", func() {
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
			completedDir := filepath.Join(tempDir, "completed")
			err := prompt.MoveToCompleted(ctx, path, completedDir, mover)
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
			err := prompt.MoveToCompleted(ctx, path, completedDir, mover)
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

	Describe("ResetFailed", func() {
		Context("with mixed statuses", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "queued")
				createPromptFile(tempDir, "002-executing.md", "executing")
				createPromptFile(tempDir, "003-completed.md", "completed")
				createPromptFile(tempDir, "004-failed.md", "failed")
				createPromptFile(tempDir, "005-failed.md", "failed")
			})

			It("resets only failed prompts to queued", func() {
				err := prompt.ResetFailed(ctx, tempDir)
				Expect(err).To(BeNil())

				// Check that failed prompts are now queued
				fm, err := prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "004-failed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("queued"))

				fm, err = prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "005-failed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("queued"))

				// Check that other statuses are unchanged
				fm, err = prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "001-queued.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("queued"))

				fm, err = prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "002-executing.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("executing"))

				fm, err = prompt.ReadFrontmatter(ctx, filepath.Join(tempDir, "003-completed.md"))
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
			})
		})

		Context("with no failed prompts", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "queued")
				createPromptFile(tempDir, "002-completed.md", "completed")
			})

			It("does nothing", func() {
				err := prompt.ResetFailed(ctx, tempDir)
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
				err := prompt.ResetFailed(ctx, tempDir)
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
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
				completedDir := filepath.Join(tempDir, "completed")
				renames, err := prompt.NormalizeFilenames(ctx, tempDir, completedDir, mover)
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
