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
