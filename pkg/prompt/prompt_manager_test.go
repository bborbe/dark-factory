// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt_test

import (
	"context"
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

	Describe("ListQueued", func() {
		Context("with explicit status: approved", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-first.md", "approved")
				createPromptFile(tempDir, "002-second.md", "approved")
			})

			It("returns prompts sorted alphabetically", func() {
				prompts, err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ListQueued(ctx)
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
				prompts, err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ListQueued(ctx)
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
				prompts, err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ListQueued(ctx)
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
				prompts, err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ListQueued(ctx)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(0))
			})
		})

		Context("with mixed files", func() {
			BeforeEach(func() {
				createPromptFile(tempDir, "001-queued.md", "approved")
				createPromptFile(tempDir, "002-completed.md", "completed")
				// Plain file with no frontmatter
				content := "# Plain Prompt\n\nContent here.\n"
				path := filepath.Join(tempDir, "003-plain.md")
				err := os.WriteFile(path, []byte(content), 0600)
				Expect(err).To(BeNil())
				createPromptFile(tempDir, "004-executing.md", "executing")
			})

			It("returns queued and plain files only", func() {
				prompts, err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ListQueued(ctx)
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
				createPromptFile(tempDir, "001-first.md", "approved")
			})

			It("ignores non-markdown files", func() {
				prompts, err := prompt.NewManager("", tempDir, "", nil, libtime.NewCurrentDateTime()).
					ListQueued(ctx)
				Expect(err).To(BeNil())
				Expect(prompts).To(HaveLen(1))
			})
		})
	})

	Describe("SetStatus", func() {
		Context("with existing frontmatter", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "approved")
			})

			It("updates status field", func() {
				err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetStatus(ctx, path, "executing")
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
				err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetStatus(ctx, path, "executing")
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
				path = createPromptFile(tempDir, "001-test.md", "approved")
			})

			It("adds container field", func() {
				err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetContainer(ctx, path, "dark-factory-001-test")
				Expect(err).To(BeNil())

				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Container).To(Equal("dark-factory-001-test"))
				Expect(fm.Status).To(Equal("approved")) // Status should be preserved
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
				err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetContainer(ctx, path, "dark-factory-001-plain")
				Expect(err).To(BeNil())

				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Container).To(Equal("dark-factory-001-plain"))
			})
		})

		Context("with existing container field", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
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
				err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetContainer(ctx, path, "new-container")
				Expect(err).To(BeNil())

				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.Container).To(Equal("new-container"))
				Expect(fm.Status).To(Equal("approved")) // Status should be preserved
			})
		})
	})

	Describe("SetVersion", func() {
		Context("with existing frontmatter", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "approved")
			})

			It("adds version field", func() {
				err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetVersion(ctx, path, "v0.2.37")
				Expect(err).To(BeNil())

				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.DarkFactoryVersion).To(Equal("v0.2.37"))
				Expect(fm.Status).To(Equal("approved")) // Status should be preserved
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
				err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetVersion(ctx, path, "v0.1.0")
				Expect(err).To(BeNil())

				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.DarkFactoryVersion).To(Equal("v0.1.0"))
			})
		})

		Context("with existing version field", func() {
			var path string

			BeforeEach(func() {
				content := `---
status: approved
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
				err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetVersion(ctx, path, "v0.2.0")
				Expect(err).To(BeNil())

				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, path)
				Expect(err).To(BeNil())
				Expect(fm.DarkFactoryVersion).To(Equal("v0.2.0"))
				Expect(fm.Status).To(Equal("approved")) // Status should be preserved
			})
		})

		Context("version persists through move to completed", func() {
			var path string

			BeforeEach(func() {
				path = createPromptFile(tempDir, "001-test.md", "approved")
			})

			It("preserves version when moved to completed", func() {
				// Set container and version
				err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetContainer(ctx, path, "dark-factory-001-test")
				Expect(err).To(BeNil())

				err = prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					SetVersion(ctx, path, "v0.5.0")
				Expect(err).To(BeNil())

				// Move to completed
				completedDir := filepath.Join(tempDir, "completed")
				err = prompt.NewManager("", "", completedDir, mover, libtime.NewCurrentDateTime()).
					MoveToCompleted(ctx, path)
				Expect(err).To(BeNil())

				// Verify version is preserved in completed file
				completedPath := filepath.Join(tempDir, "completed", "001-test.md")
				fm, err := prompt.NewManager("", "", "", nil, libtime.NewCurrentDateTime()).
					ReadFrontmatter(ctx, completedPath)
				Expect(err).To(BeNil())
				Expect(fm.Status).To(Equal("completed"))
				Expect(fm.Container).To(Equal("dark-factory-001-test"))
				Expect(fm.DarkFactoryVersion).To(Equal("v0.5.0"))
			})
		})
	})

})
